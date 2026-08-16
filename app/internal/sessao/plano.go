package sessao

import (
	"sort"
	"strconv"
	"strings"

	"thazzdraco/internal/engine"
	"thazzdraco/internal/winutil"
)

// O plano (docs/SESSAO.md §5). A varredura devolve uma lista de regras soltas;
// o plano e o que transforma isso em FILA: agrupada por fase, ordenada por
// motivo real, com selecao padrao que respeita a intencao do atendimento e os
// invariantes de seguranca.
//
// GerarPlano e funcao PURA (nao le registro, nao escreve nada): recebe o que ja
// foi lido e devolve a fila. E por isso que da para testa-la de verdade.
//
// As relacoes entre regras (depende_de / conflita_com, em rules.json) sao
// resolvidas aqui: ordenam a fila e bloqueiam o que nao pode ser marcado ainda.

// Fases de execucao.
const (
	FaseDireta = 1 // aplica agora, efeito imediato
	FaseReboot = 2 // so vale depois de reiniciar
	FaseManual = 3 // BIOS, Windows ou decisao humana — o app nao faz por voce
)

// NomeFase e o rotulo humano de cada fase.
func NomeFase(f int) string {
	switch f {
	case FaseDireta:
		return "Aplica agora"
	case FaseReboot:
		return "Pede reinício"
	case FaseManual:
		return "Na mão"
	}
	return "?"
}

type PlanoItem struct {
	ID         string `json:"id"`   // "regra:win.game-mode" — estavel, serve de chave na selecao
	Tipo       string `json:"tipo"` // regra | limpeza | manual | pagina
	Ref        string `json:"ref"`  // id da regra, id do gargalo…
	Titulo     string `json:"titulo"`
	Descricao  string `json:"descricao"`
	Porque     string `json:"porque"`      // por que este item esta no plano (texto pronto)
	OrigemTipo string `json:"origem_tipo"` // gargalo | perfil | extra
	Fase       int    `json:"fase"`
	Ordem      int    `json:"ordem"`
	Tier       string `json:"tier"`    // verde | amarelo | vermelho
	Impacto    string `json:"impacto"` // alto | medio | baixo
	Tecnico    string `json:"tecnico"` // o que mexe no sistema (transparencia)
	Orientacao string `json:"orientacao,omitempty"`
	Alvo       string `json:"alvo,omitempty"` // pagina do app, para itens tipo "pagina"
	Detectado  string `json:"detectado,omitempty"`

	RequerReboot        bool `json:"requer_reboot"`
	RequerConsentimento bool `json:"requer_consentimento"`
	Selecionavel        bool `json:"selecionavel"` // false = item de checklist humano
	Selecionado         bool `json:"selecionado"`

	DependeDe   []string `json:"depende_de,omitempty"`
	ConflitaCom []string `json:"conflita_com,omitempty"`

	Bloqueado string `json:"bloqueado,omitempty"` // motivo legível de por que não dá para marcar
	Estado    string `json:"estado"`              // pendente (S5 preenche o resto)
	Erro      string `json:"erro,omitempty"`
	BatchID   string `json:"batch_id,omitempty"` // lote que aplicou → liga ao historico de undo
}

type PlanoResumo struct {
	Total          int            `json:"total"`
	Selecionados   int            `json:"selecionados"`
	PorFase        map[string]int `json:"por_fase"`       // fase → selecionados
	TotalPorFase   map[string]int `json:"total_por_fase"` // fase → itens
	PedemReboot    int            `json:"pedem_reboot"`
	PedemConsent   int            `json:"pedem_consentimento"`
	Bloqueados     int            `json:"bloqueados"`
	ScoreAtual     int            `json:"score_atual"`
	ScoreProjetado int            `json:"score_projetado"`
}

type Plano struct {
	Gerado string      `json:"gerado"`
	Origem string      `json:"origem"` // "intencao:competitivo"
	Itens  []PlanoItem `json:"itens"`
	Resumo PlanoResumo `json:"resumo"`

	// Contagens da varredura que gerou o plano. Ficam guardadas para recalcular a
	// projecao de score quando o usuario mexe na selecao, sem escanear de novo.
	BaseAplicados int `json:"base_aplicados"`
	BasePendentes int `json:"base_pendentes"`

	// Dependencias que este PC JA satisfaz (a regra ja estava aplicada). Ficam
	// guardadas porque a resolucao roda de novo a cada clique na selecao, e sem
	// isso um item apareceria bloqueado por uma dependencia que ja esta feita.
	DepsAplicadas []string `json:"deps_aplicadas,omitempty"`
}

