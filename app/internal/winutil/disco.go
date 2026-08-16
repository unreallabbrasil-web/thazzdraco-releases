//go:build windows

package winutil

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Explorador de disco: descobre O QUE esta ocupando o disco.
//
// O diagnostico ja sabia dizer "9% livre em C:" e nao oferecia nada. Este
// arquivo e a resposta: varre a arvore de diretorios somando tamanho por pasta e
// guardando os maiores arquivos, para o tecnico ver em segundos onde estao os
// 400 GB — em vez de abrir o Explorer pasta por pasta.
//
// Decisoes que importam:
//   - Caminhada paralela (um worker por CPU, teto de 8). O gargalo e I/O de
//     metadados, e o SSD atende varias filas ao mesmo tempo.
//   - NAO segue reparse points (junctions/symlinks). Sem isso, "Documents and
//     Settings" e os hardlinks do WinSxS fariam a soma contar a mesma coisa duas
//     vezes — numero inflado e, no pior caso, laco infinito.
//   - Guarda um no por PASTA e so os N maiores ARQUIVOS. Um no por arquivo em
//     um disco de 1,5 milhao de arquivos custaria centenas de MB de RAM.
//   - Tamanho LOGICO (o que o arquivo tem), nao alocado em disco. E o numero que
//     o usuario reconhece; arquivo compactado/esparso pode ocupar menos.

const (
	maxWorkersDisco = 8
	maioresArquivos = 200 // quantos "maiores arquivos" guardar
)

// NoDisco e uma pasta na arvore, com o tamanho ja somado.
type NoDisco struct {
	Nome     string     `json:"nome"`
	Caminho  string     `json:"caminho"`
	Bytes    int64      `json:"bytes"`    // soma de tudo abaixo
	Arquivos int64      `json:"arquivos"` // contagem recursiva
	Filhos   []*NoDisco `json:"filhos,omitempty"`
	Entravel bool       `json:"entravel"` // tem subpasta: a UI deixa entrar
	// Recente = dias desde o arquivo mais novo em qualquer lugar abaixo daqui
	// (-1 = pasta vazia). E a evidencia mais barata e mais util que existe: uma
	// pasta de 12 GB que ninguem toca ha 8 meses e uma decisao facil; a mesma
	// pasta usada hoje e outra conversa. Sai de graca — a varredura ja le a data
	// de cada arquivo para as outras listas.
	Recente int `json:"recente"`
}

// ArquivoGrande e uma linha da lista dos maiores arquivos.
type ArquivoGrande struct {
	Caminho string `json:"caminho"`
	Bytes   int64  `json:"bytes"`
	Dias    int    `json:"dias"` // desde a última modificação
	// Protegido/Motivo dizem a UI o que ela NAO deve oferecer para apagar.
	//
	// Quem responde e a mesma funcao que recusa na hora de apagar
	// (PodeExcluirEscolhido). Se a tela decidisse isso por conta propria, as duas
	// regras iam divergir e a UI acabaria deixando marcar, somar no total e
	// confirmar uma exclusao que o backend sempre recusa — o pior tipo de
	// interface: a que promete o que nao cumpre.
	Protegido bool   `json:"protegido,omitempty"`
	Motivo    string `json:"motivo,omitempty"`
}

// anotaProtecao marca os itens que o portao de exclusao nunca vai aceitar.
func anotaProtecao(itens []ArquivoGrande, raiz string) {
	for i := range itens {
		if err := PodeExcluirEscolhido(itens[i].Caminho, raiz); err != nil {
			itens[i].Protegido = true
			itens[i].Motivo = motivoCurto(err)
		}
	}
}

// motivoCurto tira o caminho da mensagem: na UI o caminho ja esta ali do lado, e
// repetir empurra o motivo para fora da tela.
func motivoCurto(err error) string {
	m := err.Error()
	if i := strings.LastIndex(m, ": "); i > 0 {
		m = m[:i]
	}
	return m
}

// FamiliaArquivo agrupa o disco por TIPO de conteudo.
//
// "A pasta X tem 80 GB" nao diz o que fazer. "Voce tem 120 GB de video e 40 GB
// de imagem de disco (ISO)" diz — porque a decisao de apagar e por tipo de
// coisa, nao por lugar onde ela caiu.
type FamiliaArquivo struct {
	ID       string          `json:"id"`
	Nome     string          `json:"nome"`
	Bytes    int64           `json:"bytes"`
	Arquivos int64           `json:"arquivos"`
	Exemplos []ArquivoGrande `json:"exemplos,omitempty"`
}

