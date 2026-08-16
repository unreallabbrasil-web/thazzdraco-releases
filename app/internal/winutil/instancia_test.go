//go:build windows

package winutil

import (
	"net"
	"os"
	"testing"
)

// urlDaInstancia vira DESTINO DE JANELA. Um valor podre aqui abre a janela do
// app apontando para outro lugar — ou para uma porta morta, que da tela em
// branco e e pior que nao abrir nada.

func TestURLDaInstanciaRecusaLixo(t *testing.T) {
	defer LimparURLInstancia()
	casos := map[string]string{
		"vazio":            "",
		"não é http":       "file:///C:/Windows/System32",
		"não é loopback":   "http://192.168.0.10:5000/",
		"host externo":     "http://exemplo.com/",
		"sem porta":        "http://127.0.0.1/",
		"texto qualquer":   "abre isto por favor",
		"porta sem ninguém": "http://127.0.0.1:1/",
	}
	for nome, valor := range casos {
		t.Run(nome, func(t *testing.T) {
			GravarURLInstancia(valor)
			if got := urlDaInstancia(); got != "" {
				t.Fatalf("aceitou %q e devolveu %q", valor, got)
			}
		})
	}
}

func TestURLDaInstanciaAceitaPortaViva(t *testing.T) {
	defer LimparURLInstancia()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("sem permissão para abrir porta local")
	}
	defer ln.Close()

	u := "http://" + ln.Addr().String() + "/"
	GravarURLInstancia(u)
	if got := urlDaInstancia(); got != u {
		t.Fatalf("urlDaInstancia() = %q, queria %q", got, u)
	}

	// Instância que morreu: o arquivo continua lá, mas ninguém atende. Não pode
	// virar janela — o arquivo velho é o caso comum depois de um crash.
	ln.Close()
	if got := urlDaInstancia(); got != "" {
		t.Fatalf("aceitou uma porta que já não atende: %q", got)
	}
}

// A trava e o que impede duas instancias de subirem dois servidores e se
// atropelarem no cookie de sessao. CreateMutex com o mesmo nome devolve
// ERROR_ALREADY_EXISTS inclusive para o proprio processo, entao da para provar
// o comportamento aqui sem abrir o app duas vezes.
func TestInstanciaUnicaBloqueiaASegunda(t *testing.T) {
	primeira, _ := InstanciaUnica()
	if !primeira {
		t.Skip("já existe um ThazzDraco rodando nesta máquina — nada a provar aqui")
	}
	segunda, _ := InstanciaUnica()
	if segunda {
		t.Fatal("a segunda tentativa tomou a trava: duas instâncias subiriam, " +
			"e a mais antiga passaria a receber 403 em tudo")
	}
}

func TestLimparURLInstancia(t *testing.T) {
	GravarURLInstancia("http://127.0.0.1:9/")
	LimparURLInstancia()
	if _, err := os.Stat(arquivoURL()); !os.IsNotExist(err) {
		t.Fatal("o arquivo de instância continuou no disco depois da limpeza")
	}
}
