//go:build windows

package winutil

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Veredito: a resposta para "posso apagar isto?".
//
// A tela antiga dizia "cache regeneravel" e pedia confianca. Confianca nao e o
// que falta — falta PROVA. Quem esta na frente do PC nao sabe se o npm-cache
// serve para alguma coisa, se aqueles 26 GB da Store sao de app instalado, se o
// modelo de IA ainda e usado. E ninguem deveria precisar saber: o app tem os
// dados para responder.
//
// Entao o veredito nunca aparece sozinho. Ele vem sempre com as tres evidencias
// que o produziram:
//
//	1. QUANDO foi usado pela ultima vez  (sai da varredura, de graca)
//	2. QUEM e o dono, e se o dono existe (PATH / registro de apps da Store)
//	3. COMO volta se voce apagar         (sozinho / baixando de novo / nunca)
//
// Um veredito sem evidencia e so mais um aviso — e aviso e exatamente o que ja
// existia e nao resolvia.

const (
	VerdSeguro   = "seguro"   // some e volta sozinho; nada seu se perde
	VerdProvavel = "provavel" // provavelmente nao faz falta, mas custa para voltar
	VerdCuidado  = "cuidado"  // dado de programa que voce usa
	VerdNao      = "nao"      // apagar quebra
)

// Evidencia e o porque do veredito, em dados verificaveis.
type Evidencia struct {
	Veredito  string `json:"veredito"`
	Frase     string `json:"frase"`               // a linha que a UI mostra em destaque
	UsoDias   int    `json:"uso_dias"`            // -1 = desconhecido
	Dono      string `json:"dono,omitempty"`      // "npm", "Spotify"...
	DonoTem   string `json:"dono_tem,omitempty"`  // "sim" | "nao" | "" (não se aplica)
	ComoVolta string `json:"como_volta,omitempty"`
	Detalhavel bool  `json:"detalhavel,omitempty"` // dá para abrir item por item
}

// ---- Evidência 2: o dono existe? --------------------------------------------

// programaExiste diz se o programa dono do cache esta instalado.
//
// Se o npm nao esta instalado, o npm-cache e peso morto puro — nao ha o que
// "rebaixar depois", porque nao ha quem baixe. Essa e a diferenca entre "seguro
// apagar" e "seguro apagar e voce nem vai notar".
func programaExiste(exe string) bool {
	if exe == "" {
		return false
	}
	_, err := exec.LookPath(exe)
	return err == nil
}