const (
	// Um arquivo so entra na lista de "grande e parado" se for grande de verdade
	// e velho de verdade. Sem os dois filtros, a lista viraria o disco inteiro e
	// nao ajudaria ninguem a decidir nada.
	minAntigo  = 100 << 20 // 100 MB
	diasAntigo = 365
)

// minDuplicado: so arquivo grande entra na caca a duplicados. Varrer o disco
// inteiro atras de arquivos iguais acharia milhares de arquivinhos identicos que
// nao interessam a ninguem; o que importa e a maquina virtual de 8 GB copiada em
// dois lugares.
const minDuplicado = 256 << 20 // 256 MB

// ResultadoDisco e o que a varredura produz.
type ResultadoDisco struct {
	Raiz      string          `json:"raiz"`
	Bytes     int64           `json:"bytes"`
	Arquivos  int64           `json:"arquivos"`
	Pastas    int64           `json:"pastas"`
	SemAcesso int64           `json:"sem_acesso"` // pastas que o Windows recusou
	DuracaoMs int64           `json:"duracao_ms"`
	Quando    string          `json:"quando"`
	Maiores   []ArquivoGrande `json:"maiores"`
	Antigos   []ArquivoGrande `json:"antigos"`  // grandes e parados há mais de um ano
	Familias  []FamiliaArquivo `json:"familias"` // por tipo de conteúdo
	raiz      *NoDisco
	// candidatos a duplicado: tamanho exato → caminhos. So arquivos grandes.
	// Tamanho igual e so o primeiro filtro; a confirmacao vem depois, lendo as
	// pontas dos arquivos (ver Duplicados).
	dups map[int64][]string
}

// ---- Gerenciador (uma varredura por vez) ------------------------------------

type gerenciadorDisco struct {
	mu        sync.Mutex
	estado    string // "ocioso" | "varrendo" | "pronto" | "erro"
	raiz      string
	erro      string
	lidos     int64 // arquivos vistos (progresso ao vivo)
	bytes     int64
	iniciada  time.Time
	resultado *ResultadoDisco
	cancelar  chan struct{}
}

var gdisco = &gerenciadorDisco{estado: "ocioso"}

// DiscoMgr devolve o gerenciador global do explorador de disco.
func DiscoMgr() *gerenciadorDisco { return gdisco }

// Status devolve o andamento (poll da UI).
func (g *gerenciadorDisco) Status() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := map[string]any{"estado": g.estado, "raiz": g.raiz}
	switch g.estado {
	case "varrendo":
		r["arquivos"] = atomic.LoadInt64(&g.lidos)
		r["bytes"] = atomic.LoadInt64(&g.bytes)
		r["decorrido_s"] = round1(time.Since(g.iniciada).Seconds())
	case "erro":
		r["erro"] = g.erro
	case "pronto":
		r["resumo"] = g.resultado
	}
	return r
}

