//go:build windows

package winutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// O explorador de disco tem que dar o MESMO número em qualquer máquina — e as
// máquinas de verdade têm caminho gigante de node_modules, junção do Windows e
// pasta sem permissão. Cada teste aqui é uma dessas realidades, para a resposta
// não depender de sorte no PC do cliente.

// varreSincrono roda uma varredura e espera terminar.
func varreSincrono(t *testing.T, raiz string) *ResultadoDisco {
	t.Helper()
	g := &gerenciadorDisco{estado: "ocioso"}
	cancelar := make(chan struct{})
	g.varrer(raiz+`\`, cancelar)
	if g.resultado == nil {
		t.Fatal("varredura não produziu resultado")
	}
	return g.resultado
}

func escreve(t *testing.T, caminho string, bytes int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caminho, make([]byte, bytes), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSomaPorPastaBateComOQueFoiEscrito(t *testing.T) {
	raiz := t.TempDir()
	escreve(t, filepath.Join(raiz, "a.bin"), 1000)
	escreve(t, filepath.Join(raiz, "sub", "b.bin"), 2000)
	escreve(t, filepath.Join(raiz, "sub", "fundo", "c.bin"), 3000)

	res := varreSincrono(t, raiz)
	if res.Bytes != 6000 {
		t.Errorf("total = %d, queria 6000", res.Bytes)
	}
	if res.Arquivos != 3 {
		t.Errorf("arquivos = %d, queria 3", res.Arquivos)
	}
	// a soma de uma pasta tem que incluir tudo que está abaixo dela
	sub := acha(res.raiz, filepath.Join(raiz, "sub"))
	if sub == nil {
		t.Fatal("subpasta não entrou na árvore")
	}
	if sub.Bytes != 5000 {
		t.Errorf("sub = %d bytes, queria 5000 (2000 dela + 3000 da subpasta)", sub.Bytes)
	}
}

// Caminho longo é onde varredor amador quebra: o Windows corta em 260
// caracteres, e um node_modules fundo passa disso sozinho. Se falhar aqui, o app
// mente o tamanho justamente na pasta que mais pesa.
func TestCaminhoLongoNaoPerdeNada(t *testing.T) {
	raiz := t.TempDir()
	fundo := raiz
	for i := 0; i < 14; i++ {
		fundo = filepath.Join(fundo, "node_modules", "pacote-com-nome-bem-comprido-para-esticar")
	}
	if len(fundo) < 260 {
		t.Fatalf("o caminho de teste precisa passar de 260 caracteres; tem %d", len(fundo))
	}
	escreve(t, filepath.Join(fundo, "arquivo.bin"), 4096)

	res := varreSincrono(t, raiz)
	if res.Bytes != 4096 || res.Arquivos != 1 {
		t.Errorf("caminho de %d caracteres: %d bytes / %d arquivos; queria 4096/1",
			len(fundo), res.Bytes, res.Arquivos)
	}
	if res.SemAcesso != 0 {
		t.Errorf("%d pastas ficaram inacessíveis num caminho que existe", res.SemAcesso)
	}
}

// Junção (junction) é o pesadelo silencioso: o Windows tem várias de fábrica no
// perfil do usuário. Seguir uma conta a mesma pasta duas vezes — no melhor caso
// o total mente, no pior a varredura entra em laço e nunca termina.
func TestNaoSegueJuncao(t *testing.T) {
	raiz := t.TempDir()
	alvo := filepath.Join(raiz, "real")
	escreve(t, filepath.Join(alvo, "dados.bin"), 8192)

	link := filepath.Join(raiz, "atalho")
	if err := exec.Command("cmd", "/c", "mklink", "/J", link, alvo).Run(); err != nil {
		t.Skipf("não deu para criar junção neste ambiente: %v", err)
	}

	res := varreSincrono(t, raiz)
	if res.Bytes != 8192 {
		t.Errorf("total = %d, queria 8192 — a junção foi contada duas vezes", res.Bytes)
	}
	if res.Arquivos != 1 {
		t.Errorf("arquivos = %d, queria 1", res.Arquivos)
	}
}

func TestMaioresArquivosVemOrdenados(t *testing.T) {
	raiz := t.TempDir()
	escreve(t, filepath.Join(raiz, "pequeno.bin"), 100)
	escreve(t, filepath.Join(raiz, "grande.bin"), 9000)
	escreve(t, filepath.Join(raiz, "medio.bin"), 500)

	res := varreSincrono(t, raiz)
	if len(res.Maiores) < 3 {
		t.Fatalf("maiores = %d itens", len(res.Maiores))
	}
	if res.Maiores[0].Bytes != 9000 || res.Maiores[1].Bytes != 500 {
		t.Errorf("ordem errada: %+v", res.Maiores[:2])
	}
}

// A árvore só pode responder por caminho que está dentro da raiz varrida —
// senão a consulta viraria uma janela para o disco inteiro.
func TestArvoreRecusaCaminhoForaDaRaiz(t *testing.T) {
	raiz := t.TempDir()
	escreve(t, filepath.Join(raiz, "a.bin"), 10)
	res := varreSincrono(t, raiz)

	if acha(res.raiz, `C:\Windows\System32`) != nil {
		t.Error("caminho fora da raiz varrida não pode ser encontrado na árvore")
	}
	if acha(res.raiz, filepath.Join(raiz, "nao-existe")) != nil {
		t.Error("pasta inexistente não pode devolver nó")
	}
}

// ---- Classificação: a fronteira de segurança --------------------------------

// O catálogo é o que decide o que o app pode apagar. Estas invariantes valem
// mais que qualquer GB liberado.
func TestCatalogoNaoOfereceApagarNadaPerigoso(t *testing.T) {
	proibidos := []string{
		`\windows\installer`, `\package cache`, `\winsxs`, `\system32`, `\syswow64`,
		`\system volume information`, `\desktop`, `\documents`, `\pictures`, `\videos`, `\music`,
		`pagefile.sys`, `hiberfil.sys`,
	}
	for _, c := range montaCatalogo("") {
		if c.classe != ClasseLimpavel {
			continue // só o que o app se propõe a apagar precisa passar por aqui
		}
		for _, p := range append(append([]string{}, c.pastas...), c.arquivos...) {
			low := strings.ToLower(p)
			for _, proib := range proibidos {
				if strings.Contains(low, proib) {
					t.Errorf("categoria %q é limpável e aponta para %q — caminho proibido (%s)",
						c.id, p, proib)
				}
			}
		}
	}
}

// Downloads é pasta pessoal: pode ser MEDIDA, nunca oferecida para apagar.
func TestPastaPessoalNuncaEhLimpavel(t *testing.T) {
	for _, c := range montaCatalogo("") {
		if c.classe != ClasseLimpavel {
			continue
		}
		for _, p := range c.pastas {
			if strings.Contains(strings.ToLower(p), `\downloads`) {
				t.Errorf("categoria %q oferece apagar dentro de Downloads (%q)", c.id, p)
			}
		}
	}
}

// Toda categoria precisa explicar o que é — a tela mostra isso, e uma linha sem
// explicação vira o técnico apagando no escuro.
func TestTodaCategoriaSeExplica(t *testing.T) {
	ids := map[string]bool{}
	for _, c := range montaCatalogo("") {
		if c.id == "" || c.nome == "" || c.descricao == "" {
			t.Errorf("categoria incompleta: %+v", c.id)
		}
		if ids[c.id] {
			t.Errorf("id repetido no catálogo: %q", c.id)
		}
		ids[c.id] = true
		switch c.classe {
		case ClasseLimpavel, ClasseCoberto, ClasseRelatorio, ClasseIntocavel:
		default:
			t.Errorf("categoria %q tem classe desconhecida %q", c.id, c.classe)
		}
	}
}

// ---- Duplicados ---------------------------------------------------------------

func TestDuplicadosConfirmamPorConteudo(t *testing.T) {
	raiz := t.TempDir()
	conteudo := make([]byte, 4096)
	for i := range conteudo {
		conteudo[i] = byte(i % 251)
	}
	a := filepath.Join(raiz, "copia1.bin")
	b := filepath.Join(raiz, "copia2.bin")
	c := filepath.Join(raiz, "diferente.bin")
	os.WriteFile(a, conteudo, 0o644)
	os.WriteFile(b, conteudo, 0o644)
	diferente := append([]byte{}, conteudo...)
	diferente[0] = 0xFF // mesmo tamanho, conteúdo diferente
	os.WriteFile(c, diferente, 0o644)

	ha, err := hashPontas(a, 4096)
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := hashPontas(b, 4096)
	hc, _ := hashPontas(c, 4096)
	if ha != hb {
		t.Error("cópias idênticas têm que bater no hash das pontas")
	}
	if ha == hc {
		t.Error("mesmo tamanho com conteúdo diferente NÃO pode ser tratado como duplicado")
	}
}
