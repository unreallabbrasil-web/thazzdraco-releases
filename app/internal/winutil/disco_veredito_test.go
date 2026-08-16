//go:build windows

package winutil

import (
	"strings"
	"testing"
)

// O veredito e o que o usuario le antes de apagar. Se ele mentir, mente com
// autoridade — entao as regras que o produzem tem que estar presas por teste.

func TestVereditoNaoPrometeSeguroParaDadoDoUsuario(t *testing.T) {
	casos := []struct {
		nome    string
		classe  string
		usoDias int
		quer    string
	}{
		// Cache e sempre seguro: por definicao ele se refaz. O que a evidencia
		// muda e o CUSTO, nao o SE.
		{"cache usado hoje", ClasseLimpavel, 0, VerdSeguro},
		{"cache parado há meses", ClasseLimpavel, 200, VerdSeguro},
		{"cache sem data", ClasseLimpavel, -1, VerdSeguro},
		// Dado do usuario NUNCA vira "seguro" automaticamente, por mais velho
		// que esteja: quem decide sobre arquivo do usuario e o usuario.
		{"dado usado hoje", ClasseRelatorio, 0, VerdCuidado},
		{"dado de 3 meses", ClasseRelatorio, 90, VerdCuidado},
		{"dado de 2 anos", ClasseRelatorio, 730, VerdProvavel},
		{"dado sem data", ClasseRelatorio, -1, VerdCuidado},
		// Intocavel e intocavel.
		{"sistema", ClasseIntocavel, 0, VerdNao},
		{"sistema antigo", ClasseIntocavel, 5000, VerdNao},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			cat := catAchado{classe: c.classe, dono: "Fulano", volta: "volta sozinho",
				aviso: "quebra tudo; sério mesmo"}
			a := Achado{Classe: c.classe}
			e := avaliar(cat, &a, c.usoDias)
			if e.Veredito != c.quer {
				t.Fatalf("veredito %q, queria %q (frase: %q)", e.Veredito, c.quer, e.Frase)
			}
			if strings.TrimSpace(e.Frase) == "" {
				t.Fatal("veredito sem frase — o selo sozinho não explica nada")
			}
		})
	}
}

func TestVereditoUsaODonoAusenteComoProva(t *testing.T) {
	cat := catAchado{classe: ClasseLimpavel, dono: "Cargo", donoExe: "cargo-que-nao-existe-mesmo",
		volta: "volta no próximo build"}
	a := Achado{Classe: ClasseLimpavel}
	e := avaliar(cat, &a, 400)
	if e.DonoTem != "nao" {
		t.Fatalf("devia detectar que o dono não está instalado, veio %q", e.DonoTem)
	}
	if !strings.Contains(e.Frase, "nem está instalado") {
		t.Fatalf("a frase devia usar a ausência do dono como prova: %q", e.Frase)
	}
}

func TestQuandoTexto(t *testing.T) {
	casos := map[int]string{-1: "em data desconhecida", 0: "hoje", 1: "ontem", 12: "há 12 dias",
		90: "há 3 meses", 400: "há mais de um ano", 1100: "há 3 anos"}
	for dias, quer := range casos {
		if got := quandoTexto(dias); got != quer {
			t.Errorf("quandoTexto(%d) = %q, queria %q", dias, got, quer)
		}
	}
}

// nomeLegivel existe porque "SpotifyAB.SpotifyMusic_zpdnekdrzrea0" nao diz nada
// para quem esta tentando decidir se apaga 11 GB.
func TestNomeLegivelDaStore(t *testing.T) {
	casos := map[string]string{
		"SpotifyAB.SpotifyMusic_zpdnekdrzrea0":     "SpotifyMusic",
		"5319275A.WhatsAppDesktop_cv1g1gvanyjgm":   "WhatsAppDesktop",
		"Claude_abc123":                            "Claude",
		"Microsoft.WindowsCalculator_8wekyb3d8bbwe": "WindowsCalculator",
	}
	for in, quer := range casos {
		if got := nomeLegivel(in, "store"); got != quer {
			t.Errorf("nomeLegivel(%q) = %q, queria %q", in, got, quer)
		}
	}
	// fora da Store, o nome fica como está — é nome de pasta de verdade.
	if got := nomeLegivel("models--meta-llama--Llama-3", "modelos"); got != "models--meta-llama--Llama-3" {
		t.Errorf("não devia mexer em nome que não é da Store: %q", got)
	}
}

// familiasInstaladas le o registro real desta maquina. E a fonte que responde
// "este app ainda existe?" — se ela vier vazia, todo app da Store seria marcado
// como sobra de desinstalacao, que e o erro mais caro que este modulo pode
// cometer.
func TestFamiliasInstaladasLeORegistroDeVerdade(t *testing.T) {
	fam := familiasInstaladas()
	if len(fam) == 0 {
		t.Skip("sem apps da Store neste perfil (ou sem hive de usuário): nada a conferir")
	}
	t.Logf("famílias instaladas lidas do registro: %d", len(fam))
	// Toda instalacao do Windows 10/11 tem pelo menos um app inbox da Microsoft.
	achouMS := false
	for f := range fam {
		if strings.HasPrefix(f, "microsoft.") {
			achouMS = true
			break
		}
	}
	if !achouMS {
		t.Fatal("nenhuma família Microsoft — a derivação Nome_PublisherId provavelmente quebrou")
	}
}

func TestProgramaExiste(t *testing.T) {
	if !programaExiste("cmd") {
		t.Fatal("cmd deveria existir no PATH de qualquer Windows")
	}
	if programaExiste("programa-que-nao-existe-em-lugar-nenhum-xyz") {
		t.Fatal("achou um programa que não existe")
	}
	if programaExiste("") {
		t.Fatal("string vazia não é programa")
	}
}
