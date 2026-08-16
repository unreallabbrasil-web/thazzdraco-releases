//go:build windows

package winutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Classificacao do que a varredura de disco encontrou (docs/DISCO-LIMPEZA.md).
//
// Esta e a parte mais perigosa do app: e a unica que apaga arquivo, e apagar nao
// tem undo. Por isso o mundo e dividido em quatro classes, e a diferenca entre
// elas e o que o app PODE fazer — nao o que seria conveniente.
//
// O catalogo mora em Go, e nao num JSON de dados, de proposito: a lista de
// caminhos apagaveis e uma fronteira de seguranca. Em Go ela e compilada,
// revisavel e coberta por teste; num JSON, um erro de digitacao vira exclusao
// de pasta de sistema.

// Classes de achado.
const (
	ClasseLimpavel  = "limpavel"  // cache regenerável: o app pode oferecer limpar
	ClasseCoberto   = "coberto"   // a Limpeza profunda já cuida disso — atalho, não duplicar
	ClasseRelatorio = "relatorio" // dado do usuário: só mostra o tamanho, NUNCA apaga
	ClasseIntocavel = "intocavel" // parece lixo e não é; mostra e explica por quê
)

// Achado e uma linha do bloco "o que dá para liberar".
type Achado struct {
	ID          string   `json:"id"`
	Nome        string   `json:"nome"`
	Classe      string   `json:"classe"`
	Descricao   string   `json:"descricao"` // o que acontece se apagar / por que não dá
	Bytes       int64    `json:"bytes"`
	Arquivos    int64    `json:"arquivos"`
	Caminhos    []string `json:"caminhos"`
	Aviso       string   `json:"aviso,omitempty"`
	Atalho      string   `json:"atalho,omitempty"` // página do app que já faz isso
	Recomendado bool     `json:"recomendado"`
}

// ArquivoAchado e um arquivo listado num achado de relatorio (instaladores).
type ArquivoAchado struct {
	Caminho string `json:"caminho"`
	Bytes   int64  `json:"bytes"`
	Idade   int    `json:"idade_dias"`
}

// ResultadoAchados e a resposta completa da classificacao.
type ResultadoAchados struct {
	Raiz         string           `json:"raiz"`
	Achados      []Achado         `json:"achados"`
	Instaladores []ArquivoAchado  `json:"instaladores,omitempty"`
	Duplicados   []GrupoDuplicado `json:"duplicados,omitempty"`
	LiberavelJa  int64            `json:"liberavel_ja"` // soma do que é limpável aqui
}

// catalogo descreve uma categoria antes de ser medida.
type catAchado struct {
	id, nome, classe, descricao, aviso, atalho string
	recomendado                                bool
	pastas                                     []string // medidas na árvore da varredura
	arquivos                                   []string // medidos com stat (pagefile etc.)
}