// familiasInstaladas le as familias de apps da Store instaladas para o usuario.
//
// A pasta %LOCALAPPDATA%\Packages guarda os dados de cada app da Store — e NAO
// e limpa quando o app e desinstalado. Cruzar as duas listas responde a pergunta
// que ninguem consegue responder olhando a pasta: "este app ainda existe?".
//
// A chave guarda o nome COMPLETO do pacote
// (Nome_Versao_Arquitetura_Recurso_PublisherId); o nome da pasta e a FAMILIA
// (Nome_PublisherId). Dai a montagem com a primeira e a ultima parte.
func familiasInstaladas() map[string]bool {
	out := map[string]bool{}
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`,
		registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return out
	}
	defer k.Close()
	nomes, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return out
	}
	for _, n := range nomes {
		p := strings.Split(n, "_")
		if len(p) < 5 {
			continue
		}
		out[strings.ToLower(p[0]+"_"+p[len(p)-1])] = true
	}
	return out
}

// ---- O veredito --------------------------------------------------------------

// avaliar monta a evidencia de um achado ja medido.
func avaliar(c catAchado, a *Achado, usoDias int) Evidencia {
	e := Evidencia{UsoDias: usoDias, Dono: c.dono, ComoVolta: c.volta}
	if c.dono != "" && c.donoExe != "" {
		if programaExiste(c.donoExe) {
			e.DonoTem = "sim"
		} else {
			e.DonoTem = "nao"
		}
	}

	switch a.Classe {
	case ClasseIntocavel:
		e.Veredito = VerdNao
		e.Frase = "Não apague. " + primeiraFrase(c.aviso)
		return e

	case ClasseCoberto:
		e.Veredito = VerdSeguro
		e.Frase = "Seguro, mas outra tela já cuida disto — evita fazer o mesmo trabalho duas vezes."
		return e

	case ClasseLimpavel:
		// Cache e sempre seguro: por definicao ele se refaz. O que muda com a
		// evidencia nao e o SE, e o CUSTO — e o custo e o que decide na pratica.
		e.Veredito = VerdSeguro
		switch {
		case e.DonoTem == "nao":
			e.Frase = fmt.Sprintf("Pode apagar sem pensar: o %s nem está instalado nesta máquina, "+
				"então isto é peso morto.", c.dono)
		case usoDias >= 0 && usoDias <= 2:
			e.Frase = fmt.Sprintf("Seguro — nada seu se perde. Mas o %s usou isto %s, "+
				"então parte vai voltar logo.", c.dono, quandoTexto(usoDias))
		case usoDias > 60:
			e.Frase = fmt.Sprintf("Pode apagar: nada aqui é tocado %s, e %s.",
				quandoTexto(usoDias), minuscula(c.volta))
		default:
			e.Frase = "Seguro — nada seu se perde. " + maiuscula(c.volta) + "."
		}
		return e
	}

	// ClasseRelatorio: dado do usuario. Aqui o app nunca diz "seguro" sozinho —
	// ele mostra o que sabe e devolve a decisao, que e de quem e dono do arquivo.
	e.Detalhavel = c.detalhe != ""
	switch {
	case usoDias >= 0 && usoDias <= 7:
		e.Veredito = VerdCuidado
		e.Frase = fmt.Sprintf("Cuidado: isto foi usado %s. É dado seu, não cache.", quandoTexto(usoDias))
	case usoDias > 180:
		e.Veredito = VerdProvavel
		e.Frase = fmt.Sprintf("Nada aqui é tocado %s. Provavelmente não faz falta — mas é dado seu, "+
			"então a decisão é sua.", quandoTexto(usoDias))
	case usoDias >= 0:
		e.Veredito = VerdCuidado
		e.Frase = fmt.Sprintf("Usado %s. É dado seu; olhe o que tem dentro antes.", quandoTexto(usoDias))
	default:
		e.Veredito = VerdCuidado
		e.Frase = "É dado seu, não cache. O app mostra o tamanho e não apaga."
	}
	if e.Detalhavel {
		e.Frase += " Dá para abrir item por item."
	}
	return e
}

func quandoTexto(dias int) string {
	switch {
	case dias < 0:
		return "em data desconhecida"
	case dias == 0:
		return "hoje"
	case dias == 1:
		return "ontem"
	case dias < 30:
		return fmt.Sprintf("há %d dias", dias)
	case dias < 365:
		return fmt.Sprintf("há %d meses", dias/30)
	case dias < 730:
		return "há mais de um ano"
	}
	return fmt.Sprintf("há %d anos", dias/365)
}

func primeiraFrase(s string) string {
	if i := strings.IndexAny(s, ".;"); i > 0 {
		return maiuscula(s[:i]) + "."
	}
	return maiuscula(s)
}
func maiuscula(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
func minuscula(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// ---- Detalhamento: quebrar o agregado em itens -------------------------------

// ItemDetalhe e uma linha do detalhamento de uma categoria.
type ItemDetalhe struct {
	Nome     string `json:"nome"`
	Caminho  string `json:"caminho"`
	Bytes    int64  `json:"bytes"`
	UsoDias  int    `json:"uso_dias"`
	Veredito string `json:"veredito"`
	Frase    string `json:"frase"`
	Protegido bool  `json:"protegido,omitempty"`
	Motivo   string `json:"motivo,omitempty"`
}

// Detalhar abre uma categoria agregada em itens, com veredito por item.
//
// E o que resolve o caso mais desesperador da tela: "Dados de apps da Store,
// 26,8 GB" abre uma pasta com 134 nomes ilegiveis e ninguem sabe o que e o que.
// Quebrado por app, com o dono e a ultima utilizacao, a mesma pasta vira uma
// lista de decisoes obvias — inclusive "este app voce desinstalou".
func (g *gerenciadorDisco) Detalhar(sid, id string) ([]ItemDetalhe, bool) {
	g.mu.Lock()
	res := g.resultado
	g.mu.Unlock()
	if res == nil || res.raiz == nil {
		return nil, false
	}

	var cat *catAchado
	for _, c := range montaCatalogo(sid) {
		if c.id == id {
			cc := c
			cat = &cc
			break
		}
	}
	if cat == nil || cat.detalhe == "" {
		return nil, false
	}

	instaladas := map[string]bool{}
	if cat.detalhe == "store" {
		instaladas = familiasInstaladas()
	}

	var out []ItemDetalhe
	for _, base := range cat.pastas {
		no := acha(res.raiz, base)
		if no == nil {
			continue
		}
		for _, f := range no.Filhos {
			if f.Bytes == 0 {
				continue
			}
			it := ItemDetalhe{
				Nome: nomeLegivel(f.Nome, cat.detalhe), Caminho: f.Caminho,
				Bytes: f.Bytes, UsoDias: f.Recente,
			}
			it.Veredito, it.Frase = vereditoItem(cat.detalhe, f, instaladas)
			if err := PodeExcluirEscolhido(f.Caminho, res.Raiz); err != nil {
				it.Protegido, it.Motivo = true, motivoCurto(err)
			}
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	if len(out) > 60 {
		out = out[:60]
	}
	return out, true
}

func vereditoItem(tipo string, f *NoDisco, instaladas map[string]bool) (string, string) {
	if tipo == "store" {
		if !instaladas[strings.ToLower(f.Nome)] {
			return VerdProvavel, "O aplicativo não está mais instalado — isto é sobra de uma desinstalação."
		}
		if f.Recente >= 0 && f.Recente <= 7 {
			return VerdCuidado, fmt.Sprintf("Aplicativo instalado e usado %s. Apagar isto zera login, "+
				"configuração e o que estiver salvo dentro dele.", quandoTexto(f.Recente))
		}
		return VerdCuidado, fmt.Sprintf("Aplicativo instalado, sem uso %s. Apagar zera login e "+
			"configuração dele — o app continua funcionando, mas como recém-instalado.", quandoTexto(f.Recente))
	}
	// modelos de IA e afins: nao volta sozinho, volta baixando
	if f.Recente > 180 {
		return VerdProvavel, fmt.Sprintf("Sem uso %s. Se precisar de novo, é baixar %s outra vez.",
			quandoTexto(f.Recente), tamanhoCurto(f.Bytes))
	}
	return VerdCuidado, fmt.Sprintf("Usado %s. Baixar de novo custa %s.",
		quandoTexto(f.Recente), tamanhoCurto(f.Bytes))
}

// nomeLegivel tira o ruido do nome da pasta.
//
// "SpotifyAB.SpotifyMusic_zpdnekdrzrea0" nao diz nada para quem esta lendo;
// "SpotifyMusic" diz. O caminho completo continua na tela logo abaixo, entao
// nada e escondido — so deixa de gritar.
func nomeLegivel(nome, tipo string) string {
	if tipo != "store" {
		return nome
	}
	base := nome
	if i := strings.LastIndex(base, "_"); i > 0 {
		base = base[:i]
	}
	if i := strings.LastIndex(base, "."); i > 0 && i < len(base)-1 {
		base = base[i+1:]
	}
	return base
}

func tamanhoCurto(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%d MB", b/(1<<20))
	}
	return fmt.Sprintf("%d KB", b/1024)
}

var _ = filepath.Join // mantido: caminhos do catálogo já vêm montados
