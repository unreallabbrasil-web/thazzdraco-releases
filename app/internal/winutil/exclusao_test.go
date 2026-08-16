//go:build windows

package winutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

// O portao de exclusao e a unica coisa entre um clique e um arquivo que some.
// Estes testes existem para que uma mudanca futura no gate quebre AQUI, e nao
// no PC de um cliente.

func TestPodeExcluirRecusaOQueTemQueRecusar(t *testing.T) {
	raiz := t.TempDir()
	dentro := filepath.Join(raiz, "sub", "arquivo.bin")
	if err := os.MkdirAll(filepath.Dir(dentro), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dentro, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		nome    string
		caminho string
		raiz    string
	}{
		{"raiz de unidade", `C:\`, `C:\`},
		{"pasta do Windows", `C:\Windows\System32`, `C:\`},
		{"Program Files", `C:\Program Files\Coisa`, `C:\`},
		{"pontos de restauração", `C:\System Volume Information`, `C:\`},
		{"a própria Lixeira", `C:\$Recycle.Bin\S-1-5-21`, `C:\`},
		{"inicialização", `C:\Boot\BCD`, `C:\`},
		{"recuperação", `C:\Recovery\WindowsRE`, `C:\`},
		{"C:\\Users é raso demais", `C:\Users`, `C:\`},
		{"o perfil inteiro", `C:\Users\Fulano`, `C:\`},
		{"a pasta Downloads em si", `C:\Users\Fulano\Downloads`, `C:\`},
		{"a pasta Documents em si", `C:\Users\Fulano\Documents`, `C:\`},
		{"o OneDrive inteiro", `C:\Users\Fulano\OneDrive`, `C:\`},
		{"arquivo de paginação", `C:\Users\Fulano\AppData\pagefile.sys`, `C:\`},
		{"caminho relativo", `Downloads\x.iso`, `C:\`},
		{"salto de pasta", filepath.Join(raiz, "..", "outro"), raiz},
		{"fora da varredura", `D:\qualquer\coisa`, raiz},
		{"a própria raiz varrida", raiz, raiz},
		{"vazio", "", raiz},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if err := PodeExcluirEscolhido(c.caminho, c.raiz); err == nil {
				t.Fatalf("DEIXOU PASSAR %q — este caminho nunca pode ser apagável", c.caminho)
			}
		})
	}
}

func TestPodeExcluirAceitaOQueOUsuarioEscolheu(t *testing.T) {
	raiz := t.TempDir()
	// A troca com a limpeza por categoria: aqui pasta pessoal é permitida, desde
	// que seja o CONTEÚDO e não a pasta conhecida em si. Proibir isso seria
	// proibir justamente o caso de uso — apagar o próprio download de 30 GB.
	alvo := filepath.Join(raiz, "Downloads", "jogo-antigo.iso")
	if err := os.MkdirAll(filepath.Dir(alvo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alvo, []byte("conteudo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PodeExcluirEscolhido(alvo, raiz); err != nil {
		t.Fatalf("recusou um arquivo que o usuário escolheu dentro da varredura: %v", err)
	}
	pasta := filepath.Join(raiz, "Downloads")
	if err := PodeExcluirEscolhido(pasta, raiz); err != nil {
		t.Fatalf("recusou uma pasta comum dentro da varredura: %v", err)
	}
}

func TestExcluirSemConfirmarNaoApaga(t *testing.T) {
	raiz := t.TempDir()
	alvo := filepath.Join(raiz, "x.bin")
	if err := os.WriteFile(alvo, []byte("dados"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExcluirEscolhidos([]string{alvo}, raiz, false, false); err == nil {
		t.Fatal("apagou sem confirmação explícita")
	}
	if _, err := os.Stat(alvo); err != nil {
		t.Fatal("o arquivo sumiu mesmo sem confirmação")
	}
}

func TestExcluirRecusadoNaoContaComoLiberado(t *testing.T) {
	raiz := t.TempDir()
	bom := filepath.Join(raiz, "bom.bin")
	if err := os.WriteFile(bom, make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ExcluirEscolhidos([]string{bom, `C:\Windows\System32`, `C:\`}, raiz, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Recusados) != 2 {
		t.Fatalf("esperava 2 recusas, veio %d: %v", len(res.Recusados), res.Recusados)
	}
	if res.Liberado != 2048 {
		t.Fatalf("liberado devia contar só o que sumiu de verdade: %d", res.Liberado)
	}
	if _, err := os.Stat(bom); err == nil {
		t.Fatal("o arquivo escolhido continua no disco")
	}
}

func TestExcluirNaoSegueJuncao(t *testing.T) {
	raiz := t.TempDir()
	real := filepath.Join(raiz, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	dentro := filepath.Join(real, "importante.txt")
	if err := os.WriteFile(dentro, []byte("nao pode sumir"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(raiz, "atalho")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("sem permissão para criar link simbólico neste ambiente")
	}
	// Apagar pelo link levaria a exclusão para fora do que o usuário viu — que é
	// o pior acidente possível aqui.
	if err := PodeExcluirEscolhido(link, raiz); err == nil {
		t.Fatal("aceitou apagar por uma junção/link")
	}
	if _, err := os.Stat(dentro); err != nil {
		t.Fatal("o arquivo do outro lado do link sumiu")
	}
}

func TestDentroDaVarredura(t *testing.T) {
	casos := []struct {
		caminho, raiz string
		quer          bool
	}{
		{`C:\Jogos\steam`, `C:\Jogos`, true},
		{`C:\Jogos`, `C:\Jogos`, true},
		{`C:\JogosOutro\x`, `C:\Jogos`, false}, // prefixo de texto não é prefixo de pasta
		{`C:\Windows`, `C:\Jogos`, false},
		{`C:\Jogos\..\Windows`, `C:\Jogos`, false},
		{"", `C:\Jogos`, false},
		{`C:\Jogos\x`, "", false},
	}
	for _, c := range casos {
		if got := DentroDaVarredura(c.caminho, c.raiz); got != c.quer {
			t.Errorf("DentroDaVarredura(%q, %q) = %v, queria %v", c.caminho, c.raiz, got, c.quer)
		}
	}
}

// O layout do SHFILEOPSTRUCTW e o que decide se a flag FOF_ALLOWUNDO cai no
// lugar certo. Se ela cair errado, a API apaga DE VEZ achando que esta mandando
// para a Lixeira — a falha mais cara possivel aqui, e silenciosa.
func TestLayoutDoSHFileOpBateComOWin32(t *testing.T) {
	var op shFileOpStructW
	if got := unsafe.Sizeof(op); got != 56 {
		t.Fatalf("SHFILEOPSTRUCTW deveria ter 56 bytes em x64, tem %d", got)
	}
	if got := unsafe.Offsetof(op.fFlags); got != 32 {
		t.Fatalf("fFlags no offset errado (%d, esperado 32) — FOF_ALLOWUNDO não chegaria à API", got)
	}
	if got := unsafe.Offsetof(op.pFrom); got != 16 {
		t.Fatalf("pFrom no offset errado (%d, esperado 16)", got)
	}
}

func TestCaminhoLongoPrefixa(t *testing.T) {
	casos := map[string]string{
		`C:\Users\x`:            `\\?\C:\Users\x`,
		`\\?\C:\ja tem`:         `\\?\C:\ja tem`,
		`\\servidor\pasta\arq`:  `\\?\UNC\servidor\pasta\arq`,
	}
	for in, quer := range casos {
		if got := caminhoLongo(in); got != quer {
			t.Errorf("caminhoLongo(%q) = %q, queria %q", in, got, quer)
		}
	}
}

func TestAlvoDeVarreduraNormalizaERecusa(t *testing.T) {
	dir := t.TempDir()
	if got, err := AlvoDeVarredura(dir); err != nil || got != filepath.Clean(dir) {
		t.Fatalf("AlvoDeVarredura(%q) = %q, %v", dir, got, err)
	}
	// "D:" sozinho vira a raiz da unidade — é como o técnico digita.
	sysDrive := strings.TrimSpace(os.Getenv("SystemDrive"))
	if got, err := AlvoDeVarredura(sysDrive); err != nil || got != sysDrive+`\` {
		t.Fatalf("AlvoDeVarredura(%q) = %q, %v — queria a raiz", sysDrive, got, err)
	}
	for _, ruim := range []string{"", "pasta\\relativa", filepath.Join(dir, "nao-existe")} {
		if _, err := AlvoDeVarredura(ruim); err == nil {
			t.Errorf("aceitou alvo inválido %q", ruim)
		}
	}
}