// Entradas e tudo que o planejador precisa. Vem pronto de fora (scan, diagnose,
// presets) — o planejador nao vai ao sistema.
type Entradas struct {
	Intencao string
	Regras   []engine.RuleView
	Gargalos []winutil.Gargalo
	Preset   []string // ids do preset da intencao ("livre" = vazio)
}

var rankImpacto = map[string]int{"alto": 0, "medio": 1, "baixo": 2}
var rankTier = map[string]int{"verde": 0, "amarelo": 1, "vermelho": 2}

// consultivaDoGargalo liga o gargalo a regra consultiva que diz a MESMA coisa.
// Sem isso o plano mostra o item duas vezes ("Tela abaixo da taxa máxima" do
// diagnostico e "Usar a taxa de atualizacao maxima" da regra) — e o tecnico fica
// achando que sao duas tarefas. Ganha o gargalo, que carrega o valor medido.
//
// Isto vive aqui, e nao em rules.json, porque liga dois catalogos DIFERENTES
// (gargalo x regra) — depende_de/conflita_com sao entre regras, e nao serviriam.
var consultivaDoGargalo = map[string]string{
	"display.refresh": "display.refresh-rate-max",
	"ram.xmp":         "ram.xmp-profile",
}

// origem de um item vindo do diagnostico
type deGargalo struct {
	sev       string
	titulo    string
	detectado string
}

// GerarPlano monta a fila a partir do que ja foi lido.
func GerarPlano(e Entradas) *Plano {
	intencao := normalizaIntencao(e.Intencao)
	emPreset := map[string]bool{}
	for _, id := range e.Preset {
		emPreset[id] = true
	}

	// 1) Diagnostico: gargalo que tem regra vira MOTIVO da regra; gargalo sem
	//    regra vira item proprio na fase 3 (o app nao entra na BIOS por voce).
	porRegra := map[string]deGargalo{}
	jaDito := map[string]bool{} // regra consultiva que o gargalo já cobre
	var itens []PlanoItem
	for _, g := range e.Gargalos {
		if g.Severidade != "critico" && g.Severidade != "atencao" {
			continue // "bom"/"info" nao geram trabalho
		}
		if id, ok := consultivaDoGargalo[g.ID]; ok {
			jaDito[id] = true
		}
		switch {
		case strings.HasPrefix(g.Acao, "regra:"):
			porRegra[strings.TrimPrefix(g.Acao, "regra:")] = deGargalo{g.Severidade, g.Titulo, g.Detectado}
		case g.Acao == "manual" || strings.HasPrefix(g.Acao, "pagina:"):
			tipo, alvo := "manual", ""
			if strings.HasPrefix(g.Acao, "pagina:") {
				tipo, alvo = "pagina", strings.TrimPrefix(g.Acao, "pagina:")
			}
			itens = append(itens, PlanoItem{
				ID: tipo + ":" + g.ID, Tipo: tipo, Ref: g.ID,
				Titulo: g.Titulo, Descricao: g.Impacto,
				Porque: "Diagnóstico: " + g.Detectado, OrigemTipo: "gargalo",
				Fase: FaseManual, Tier: "verde", Impacto: impactoDaSeveridade(g.Severidade),
				Orientacao: g.Correcao, Alvo: alvo, Detectado: g.Detectado,
				Estado: "pendente",
			})
		}
	}

	// 2) Regras. O plano mostra TUDO que da para fazer neste PC; a intencao
	//    decide o que ja vem marcado. Esconder opcao seria decidir pelo tecnico.
	aplicados, pendentes := 0, 0
	for _, rv := range e.Regras {
		switch rv.Estado {
		case "aplicado":
			aplicados++
		case "pendente":
			pendentes++
		}

		if jaDito[rv.ID] {
			continue // o gargalo já diz isso, com o valor medido
		}
		it, ok := itemDaRegra(rv)
		if !ok {
			continue
		}
		if g, temGargalo := porRegra[rv.ID]; temGargalo {
			it.OrigemTipo = "gargalo"
			it.Porque = "Diagnóstico: " + textoGargalo(g)
			it.Detectado = g.detectado
			if g.sev == "critico" {
				it.Impacto = "alto" // gargalo crítico manda no impacto, não o rótulo genérico da regra
			}
		} else if emPreset[rv.ID] {
			it.OrigemTipo = "perfil"
			it.Porque = "Faz parte do perfil " + intencao
		} else {
			it.OrigemTipo = "extra"
			switch {
			case it.Selecionavel:
				it.Porque = "Ganho disponível fora do perfil escolhido"
			case it.Detectado != "":
				it.Porque = "Medido: " + it.Detectado // consultiva: o número é que sustenta a recomendação
			default:
				it.Porque = "Recomendação — você aplica, o app não mexe"
			}
		}
		it.Selecionado = selecionaPorPadrao(it, rv, intencao, emPreset[rv.ID])
		itens = append(itens, it)
	}

	// 3) Ordena: fase, depois DEPENDENCIA (quem depende vem sempre depois de quem
	//    e dependido — isso e ordem de execucao, nao estetica), depois o que tem
	//    motivo real (gargalo), impacto e risco. Titulo no fim so para a ordem
	//    nao dancar entre duas geracoes.
	prof := profundidades(itens)
	sort.SliceStable(itens, func(i, j int) bool {
		a, b := itens[i], itens[j]
		if a.Fase != b.Fase {
			return a.Fase < b.Fase
		}
		if pa, pb := prof[a.Ref], prof[b.Ref]; pa != pb {
			return pa < pb
		}
		if ra, rb := rankOrigem(a.OrigemTipo), rankOrigem(b.OrigemTipo); ra != rb {
			return ra < rb
		}
		if ra, rb := rankImpacto[a.Impacto], rankImpacto[b.Impacto]; ra != rb {
			return ra < rb
		}
		if ra, rb := rankTier[a.Tier], rankTier[b.Tier]; ra != rb {
			return ra < rb
		}
		return a.Titulo < b.Titulo
	})
	for i := range itens {
		itens[i].Ordem = i + 1
	}

	p := &Plano{
		Gerado: agora(), Origem: "intencao:" + intencao, Itens: itens,
		BaseAplicados: aplicados, BasePendentes: pendentes,
		DepsAplicadas: depsJaAplicadas(itens, e.Regras),
	}
	p.Recalcular() // já resolve dependências e conflitos
	return p
}

