//go:build windows

package winutil

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Instancia unica.
//
// Sem isto, clicar duas vezes no .exe subia DOIS servidores em portas
// diferentes — e como cookie nao tem porta no escopo (RFC 6265 §8.5), a segunda
// instancia sobrescrevia o cookie de sessao da primeira. A janela da primeira
// passava a receber 403 em toda chamada, para sempre, porque a tela nao
// recarrega sozinha.
//
// E o clique duplo nao e descuido: no PC de um cliente o primeiro clique demora
// (SmartScreen, UAC), a pessoa acha que nao pegou e clica de novo. Tem que
// funcionar assim mesmo.
//
// A trava e um mutex nomeado do Windows, que o proprio sistema libera se o
// processo morrer — inclusive se morrer de forma feia. Um arquivo de lock
// deixaria o app travado para sempre depois de um crash.

const nomeMutexInstancia = `Local\ThazzDracoOptimizer`

// mantido vivo pelo tempo do processo: se o handle for coletado e fechado, a
// trava some e uma segunda instancia sobe.
var handleInstancia windows.Handle

// InstanciaUnica tenta tomar a trava.
//
// Devolve (true, "") quando esta instancia e a unica. Devolve (false, url)
// quando ja existe outra rodando — a url e a da instancia viva, para que este
// processo apenas abra a janela dela em vez de subir um servidor concorrente.
func InstanciaUnica() (bool, string) {
	nome, err := windows.UTF16PtrFromString(nomeMutexInstancia)
	if err != nil {
		return true, "" // nome inválido não deveria acontecer; não trava o app por isso
	}
	h, err := windows.CreateMutex(nil, false, nome)
	if h != 0 {
		handleInstancia = h
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		return false, urlDaInstancia()
	}
	// Qualquer outra falha (permissão, nome em uso por outro objeto): seguir em
	// frente. Falhar em abrir o app por causa da trava seria pior que o problema
	// que ela evita.
	return true, ""
}

func arquivoURL() string { return filepath.Join(DataDir(), "instancia.url") }

// GravarURLInstancia registra onde esta instancia esta atendendo.
func GravarURLInstancia(u string) {
	_ = WriteFileAtomic(arquivoURL(), []byte(u))
}

// LimparURLInstancia apaga o registro (saida limpa).
func LimparURLInstancia() { _ = os.Remove(arquivoURL()) }

// urlDaInstancia le a URL da instancia viva e confere que ela e mesmo local e
// esta mesmo atendendo. Sem a conferencia, um arquivo velho de uma execucao
// anterior faria a janela abrir numa porta morta — tela em branco, que e pior
// que nao abrir nada.
func urlDaInstancia() string {
	b, err := os.ReadFile(arquivoURL())
	if err != nil {
		return ""
	}
	u := strings.TrimSpace(string(b))
	p, err := url.Parse(u)
	if err != nil || p.Scheme != "http" {
		return ""
	}
	host, porta, err := net.SplitHostPort(p.Host)
	if err != nil {
		return ""
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return "" // só loopback: este valor vira destino de janela
	}
	c, err := net.Dial("tcp", net.JoinHostPort(host, porta))
	if err != nil {
		return "" // ninguém atendendo: arquivo velho
	}
	c.Close()
	return u
}
