//go:build windows

package winutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Este é o único código do app que apaga arquivo, e apagar não tem undo. Cada
// teste aqui é um acidente que NÃO pode acontecer na máquina de um cliente.

func TestPodeApagarRecusaOQueNaoPode(t *testing.T) {
	casos := []struct {
		caminho, porque string
	}{
		{`C:\`, "raiz da unidade"},
		{`C:\Windows`, "raso demais"},
		{`C:\Users`, "raso demais"},
		{`C:\Windows\Installer`, "quebra desinstalação de programas"},
		{`C:\Windows\System32\drivers`, "sistema"},
		{`C:\Windows\WinSxS\Backup`, "componentes do Windows"},
		{`C:\Program Files\qualquer\coisa`, "programas instalados"},
		{`C:\ProgramData\Package Cache\algo`, "instaladores de reparo"},
		{`C:\Users\fulano\Desktop`, "pasta pessoal"},
		{`C:\Users\fulano\Documents\sub`, "pasta pessoal"},
		{`C:\Users\fulano\Downloads\sub`, "pasta pessoal"},
		{`C:\Users\fulano\OneDrive\x`, "nuvem sincronizada"},
		{`C:\Users\fulano\Saved Games\jogo`, "saves de jogo"},
		{`C:\System Volume Information`, "pontos de restauração"},
		{`AppData\Local\npm-cache`, "caminho relativo"},
		{`C:\Users\fulano\..\..\Windows\System32`, "salto de pasta"},
		{``, "vazio"},
	}
	for _, c := range casos {
		if err := podeApagar(c.caminho); err == nil {
			t.Errorf("podeApagar(%q) FOI PERMITIDO — devia recusar: %s", c.caminho, c.porque)
		}
	}
}

func TestPodeApagarAceitaCachePropriamenteFundo(t *testing.T) {
	base := t.TempDir() // ex.: C:\Users\...\Temp\TestXxx
	alvo := filepath.Join(base, "npm-cache")
	if err := os.MkdirAll(alvo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := podeApagar(alvo); err != nil {
		t.Errorf("um cache legítimo foi recusado: %v", err)
	}
}

func TestPodeApagarRecusaArquivoEJuncao(t *testing.T) {
	base := t.TempDir()
	arq := filepath.Join(base, "arquivo.bin")
	os.WriteFile(arq, []byte("x"), 0o644)
	if err := podeApagar(arq); err == nil {
		t.Error("arquivo solto não pode ser alvo — só conteúdo de pasta")
	}

	real := filepath.Join(base, "real")
	os.MkdirAll(real, 0o755)
	link := filepath.Join(base, "link")
	if err := exec.Command("cmd", "/c", "mklink", "/J", link, real).Run(); err != nil {
		t.Skipf("sem junção neste ambiente: %v", err)
	}
	if err := podeApagar(link); err == nil {
		t.Error("junção foi aceita — apagar por ela sairia da pasta autorizada")
	}
}

func TestApagaConteudoLimpaMasMantemAPastaDeCima(t *testing.T) {
	base := t.TempDir()
	cache := filepath.Join(base, "cache")
	escreve(t, filepath.Join(cache, "a.bin"), 1000)
	escreve(t, filepath.Join(cache, "sub", "b.bin"), 2000)
	escreve(t, filepath.Join(cache, "sub", "fundo", "c.bin"), 3000)

	bytes, arquivos, travados := apagaConteudo(cache)
	if bytes != 6000 || arquivos != 3 {
		t.Errorf("apagou %d bytes em %d arquivos; queria 6000/3", bytes, arquivos)
	}
	if travados != 0 {
		t.Errorf("%d arquivos travados sem motivo", travados)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Error("a pasta de cima tem que continuar existindo — programas contam com ela")
	}
	if _, err := os.Stat(filepath.Join(cache, "sub")); err == nil {
		t.Error("subpasta vazia devia ter sido removida")
	}
}

// Se uma junção apontar para fora, apagar o conteúdo NÃO pode atravessá-la.
// É o pior acidente possível: apagar a pasta errada por causa de um link.
func TestApagaConteudoNaoAtravessaJuncao(t *testing.T) {
	base := t.TempDir()
	fora := filepath.Join(base, "NAO_TOCAR")
	escreve(t, filepath.Join(fora, "importante.bin"), 500)

	cache := filepath.Join(base, "cache")
	os.MkdirAll(cache, 0o755)
	escreve(t, filepath.Join(cache, "lixo.bin"), 100)
	link := filepath.Join(cache, "atalho")
	if err := exec.Command("cmd", "/c", "mklink", "/J", link, fora).Run(); err != nil {
		t.Skipf("sem junção neste ambiente: %v", err)
	}

	bytes, _, _ := apagaConteudo(cache)
	if bytes != 100 {
		t.Errorf("apagou %d bytes; devia apagar só os 100 do lixo local", bytes)
	}
	if _, err := os.Stat(filepath.Join(fora, "importante.bin")); err != nil {
		t.Fatal("APAGOU ATRAVÉS DA JUNÇÃO — o arquivo de fora sumiu")
	}
}

func TestLimparExigeConfirmacaoExplicita(t *testing.T) {
	if _, err := Limpar("", []string{"cache-npm"}, false); err != ErrPrecisaConfirmarLimpeza {
		t.Errorf("sem confirmar: erro = %v, queria %v", err, ErrPrecisaConfirmarLimpeza)
	}
	if _, err := Limpar("", nil, true); err == nil {
		t.Error("limpar sem categoria nenhuma devia falhar")
	}
}

// A trava que mais importa: mesmo que a UI (ou qualquer cliente) peça para
// limpar uma categoria intocável, o backend recusa.
func TestLimparRecusaCategoriaQueNaoEhLimpavel(t *testing.T) {
	res, err := Limpar("", []string{"windows-installer", "dados-apps", "modelos-hf", "nao-existe"}, true)
	if err != nil {
		t.Fatalf("a chamada não devia falhar, e sim recusar item a item: %v", err)
	}
	if res.Liberado != 0 {
		t.Errorf("liberou %d bytes de categorias que não são limpáveis", res.Liberado)
	}
	if len(res.Categorias) != 0 {
		t.Errorf("executou %d categorias que deviam ser recusadas", len(res.Categorias))
	}
	if len(res.Recusadas) != 4 {
		t.Errorf("recusadas = %d, queria 4: %v", len(res.Recusadas), res.Recusadas)
	}
	juntas := strings.Join(res.Recusadas, " | ")
	if !strings.Contains(juntas, "não é limpável") {
		t.Errorf("a recusa tem que dizer o motivo; veio: %s", juntas)
	}
}

// A cadeia inteira, de ponta a ponta, num perfil de mentira: catálogo →
// podeApagar → apagaConteudo. Prova que a limpeza realmente apaga o cache certo
// E que não encosta no que está ao lado.
func TestLimparApagaOCacheCertoENadaMais(t *testing.T) {
	falso := t.TempDir()
	t.Setenv("USERPROFILE", falso) // o catálogo cai aqui quando não há SID

	npm := filepath.Join(falso, "AppData", "Local", "npm-cache")
	escreve(t, filepath.Join(npm, "_cacache", "conteudo", "a.bin"), 5000)
	escreve(t, filepath.Join(npm, "b.bin"), 1000)

	// vizinhos que NÃO podem ser tocados
	docs := filepath.Join(falso, "Documents")
	escreve(t, filepath.Join(docs, "curriculo.docx"), 999)
	pip := filepath.Join(falso, "AppData", "Local", "pip", "cache")
	escreve(t, filepath.Join(pip, "roda.whl"), 777)

	res, err := Limpar("", []string{"cache-npm"}, true)
	if err != nil {
		t.Fatalf("limpar: %v", err)
	}
	if res.Liberado != 6000 {
		t.Errorf("liberou %d bytes, queria 6000", res.Liberado)
	}
	if len(res.Categorias) != 1 || res.Categorias[0].Arquivos != 2 {
		t.Errorf("categorias = %+v", res.Categorias)
	}
	if _, err := os.Stat(npm); err != nil {
		t.Error("a pasta npm-cache tem que continuar existindo — o npm conta com ela")
	}
	if _, err := os.Stat(filepath.Join(npm, "b.bin")); err == nil {
		t.Error("o arquivo do cache não foi apagado")
	}
	// o que não foi pedido continua intacto
	if _, err := os.Stat(filepath.Join(docs, "curriculo.docx")); err != nil {
		t.Fatal("APAGOU FORA DA CATEGORIA — arquivo pessoal sumiu")
	}
	if _, err := os.Stat(filepath.Join(pip, "roda.whl")); err != nil {
		t.Fatal("APAGOU outra categoria que não foi pedida")
	}
}

// Todo caminho de toda categoria limpável tem que passar por podeApagar — é a
// terceira camada, a que pega erro de quem editar o catálogo no futuro.
func TestTodoCaminhoLimpavelPassaNaValidacao(t *testing.T) {
	for _, c := range montaCatalogo("") {
		if c.classe != ClasseLimpavel {
			continue
		}
		for _, p := range c.pastas {
			if p == "" {
				continue
			}
			err := podeApagar(p)
			// não existir nesta máquina é normal; o que não pode é ser PROIBIDO
			if err != nil && !os.IsNotExist(err) {
				t.Errorf("categoria %q aponta para caminho que a validação recusa: %v", c.id, err)
			}
		}
	}
}