// depsJaAplicadas lista as dependencias que este PC ja satisfaz. Uma regra ja
// aplicada some do plano (nao ha o que fazer com ela) — sem esta lista, quem
// depende dela apareceria bloqueado por algo que ja esta feito.
func depsJaAplicadas(itens []PlanoItem, regras []engine.RuleView) []string {
	precisa := map[string]bool{}
	for _, it := range itens {
		for _, d := range it.DependeDe {
			precisa[d] = true
		}
	}
	if len(precisa) == 0 {
		return nil
	}
	var out []string
	for _, rv := range regras {
		if precisa[rv.ID] && rv.Estado == "aplicado" {
			out = append(out, rv.ID)
		}
	}
	sort.Strings(out)
	return out
}

// profundidades mede a distancia de cada item ate uma regra sem dependencia.
// Ordenar por ela dentro da fase garante que quem depende venha depois — sem
// precisar de uma ordenacao topologica inteira para duas relacoes.
func profundidades(itens []PlanoItem) map[string]int {
	deps := map[string][]string{}
	for _, it := range itens {
		if len(it.DependeDe) > 0 {
			deps[it.Ref] = it.DependeDe
		}
	}
	memo := map[string]int{}
	var calc func(ref string, visto map[string]bool) int
	calc = func(ref string, visto map[string]bool) int {
		if v, ok := memo[ref]; ok {
			return v
		}
		if visto[ref] {
			return 0 // ciclo: nao trava o plano, so para de contar
		}
		visto[ref] = true
		max := 0
		for _, d := range deps[ref] {
			if p := calc(d, visto) + 1; p > max {
				max = p
			}
		}
		delete(visto, ref)
		memo[ref] = max
		return max
	}
	out := map[string]int{}
	for _, it := range itens {
		out[it.Ref] = calc(it.Ref, map[string]bool{})
	}
	return out
}