// montaCatalogo resolve os caminhos para o usuario real desta maquina.
func montaCatalogo(sid string) []catAchado {
	perfil := RealUserProfileDir(sid)
	if len(perfil) < 3 || perfil[1] != ':' {
		perfil = os.Getenv("USERPROFILE")
	}
	pj := func(p ...string) string {
		if perfil == "" {
			return ""
		}
		return filepath.Join(append([]string{perfil}, p...)...)
	}
	la := func(p ...string) string { return pj(append([]string{"AppData", "Local"}, p...)...) }
	win := os.Getenv("SystemRoot")
	if len(win) < 3 || win[1] != ':' {
		win = `C:\Windows`
	}
	pd := os.Getenv("ProgramData")
	if len(pd) < 3 || pd[1] != ':' {
		pd = `C:\ProgramData`
	}
	sysDrive := filepath.VolumeName(win) + `\`

	return []catAchado{
		// ---- Cache regenerável: o app pode oferecer limpar --------------------
		{
			id: "cache-npm", nome: "Cache do npm", classe: ClasseLimpavel, recomendado: true,
			descricao: "Pacotes JavaScript já baixados. O próximo `npm install` rebaixa o que precisar.",
			pastas:    []string{la("npm-cache")},
		},
		{
			id: "cache-pip", nome: "Cache do pip", classe: ClasseLimpavel, recomendado: true,
			descricao: "Pacotes Python já baixados. O próximo `pip install` rebaixa o que precisar.",
			pastas:    []string{la("pip", "cache")},
		},
		{
			id: "cache-go", nome: "Cache de build do Go", classe: ClasseLimpavel, recomendado: true,
			descricao: "Resultados de compilação. A próxima compilação refaz — fica mais lenta uma vez só.",
			pastas:    []string{la("go-build")},
		},
		{
			id: "cache-yarn", nome: "Cache do Yarn", classe: ClasseLimpavel, recomendado: true,
			descricao: "Pacotes já baixados pelo Yarn; voltam sozinhos no próximo install.",
			pastas:    []string{la("Yarn", "Cache"), pj(".yarn", "cache")},
		},
		{
			id: "cache-gradle", nome: "Cache do Gradle", classe: ClasseLimpavel, recomendado: true,
			descricao: "Dependências de projetos Java/Android; voltam no próximo build.",
			pastas:    []string{pj(".gradle", "caches")},
		},
		{
			id: "cache-cargo", nome: "Cache do Cargo (Rust)", classe: ClasseLimpavel, recomendado: true,
			descricao: "Fontes e pacotes baixados do crates.io; voltam no próximo build.",
			pastas:    []string{pj(".cargo", "registry", "cache"), pj(".cargo", "registry", "src")},
		},
		{
			id: "cache-nuget", nome: "Pacotes do NuGet", classe: ClasseLimpavel,
			descricao: "Pacotes .NET baixados. Some e o próximo restore rebaixa.",
			aviso:     "Sem internet no momento do build, o projeto não compila até rebaixar.",
			pastas:    []string{pj(".nuget", "packages")},
		},

		// ---- Já coberto pela Limpeza profunda: atalho, não duplicar -----------
		{
			id: "shader-cache", nome: "Cache de shaders (GPU)", classe: ClasseCoberto, atalho: "limpeza",
			descricao: "Shaders compilados de NVIDIA/AMD/DirectX. A Limpeza profunda já trata disso.",
			pastas:    []string{la("D3DSCache"), la("NVIDIA", "DXCache"), la("NVIDIA", "GLCache"), la("AMD", "DxCache")},
		},
		{
			id: "temp-usuario", nome: "Temporários do usuário", classe: ClasseCoberto, atalho: "limpeza",
			descricao: "Pasta TEMP do perfil. A limpeza de temporários já trata disso.",
			pastas:    []string{la("Temp")},
		},
		{
			id: "update-cache", nome: "Cache do Windows Update", classe: ClasseCoberto, atalho: "limpeza",
			descricao: "Instaladores de atualização já aplicados. A Limpeza profunda já trata disso.",
			pastas:    []string{filepath.Join(win, "SoftwareDistribution", "Download")},
		},
		{
			id: "windows-old", nome: "Instalação anterior do Windows", classe: ClasseCoberto, atalho: "limpeza",
			descricao: "Sobra de upgrade do Windows. A Limpeza profunda remove com confirmação.",
			pastas:    []string{filepath.Join(sysDrive, "Windows.old")},
		},

		// ---- Dado do usuário: SÓ RELATÓRIO -----------------------------------
		{
			id: "modelos-hf", nome: "Modelos de IA (Hugging Face)", classe: ClasseRelatorio,
			descricao: "Modelos que você baixou de propósito. São GB que levam muito tempo para voltar — o app não apaga.",
			pastas:    []string{pj(".cache", "huggingface")},
		},
		{
			id: "modelos-ollama", nome: "Modelos de IA (Ollama)", classe: ClasseRelatorio,
			descricao: "Acervo de modelos baixados, não cache. Quem decide o que sai é você.",
			pastas:    []string{pj(".ollama")},
		},
		{
			id: "downloads", nome: "Downloads", classe: ClasseRelatorio,
			descricao: "Pasta pessoal. O app lista os instaladores grandes e antigos, e não apaga nada.",
			pastas:    []string{pj("Downloads")},
		},
		{
			id: "dados-apps", nome: "Dados de apps da Store", classe: ClasseRelatorio,
			descricao: "Cada app da Microsoft Store guarda dados aqui. Apagar em bloco quebra os apps.",
			pastas:    []string{la("Packages")},
		},

		// ---- Parece lixo e não é ---------------------------------------------
		{
			id: "windows-installer", nome: "Cache do instalador do Windows", classe: ClasseIntocavel,
			descricao: "Guarda o necessário para DESINSTALAR e REPARAR os programas instalados. " +
				"É a armadilha clássica dos limpadores de PC: apagar aqui parece liberar espaço e quebra a desinstalação.",
			pastas: []string{filepath.Join(win, "Installer")},
		},
		{
			id: "package-cache", nome: "Package Cache (Visual Studio / redistribuíveis)", classe: ClasseIntocavel,
			descricao: "Instaladores usados para reparar Visual Studio e pacotes redistribuíveis.",
			pastas:    []string{filepath.Join(pd, "Package Cache")},
		},
		{
			id: "winsxs", nome: "Componentes do Windows (WinSxS)", classe: ClasseIntocavel,
			descricao: "Não se apaga à mão. Só encolhe pelo caminho oficial: Reparo → DISM (limpeza de componentes).",
			atalho:    "reparo",
			pastas:    []string{filepath.Join(win, "WinSxS")},
		},
		{
			id: "arquivos-kernel", nome: "Paginação e hibernação", classe: ClasseIntocavel,
			descricao: "Arquivos em uso pelo Windows. Mudam por configuração (memória virtual / hibernação), nunca por exclusão.",
			atalho:    "reparar",
			arquivos:  []string{filepath.Join(sysDrive, "pagefile.sys"), filepath.Join(sysDrive, "hiberfil.sys"), filepath.Join(sysDrive, "swapfile.sys")},
		},
		{
			id: "restauracao", nome: "Pontos de restauração", classe: ClasseIntocavel,
			descricao: "É a própria rede de segurança do app: o que permite voltar atrás depois de uma otimização.",
			pastas:    []string{filepath.Join(sysDrive, "System Volume Information")},
		},
	}
}

// Achados classifica a ultima varredura. Usa os tamanhos JA MEDIDOS na arvore —
// nao varre de novo, e nao estima nada.
func (g *gerenciadorDisco) Achados(sid string) (*ResultadoAchados, bool) {
	g.mu.Lock()
	res := g.resultado
	g.mu.Unlock()
	if res == nil || res.raiz == nil {
		return nil, false
	}

	out := &ResultadoAchados{Raiz: res.Raiz}
	for _, c := range montaCatalogo(sid) {
		a := Achado{
			ID: c.id, Nome: c.nome, Classe: c.classe, Descricao: c.descricao,
			Aviso: c.aviso, Atalho: c.atalho, Recomendado: c.recomendado,
		}
		for _, p := range c.pastas {
			if p == "" {
				continue
			}
			no := acha(res.raiz, p) // só encontra o que está sob a raiz varrida
			if no == nil || no.Bytes == 0 {
				continue
			}
			a.Bytes += no.Bytes
			a.Arquivos += no.Arquivos
			a.Caminhos = append(a.Caminhos, p)
		}
		for _, f := range c.arquivos {
			st, err := os.Stat(f)
			if err != nil || st.IsDir() || st.Size() == 0 {
				continue
			}
			a.Bytes += st.Size()
			a.Arquivos++
			a.Caminhos = append(a.Caminhos, f)
		}
		if len(a.Caminhos) == 0 {
			continue // não existe nesta máquina: não polui a tela
		}
		if a.Classe == ClasseLimpavel {
			out.LiberavelJa += a.Bytes
		}
		out.Achados = append(out.Achados, a)
	}

	sort.SliceStable(out.Achados, func(i, j int) bool {
		if ri, rj := rankClasse(out.Achados[i].Classe), rankClasse(out.Achados[j].Classe); ri != rj {
			return ri < rj
		}
		return out.Achados[i].Bytes > out.Achados[j].Bytes
	})
	out.Instaladores = instaladoresEmDownloads(sid)
	out.Duplicados = g.Duplicados()
	return out, true
}

// CaminhoDeAchado diz se um caminho veio da propria classificacao. E o que
// permite "abrir a pasta" sem virar um endpoint que abre qualquer coisa que o
// cliente mandar: o caminho tem que estar na lista que o backend acabou de
// produzir, nao no corpo da requisicao.
func (g *gerenciadorDisco) CaminhoDeAchado(sid, caminho string) bool {
	ach, ok := g.Achados(sid)
	if !ok {
		return false
	}
	alvo := strings.TrimRight(strings.ToLower(strings.TrimSpace(caminho)), `\`)
	if alvo == "" {
		return false
	}
	for _, a := range ach.Achados {
		for _, c := range a.Caminhos {
			if strings.TrimRight(strings.ToLower(c), `\`) == alvo {
				return true
			}
		}
	}
	for _, f := range ach.Instaladores {
		if strings.ToLower(f.Caminho) == alvo {
			return true
		}
	}
	for _, d := range ach.Duplicados {
		for _, c := range d.Caminhos {
			if strings.ToLower(c) == alvo {
				return true
			}
		}
	}
	return false
}

func rankClasse(c string) int {
	switch c {
	case ClasseLimpavel:
		return 0
	case ClasseCoberto:
		return 1
	case ClasseRelatorio:
		return 2
	}
	return 3
}

// AbrirNoExplorer abre a pasta (ou seleciona o arquivo) no Explorer do Windows.
// Quem valida se o caminho pode ser aberto e o chamador — aqui so executa.
func AbrirNoExplorer(caminho string) error {
	st, err := os.Stat(caminho)
	if err != nil {
		return err
	}
	args := []string{caminho}
	if !st.IsDir() {
		args = []string{"/select,", caminho} // abre a pasta com o arquivo já marcado
	}
	cmd := exec.Command("explorer.exe", args...)
	// explorer.exe devolve código de saída != 0 mesmo quando abre certo; o que
	// importa é ter conseguido iniciar o processo.
	_ = cmd.Start()
	return nil
}

// minInstalador: abaixo disso nao vale a pena mostrar na lista.
const minInstalador = 50 << 20 // 50 MB

var extInstalador = map[string]bool{
	".exe": true, ".msi": true, ".msix": true, ".appx": true,
	".iso": true, ".zip": true, ".7z": true, ".rar": true,
}

// instaladoresEmDownloads lista os arquivos grandes de instalacao na pasta
// Downloads. E RELATORIO: pasta pessoal o app nao apaga, so mostra para o dono
// decidir. Le so o primeiro nivel — e barato e e onde as coisas caem.
func instaladoresEmDownloads(sid string) []ArquivoAchado {
	perfil := RealUserProfileDir(sid)
	if len(perfil) < 3 || perfil[1] != ':' {
		perfil = os.Getenv("USERPROFILE")
	}
	if perfil == "" {
		return nil
	}
	dir := filepath.Join(perfil, "Downloads")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	agora := time.Now()
	var out []ArquivoAchado
	for _, e := range ents {
		if e.IsDir() || !extInstalador[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() < minInstalador {
			continue
		}
		out = append(out, ArquivoAchado{
			Caminho: filepath.Join(dir, e.Name()),
			Bytes:   info.Size(),
			Idade:   int(agora.Sub(info.ModTime()).Hours() / 24),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	if len(out) > 15 {
		out = out[:15]
	}
	return out
}