// Iniciar dispara a varredura de uma raiz. Uma por vez.
//
// Aceita unidade ("D:\") e tambem pasta ("E:\Jogos"): varrer 4 TB para olhar uma
// pasta e desperdicio de tempo de quem esta esperando na frente do PC.
func (g *gerenciadorDisco) Iniciar(raiz string) error {
	if strings.TrimSpace(raiz) == "" {
		raiz = os.Getenv("SystemDrive") + `\`
	}
	raiz, err := AlvoDeVarredura(raiz)
	if err != nil {
		return err
	}

	g.mu.Lock()
	if g.estado == "varrendo" {
		g.mu.Unlock()
		return errJaVarrendo
	}
	g.estado, g.raiz, g.erro = "varrendo", raiz, ""
	atomic.StoreInt64(&g.lidos, 0)
	atomic.StoreInt64(&g.bytes, 0)
	g.iniciada = time.Now()
	g.resultado = nil
	g.cancelar = make(chan struct{})
	cancelar := g.cancelar
	g.mu.Unlock()

	go g.varrer(raiz, cancelar)
	return nil
}

// RaizAtual devolve a raiz da varredura pronta.
//
// E o LIMITE DE CONSENTIMENTO da exclusao: o usuario mandou varrer aquilo, entao
// aquilo e o universo do que ele pode mandar apagar nesta tela. Sem varredura
// pronta devolve vazio, e nada pode ser excluido.
func (g *gerenciadorDisco) RaizAtual() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.resultado == nil {
		return ""
	}
	return g.resultado.Raiz
}

// Cancelar interrompe a varredura em andamento (a UI pode desistir).
func (g *gerenciadorDisco) Cancelar() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.estado == "varrendo" && g.cancelar != nil {
		close(g.cancelar)
		g.cancelar = nil
	}
}

var errJaVarrendo = os.ErrExist

// ---- A varredura -------------------------------------------------------------

type tarefaDir struct {
	caminho string
	no      *NoDisco
}

// filaDirs e a fila de pastas a visitar, com deteccao de termino.
//
// Existe porque a primeira versao usava channel + "se encher, dispara uma
// goroutine nova". Numa varredura de disco isso vira armadilha: ReadDir bloqueia
// em syscall, e o Go cria UMA THREAD DO SO por goroutine bloqueada. O processo
// chegou a 10.003 threads (o teto do runtime) e se estrangulou — a varredura nao
// terminava e ate o endpoint de status parava de responder.
//
// Aqui o paralelismo e limitado de verdade: N workers, N threads, sempre. A fila
// cresce em memoria (barato) em vez de criar goroutines (caro).
type filaDirs struct {
	mu     sync.Mutex
	cond   *sync.Cond
	itens  []tarefaDir
	ativos int // workers processando agora
	fim    bool
}

func novaFila() *filaDirs {
	q := &filaDirs{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// pop entrega a proxima pasta. Bloqueia enquanto houver trabalho em andamento
// que ainda pode gerar filhos; devolve false quando a arvore acabou.
func (q *filaDirs) pop() (tarefaDir, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.itens) == 0 {
		if q.fim {
			return tarefaDir{}, false
		}
		if q.ativos == 0 {
			q.fim = true // ninguém processando e nada na fila: acabou
			q.cond.Broadcast()
			return tarefaDir{}, false
		}
		q.cond.Wait()
	}
	// pilha (LIFO) = busca em profundidade: fila menor e melhor localidade
	t := q.itens[len(q.itens)-1]
	q.itens = q.itens[:len(q.itens)-1]
	q.ativos++
	return t, true
}

func (q *filaDirs) push(ts []tarefaDir) {
	if len(ts) == 0 {
		return
	}
	q.mu.Lock()
	q.itens = append(q.itens, ts...)
	q.cond.Broadcast()
	q.mu.Unlock()
}

// feito marca o fim do processamento de uma pasta. Tem que ser chamado DEPOIS
// de empilhar os filhos — senao outro worker veria fila vazia e encerraria tudo
// antes da hora.
func (q *filaDirs) feito() {
	q.mu.Lock()
	q.ativos--
	if q.ativos == 0 && len(q.itens) == 0 {
		q.fim = true
		q.cond.Broadcast()
	}
	q.mu.Unlock()
}

func (q *filaDirs) encerrar() {
	q.mu.Lock()
	q.fim = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (g *gerenciadorDisco) varrer(raiz string, cancelar chan struct{}) {
	inicio := time.Now()
	res := &ResultadoDisco{Raiz: raiz, dups: map[int64][]string{}}
	noRaiz := &NoDisco{Nome: raiz, Caminho: raiz}

	q := novaFila()
	var mu sync.Mutex // protege res (contadores agregados)
	maiores := novoTopArquivos(maioresArquivos)
	antigos := novoTopArquivos(maioresArquivos)
	porExt := map[string]*somaExt{} // protegido por mu
	agora := time.Now()

	workers := runtime.NumCPU()
	if workers > maxWorkersDisco {
		workers = maxWorkersDisco
	}
	if workers < 2 {
		workers = 2
	}

	processa := func(t tarefaDir) {
		select {
		case <-cancelar:
			q.encerrar()
			return
		default:
		}

		ents, err := os.ReadDir(t.caminho)
		if err != nil {
			mu.Lock()
			res.SemAcesso++
			mu.Unlock()
			return
		}

		var bytesLocais, arquivosLocais int64
		recenteLocal := -1
		var subs []*NoDisco
		// Soma por extensao acumulada LOCALMENTE e mesclada uma vez por pasta.
		// Travar o mapa global a cada arquivo seria 1,5 milhao de disputas de
		// cadeado num disco cheio; por pasta sao algumas centenas de milhares —
		// e a pasta tipica tem meia duzia de extensoes distintas.
		locais := map[string]*somaExt{}
		for _, e := range ents {
			nome := e.Name()
			cam := filepath.Join(t.caminho, nome)
			info, err := e.Info()
			if err != nil {
				continue
			}
			if ehReparsePoint(info) {
				continue // junction/symlink: contaria duas vezes ou entraria em laço
			}
			if e.IsDir() {
				sub := &NoDisco{Nome: nome, Caminho: cam}
				subs = append(subs, sub)
				continue
			}
			tam := info.Size()
			dias := int(agora.Sub(info.ModTime()).Hours() / 24)
			if dias < 0 {
				dias = 0 // arquivo com data no futuro (relógio errado, zip antigo)
			}
			bytesLocais += tam
			arquivosLocais++
			if recenteLocal < 0 || dias < recenteLocal {
				recenteLocal = dias
			}
			maiores.oferecer(cam, tam, dias)
			if tam >= minAntigo && dias >= diasAntigo {
				antigos.oferecer(cam, tam, dias)
			}

			ext := extensaoDe(nome)
			s := locais[ext]
			if s == nil {
				s = &somaExt{}
				locais[ext] = s
			}
			s.bytes += tam
			s.arquivos++
			if tam > s.maior.Bytes {
				s.maior = ArquivoGrande{Caminho: cam, Bytes: tam, Dias: dias}
			}

			if tam >= minDuplicado {
				mu.Lock()
				res.dups[tam] = append(res.dups[tam], cam)
				mu.Unlock()
			}
		}

		// só este worker mexe neste nó; os contadores globais é que precisam do lock
		t.no.Filhos = subs
		t.no.Entravel = len(subs) > 0
		t.no.Bytes = bytesLocais
		t.no.Arquivos = arquivosLocais
		t.no.Recente = recenteLocal

		mu.Lock()
		res.Pastas += int64(len(subs))
		for ext, s := range locais {
			g := porExt[ext]
			if g == nil {
				g = &somaExt{}
				porExt[ext] = g
			}
			g.bytes += s.bytes
			g.arquivos += s.arquivos
			if s.maior.Bytes > g.maior.Bytes {
				g.maior = s.maior
			}
		}
		mu.Unlock()

		// progresso ao vivo por átomo: o poll de status nunca disputa cadeado com
		// os workers (foi o que deixou o status mudo na primeira versão)
		atomic.AddInt64(&g.lidos, arquivosLocais)
		atomic.AddInt64(&g.bytes, bytesLocais)

		filhos := make([]tarefaDir, 0, len(subs))
		for _, s := range subs {
			filhos = append(filhos, tarefaDir{caminho: s.Caminho, no: s})
		}
		q.push(filhos) // empilha ANTES de marcar como feito
	}

	q.push([]tarefaDir{{caminho: raiz, no: noRaiz}})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				t, ok := q.pop()
				if !ok {
					return
				}
				processa(t)
				q.feito()
			}
		}()
	}
	wg.Wait()

	// Soma de baixo para cima: cada pasta ja tem os arquivos dela; agora herda o
	// total das subpastas.
	somaRecursiva(noRaiz)

	res.raiz = noRaiz
	res.Bytes = noRaiz.Bytes
	res.Arquivos = noRaiz.Arquivos
	res.Maiores = maiores.ordenados()
	res.Antigos = antigos.ordenados()
	res.Familias = agrupaFamilias(porExt)
	anotaProtecao(res.Maiores, raiz)
	anotaProtecao(res.Antigos, raiz)
	for i := range res.Familias {
		anotaProtecao(res.Familias[i].Exemplos, raiz)
	}
	res.DuracaoMs = time.Since(inicio).Milliseconds()
	res.Quando = time.Now().Format("2006-01-02 15:04:05")

	select {
	case <-cancelar:
		g.mu.Lock()
		g.estado = "ocioso"
		g.mu.Unlock()
		return
	default:
	}

	g.mu.Lock()
	g.resultado = res
	g.estado = "pronto"
	g.mu.Unlock()
}

// somaRecursiva propaga o tamanho das subpastas para cima.
func somaRecursiva(n *NoDisco) (int64, int64) {
	bytes, arquivos := n.Bytes, n.Arquivos
	recente := n.Recente
	for _, f := range n.Filhos {
		b, a := somaRecursiva(f)
		bytes += b
		arquivos += a
		// "tocada" vale para qualquer arquivo abaixo: uma pasta cujo neto mudou
		// hoje foi usada hoje, mesmo que ela propria nao tenha arquivo nenhum.
		if f.Recente >= 0 && (recente < 0 || f.Recente < recente) {
			recente = f.Recente
		}
	}
	n.Bytes, n.Arquivos, n.Recente = bytes, arquivos, recente
	return bytes, arquivos
}

// ehReparsePoint diz se a entrada e junction/symlink — que NAO deve ser seguida.
func ehReparsePoint(info os.FileInfo) bool {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return d.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

// ---- Consulta da arvore (sob demanda) ---------------------------------------

// Arvore devolve os filhos de um caminho, ordenados do maior para o menor.
// A UI pede um nivel por vez: mandar a arvore inteira seria um JSON de dezenas
// de MB para um disco cheio.
func (g *gerenciadorDisco) Arvore(caminho string, limite int) (map[string]any, bool) {
	g.mu.Lock()
	res := g.resultado
	g.mu.Unlock()
	if res == nil || res.raiz == nil {
		return nil, false
	}
	if limite <= 0 || limite > 500 {
		limite = 40
	}
	no := acha(res.raiz, caminho)
	if no == nil {
		return nil, false
	}

	filhos := make([]*NoDisco, len(no.Filhos))
	copy(filhos, no.Filhos)
	sort.Slice(filhos, func(i, j int) bool { return filhos[i].Bytes > filhos[j].Bytes })
	if len(filhos) > limite {
		filhos = filhos[:limite]
	}

	// copia rasa: sem os netos, para o JSON nao explodir
	var out []map[string]any
	var somaFilhos int64
	for _, f := range filhos {
		somaFilhos += f.Bytes
		linha := map[string]any{
			"nome": f.Nome, "caminho": f.Caminho, "bytes": f.Bytes,
			"arquivos": f.Arquivos, "entravel": f.Entravel,
		}
		// A UI so oferece marcar o que o portao aceitaria de verdade.
		if err := PodeExcluirEscolhido(f.Caminho, res.Raiz); err != nil {
			linha["protegido"] = true
			linha["motivo"] = motivoCurto(err)
		}
		out = append(out, linha)
	}
	// "arquivos soltos" = o que esta na pasta mas nao em subpasta nenhuma
	var somaTodas int64
	for _, f := range no.Filhos {
		somaTodas += f.Bytes
	}
	return map[string]any{
		"caminho":  no.Caminho,
		"nome":     no.Nome,
		"bytes":    no.Bytes,
		"arquivos": no.Arquivos,
		"proprios": no.Bytes - somaTodas, // bytes dos arquivos direto nesta pasta
		"filhos":   out,
		"ocultos":  len(no.Filhos) - len(filhos),
		"pai":      paiDe(no.Caminho, res.raiz.Caminho),
	}, true
}

func acha(n *NoDisco, caminho string) *NoDisco {
	alvo := strings.TrimRight(strings.ToLower(caminho), `\`)
	atual := strings.TrimRight(strings.ToLower(n.Caminho), `\`)
	if alvo == "" || alvo == atual {
		return n
	}
	if !strings.HasPrefix(alvo+`\`, atual+`\`) {
		return nil
	}
	for _, f := range n.Filhos {
		if r := acha(f, caminho); r != nil {
			return r
		}
	}
	return nil
}

func paiDe(caminho, raiz string) string {
	c := strings.TrimRight(caminho, `\`)
	r := strings.TrimRight(raiz, `\`)
	if strings.EqualFold(c, r) {
		return "" // já está na raiz
	}
	p := filepath.Dir(c)
	if len(p) < len(r) {
		return raiz
	}
	return p
}

// ---- Top N maiores arquivos --------------------------------------------------

// topArquivos guarda so os N maiores. Uma lista com todos os arquivos de um
// disco cheio custaria centenas de MB; isto custa alguns KB.
type topArquivos struct {
	mu    sync.Mutex
	n     int
	itens []ArquivoGrande
	menor int64
}

func novoTopArquivos(n int) *topArquivos {
	return &topArquivos{n: n, itens: make([]ArquivoGrande, 0, n+1)}
}

func (t *topArquivos) oferecer(caminho string, bytes int64, dias int) {
	if bytes <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.itens) == t.n && bytes <= t.menor {
		return
	}
	t.itens = append(t.itens, ArquivoGrande{Caminho: caminho, Bytes: bytes, Dias: dias})
	if len(t.itens) > t.n {
		sort.Slice(t.itens, func(i, j int) bool { return t.itens[i].Bytes > t.itens[j].Bytes })
		t.itens = t.itens[:t.n]
	}
	if len(t.itens) == t.n {
		t.menor = t.itens[len(t.itens)-1].Bytes
	}
}

func (t *topArquivos) ordenados() []ArquivoGrande {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ArquivoGrande, len(t.itens))
	copy(out, t.itens)
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}