// itemDaRegra converte uma regra varrida em item de plano. Devolve false quando
// nao ha o que fazer com ela (ja aplicada, fora do hardware, sem oportunidade).
func itemDaRegra(rv engine.RuleView) (PlanoItem, bool) {
	var tipo string
	fase := FaseDireta
	switch rv.Modo {
	case "acionavel":
		if rv.Estado != "pendente" || !rv.Aplicavel {
			return PlanoItem{}, false
		}
		tipo = "regra"
		if rv.RequerReboot {
			fase = FaseReboot
		}
	case "limpeza":
		if rv.Estado != "oportunidade" {
			return PlanoItem{}, false // nada relevante para limpar
		}
		tipo = "limpeza"
	case "consultivo":
		// consultiva nao aciona nada: e orientacao, entra como item de mao.
		if rv.Estado != "recomendado" && rv.Estado != "oportunidade" {
			return PlanoItem{}, false
		}
		tipo, fase = "manual", FaseManual
	default:
		return PlanoItem{}, false
	}

	return PlanoItem{
		ID: tipo + ":" + rv.ID, Tipo: tipo, Ref: rv.ID,
		Titulo: rv.Titulo, Descricao: rv.Descricao,
		Fase: fase, Tier: rv.Tier, Impacto: rv.Impacto, Tecnico: rv.Tecnico,
		Orientacao: rv.Orientacao, Detectado: rv.Detalhe,
		RequerReboot: rv.RequerReboot, RequerConsentimento: rv.RequerConsentimento,
		DependeDe: rv.DependeDe, ConflitaCom: rv.ConflitaCom,
		Selecionavel: tipo == "regra" || tipo == "limpeza",
		Estado:       "pendente",
	}, true
}

// selecionaPorPadrao decide o que ja vem marcado. Os "nao" aqui sao invariantes
// do produto, nao preferencia de UI:
//   - risco alto (vermelho) NUNCA vem marcado, em nenhuma intencao;
//   - o que pede consentimento (inclusive a limpeza, que nao tem undo) tambem
//     nao — consentimento marcado por padrao nao e consentimento;
//   - amarelo so entra pelo perfil que assumiu esse risco (ex.: Competitivo).
func selecionaPorPadrao(it PlanoItem, rv engine.RuleView, intencao string, emPreset bool) bool {
	if !it.Selecionavel || intencao == "livre" {
		return false
	}
	if rv.Tier == "vermelho" || rv.RequerConsentimento {
		return false
	}
	if rv.Tier == "amarelo" && !emPreset {
		return false
	}
	return emPreset || it.OrigemTipo == "gargalo"
}

func rankOrigem(o string) int {
	switch o {
	case "gargalo":
		return 0
	case "perfil":
		return 1
	}
	return 2
}

// textoGargalo junta titulo + o que foi medido: "HAGS desligado — HAGS desativado".
// O que foi medido e o que da credibilidade ao item; sem ele vira so um rotulo.
func textoGargalo(g deGargalo) string {
	if g.detectado == "" {
		return g.titulo
	}
	if g.titulo == "" {
		return g.detectado
	}
	return g.titulo + " — " + g.detectado
}

func impactoDaSeveridade(sev string) string {
	if sev == "critico" {
		return "alto"
	}
	return "medio"
}

// Selecionar aplica as marcacoes vindas da UI. Item nao selecionavel (checklist
// de mao) e ignorado. Devolve quantos itens mudaram.
func (p *Plano) Selecionar(marcas map[string]bool) int {
	n := 0
	for i := range p.Itens {
		it := &p.Itens[i]
		v, ok := marcas[it.ID]
		if !ok || !it.Selecionavel || it.Selecionado == v {
			continue
		}
		it.Selecionado = v
		n++
	}
	p.Recalcular()
	return n
}

// resolverRelacoes aplica depende_de e conflita_com (docs/RULES-SCHEMA.md).
//
// Roda a cada recalculo, e nao so na geracao, porque as duas relacoes dependem
// da SELECAO: marcar a dependencia desbloqueia quem depende dela, e desmarcar o
// vencedor de um conflito libera o perdedor. Um bloqueio calculado uma vez so
// ficaria mentindo no primeiro clique.
func (p *Plano) resolverRelacoes() {
	porRef := make(map[string]*PlanoItem, len(p.Itens))
	for i := range p.Itens {
		if r := p.Itens[i].Ref; r != "" {
			porRef[r] = &p.Itens[i]
		}
	}
	jaAplicada := paraSet(p.DepsAplicadas)

	for i := range p.Itens {
		p.Itens[i].Bloqueado = "" // recalculado do zero: bloqueio velho não sobrevive
	}

	// 1) Dependencias. Satisfeita = ja aplicada neste PC OU marcada no plano.
	for i := range p.Itens {
		it := &p.Itens[i]
		for _, dep := range it.DependeDe {
			if jaAplicada[dep] {
				continue
			}
			if d := porRef[dep]; d != nil && d.Selecionado {
				continue
			}
			it.Bloqueado = "depende de " + tituloDaRef(porRef, dep) + " — marque essa primeiro"
			it.Selecionado = false
			break
		}
	}

	// 2) Conflitos. A relacao e simetrica mesmo declarada de um lado so.
	conflitos := map[string]map[string]bool{}
	liga := func(a, b string) {
		if conflitos[a] == nil {
			conflitos[a] = map[string]bool{}
		}
		conflitos[a][b] = true
	}
	for _, it := range p.Itens {
		for _, o := range it.ConflitaCom {
			liga(it.Ref, o)
			liga(o, it.Ref)
		}
	}
	for i := range p.Itens {
		a := &p.Itens[i]
		if !a.Selecionado {
			continue
		}
		for ref := range conflitos[a.Ref] {
			b := porRef[ref]
			if b == nil || !b.Selecionado || b == a {
				continue
			}
			perdedor, vencedor := a, b
			if venceConflito(a, b) == a {
				perdedor, vencedor = b, a
			}
			perdedor.Selecionado = false
			perdedor.Bloqueado = "faz o mesmo que " + vencedor.Titulo + " — o plano mantém só um"
		}
	}
}

// venceConflito escolhe, entre dois itens que fazem a mesma coisa, qual fica.
// Criterios em ordem: maior impacto, menor risco, e — entre iguais — o que NAO
// pede reinicio (mesmo resultado, menos atrito para o cliente). O id no fim
// existe so para a escolha ser sempre a mesma.
func venceConflito(a, b *PlanoItem) *PlanoItem {
	if ra, rb := rankImpacto[a.Impacto], rankImpacto[b.Impacto]; ra != rb {
		if ra < rb {
			return a
		}
		return b
	}
	if ra, rb := rankTier[a.Tier], rankTier[b.Tier]; ra != rb {
		if ra < rb {
			return a
		}
		return b
	}
	if a.RequerReboot != b.RequerReboot {
		if !a.RequerReboot {
			return a
		}
		return b
	}
	if a.Ref < b.Ref {
		return a
	}
	return b
}

func tituloDaRef(porRef map[string]*PlanoItem, ref string) string {
	if it := porRef[ref]; it != nil {
		return `"` + it.Titulo + `"`
	}
	return ref
}

// Recalcular refaz o resumo. O score projetado usa a MESMA formula do Boost
// Score (engine/scan.go): os itens marcados entram como se ja estivessem
// aplicados. E projecao do score, nao previsao de FPS — quem mede FPS e a etapa
// Provar.
func (p *Plano) Recalcular() {
	p.resolverRelacoes() // bloqueio e desmarque vêm antes de contar
	r := PlanoResumo{PorFase: map[string]int{}, TotalPorFase: map[string]int{}}
	ganham := 0 // itens que mexem no que o score conta
	for _, it := range p.Itens {
		r.Total++
		r.TotalPorFase[strconv.Itoa(it.Fase)]++
		if !it.Selecionado {
			continue
		}
		r.Selecionados++
		r.PorFase[strconv.Itoa(it.Fase)]++
		if it.RequerReboot {
			r.PedemReboot++
		}
		if it.RequerConsentimento {
			r.PedemConsent++
		}
		if it.Tipo == "regra" {
			ganham++
		}
	}
	for _, it := range p.Itens {
		if it.Bloqueado != "" {
			r.Bloqueados++
		}
	}
	base := p.BaseAplicados + p.BasePendentes
	if base <= 0 {
		r.ScoreAtual, r.ScoreProjetado = 100, 100
	} else {
		r.ScoreAtual = int(float64(p.BaseAplicados) / float64(base) * 100.0)
		alvo := p.BaseAplicados + ganham
		if alvo > base {
			alvo = base
		}
		r.ScoreProjetado = int(float64(alvo) / float64(base) * 100.0)
	}
	p.Resumo = r
}
