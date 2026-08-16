//go:build windows

package winutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// Exclusao ESCOLHIDA — o usuario aponta o arquivo/pasta e manda apagar.
//
// Isto e diferente da limpeza por categoria (disco_limpeza.go). La o app decide
// o que apagar e por isso a lista de caminhos e fechada, escrita a mao, e
// pessoal do usuario esta proibido. Aqui quem decide e o usuario, olhando o
// arquivo — entao proibir a pasta Downloads seria proibir exatamente o caso de
// uso. A troca de garantias e deliberada:
//
//	limpeza por categoria     exclusao escolhida
//	--------------------      ------------------
//	app escolhe               usuario escolhe, um por um, vendo o tamanho
//	so cache regeneravel      qualquer dado dele
//	apaga de vez              LIXEIRA por padrao (da para desfazer)
//	pessoal bloqueado         sistema bloqueado
//
// A Lixeira e a peca central. Ela transforma a unica operacao sem volta do
// produto em uma operacao com volta — e sem depender de o app estar certo sobre
// o que era importante.
//
// O que continua proibido, doa a quem doer: Windows, Program Files, area de
// inicializacao, recuperacao, pontos de restauracao, raiz de unidade e os
// arquivos que o sistema usa em tempo real (pagefile e afins). Nao existe pedido
// do cliente que libere isso.

var (
	ErrForaDaVarredura = errors.New("este caminho não faz parte da varredura atual")
	ErrPrecisaConfirmarExclusao = errors.New("a exclusão precisa de confirmação explícita")
)

// sistemaNegado sao trechos que jamais podem ser excluidos, nem a pedido.
// Diferente de `negados` (limpeza por categoria), aqui NAO entra pasta pessoal:
// apagar o proprio download e um direito de quem esta na frente do PC.
var sistemaNegado = []string{
	`\windows`,
	`\program files`,
	`\program files (x86)`,
	`\programdata\package cache`,
	`\programdata\microsoft\windows defender`,
	`\system volume information`,
	`\$recycle.bin`,
	`\recycler`,
	`\boot`,
	`\recovery`,
	`\efi`,
	`\documents and settings`, // junction herdada; seguir daria volta ao começo
	`\perflogs`,
}

// nomesNegados: arquivos em uso permanente pelo sistema.
var nomesNegados = []string{
	"pagefile.sys", "hiberfil.sys", "swapfile.sys", "ntuser.dat",
	"bootmgr", "bootnxt", "ntldr", "boot.ini", "desktop.ini",
}

// pastasPessoaisRaiz: o CONTEUDO pode ser apagado, a pasta em si nao. Some a
// pasta Downloads e o Windows recria um lugar novo, atalhos quebram e o usuario
// acha que perdeu tudo — por um clique que ele achou que apagava "os downloads".
var pastasPessoaisRaiz = []string{
	"desktop", "documents", "downloads", "pictures", "videos", "music",
	"saved games", "favorites", "links", "contacts", "searches",
	"onedrive", "dropbox", "google drive",
}

// PodeExcluirEscolhido decide se um caminho apontado pelo usuario pode ser
// excluido. Recusa por padrao: qualquer duvida vira erro.
//
// `raiz` e a raiz da varredura atual. Ela existe como LIMITE DE CONSENTIMENTO:
// o usuario mandou varrer aquilo, entao aquilo e o universo do que ele pode
// mandar apagar nesta tela. Sem esse limite, um caminho qualquer vindo do
// cliente chegaria a exclusao — e essa e a porta que a gente nao deixa aberta.
func PodeExcluirEscolhido(caminho, raiz string) error {
	c := strings.TrimSpace(caminho)
	if c == "" {
		return errors.New("caminho vazio")
	}
	if !filepath.IsAbs(c) {
		return fmt.Errorf("caminho relativo não é aceito: %q", c)
	}
	limpo := filepath.Clean(c)
	if strings.Contains(limpo, "..") {
		return fmt.Errorf("caminho com salto de pasta: %q", c)
	}
	vol := filepath.VolumeName(limpo)
	if vol == "" {
		return fmt.Errorf("caminho sem unidade: %q", c)
	}
	if strings.EqualFold(strings.TrimRight(limpo, `\`), strings.TrimRight(vol, `\`)) {
		return fmt.Errorf("a raiz da unidade nunca é apagável: %q", c)
	}

	// dentro da varredura atual
	if raiz != "" {
		r := strings.ToLower(strings.TrimRight(filepath.Clean(raiz), `\`))
		l := strings.ToLower(limpo)
		if l != r && !strings.HasPrefix(l, r+`\`) {
			return ErrForaDaVarredura
		}
		if l == r {
			return fmt.Errorf("é a raiz da varredura, não um item dentro dela: %q", c)
		}
	}

	low := strings.ToLower(limpo)
	semVol := strings.TrimPrefix(low, strings.ToLower(vol))
	for _, n := range sistemaNegado {
		if semVol == n || strings.HasPrefix(semVol, n+`\`) {
			return fmt.Errorf("caminho do sistema, protegido (%s): %q", strings.Trim(n, `\`), c)
		}
	}
	if ehNomeNegado(filepath.Base(limpo)) {
		return fmt.Errorf("arquivo usado pelo Windows em tempo real: %q", filepath.Base(limpo))
	}

	partes := strings.Split(strings.Trim(semVol, `\`), `\`)
	// Perfis: C:\Users é raso demais e C:\Users\Fulano é o perfil inteiro. Só
	// vale do terceiro nível para baixo.
	if len(partes) > 0 && (partes[0] == "users" || partes[0] == "usuários" || partes[0] == "usuarios") {
		if len(partes) < 3 {
			return fmt.Errorf("é uma pasta de perfil inteira, raso demais para apagar: %q", c)
		}
		if len(partes) == 3 && ehPastaPessoalRaiz(partes[2]) {
			return fmt.Errorf("a pasta %s em si não é apagável — o conteúdo dela é: %q", partes[2], c)
		}
	}

	st, err := os.Lstat(limpo)
	if err != nil {
		return err
	}
	if ehReparsePoint(st) {
		return fmt.Errorf("é uma junção/link, não a pasta de verdade: %q", c)
	}
	return nil
}

// DentroDaVarredura diz se um caminho pertence a raiz varrida. Usado tambem por
// quem so quer ABRIR a pasta: o app nao abre caminho que ele nunca listou.
func DentroDaVarredura(caminho, raiz string) bool {
	if caminho == "" || raiz == "" {
		return false
	}
	c := strings.ToLower(filepath.Clean(strings.TrimSpace(caminho)))
	if strings.Contains(c, "..") {
		return false
	}
	r := strings.ToLower(strings.TrimRight(filepath.Clean(raiz), `\`))
	return c == r || strings.HasPrefix(c, r+`\`)
}

func ehNomeNegado(nome string) bool {
	n := strings.ToLower(nome)
	for _, x := range nomesNegados {
		if n == x {
			return true
		}
	}
	return false
}

func ehPastaPessoalRaiz(nome string) bool {
	n := strings.ToLower(nome)
	for _, x := range pastasPessoaisRaiz {
		if n == x {
			return true
		}
	}
	return false
}

// ---- Resultado ---------------------------------------------------------------

type ItemExcluido struct {
	Caminho string `json:"caminho"`
	Nome    string `json:"nome"`
	Bytes   int64  `json:"bytes"`
	Pasta   bool   `json:"pasta"`
	Ok      bool   `json:"ok"`
	Erro    string `json:"erro,omitempty"`
}

type ResultadoExclusao struct {
	Quando    string         `json:"quando"`
	Liberado  int64          `json:"liberado"`
	ParaLixeira bool         `json:"para_lixeira"`
	Itens     []ItemExcluido `json:"itens"`
	Recusados []string       `json:"recusados,omitempty"`
}

// ExcluirEscolhidos apaga os caminhos apontados pelo usuario.
//
// paraLixeira=true manda para a Lixeira (da para restaurar pelo Windows).
// paraLixeira=false apaga de vez — so quando o usuario pede explicitamente,
// porque item grande demais nao cabe na Lixeira e ficar apagando de vez sem
// avisar seria mentir sobre o que o botao faz.
func ExcluirEscolhidos(caminhos []string, raiz string, paraLixeira, confirmar bool) (*ResultadoExclusao, error) {
	if !confirmar {
		return nil, ErrPrecisaConfirmarExclusao
	}
	if len(caminhos) == 0 {
		return nil, errors.New("nenhum item escolhido")
	}
	if len(caminhos) > 500 {
		return nil, errors.New("muitos itens de uma vez (máximo 500) — faça em partes para conseguir conferir a lista")
	}

	res := &ResultadoExclusao{
		Quando:      time.Now().Format("2006-01-02 15:04:05"),
		ParaLixeira: paraLixeira,
	}
	destino := "LIXEIRA"
	if !paraLixeira {
		destino = "DEFINITIVO"
	}
	LogOperacao(fmt.Sprintf("=== %s  EXCLUSAO ESCOLHIDA (%s)  %d itens  raiz=%s ===",
		res.Quando, destino, len(caminhos), raiz))

	vistos := map[string]bool{}
	for _, bruto := range caminhos {
		limpo := filepath.Clean(strings.TrimSpace(bruto))
		chave := strings.ToLower(limpo)
		if vistos[chave] {
			continue
		}
		vistos[chave] = true

		if err := PodeExcluirEscolhido(limpo, raiz); err != nil {
			res.Recusados = append(res.Recusados, err.Error())
			LogOperacao("    RECUSADO  " + err.Error())
			continue
		}

		it := ItemExcluido{Caminho: limpo, Nome: filepath.Base(limpo)}
		st, err := os.Lstat(limpo)
		if err != nil {
			res.Recusados = append(res.Recusados, err.Error())
			continue
		}
		it.Pasta = st.IsDir()
		if it.Pasta {
			it.Bytes = somaPasta(limpo)
		} else {
			it.Bytes = st.Size()
		}

		if paraLixeira {
			err = mandarParaLixeira(limpo)
		} else {
			err = apagarDeVez(limpo)
		}
		// A verdade e o disco, nao o codigo de retorno: se sumiu, sumiu. O
		// SHFileOperation devolve erro em situacoes em que apagou parte, e o
		// usuario precisa ver o que realmente aconteceu.
		if _, errStat := os.Lstat(limpo); errStat != nil {
			it.Ok = true
		} else if err != nil {
			it.Erro = mensagemExclusao(err)
		} else {
			it.Erro = "o Windows não relatou erro, mas o item continua no disco (em uso?)"
		}

		if it.Ok {
			res.Liberado += it.Bytes
			LogOperacao(fmt.Sprintf("    %s  %s  %.1f MB", destino, limpo, float64(it.Bytes)/(1<<20)))
		} else {
			it.Bytes = 0
			LogOperacao("    FALHOU  " + limpo + " — " + it.Erro)
		}
		res.Itens = append(res.Itens, it)
	}

	LogOperacao(fmt.Sprintf("    TOTAL LIBERADO: %.1f MB", float64(res.Liberado)/(1<<20)))
	return res, nil
}

func mensagemExclusao(err error) string {
	m := err.Error()
	switch {
	case strings.Contains(m, "sendo usado") || strings.Contains(m, "being used") || strings.Contains(m, "another process"):
		return "está em uso por um programa aberto"
	case strings.Contains(m, "Acesso negado") || strings.Contains(m, "Access is denied"):
		return "acesso negado — o Windows protege este item"
	}
	return m
}

// somaPasta soma o tamanho logico do que ha dentro. Nao segue junction, pelo
// mesmo motivo da varredura: contaria duas vezes.
func somaPasta(dir string) int64 {
	var total int64
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range ents {
		info, err := e.Info()
		if err != nil || ehReparsePoint(info) {
			continue
		}
		if e.IsDir() {
			total += somaPasta(filepath.Join(dir, e.Name()))
			continue
		}
		total += info.Size()
	}
	return total
}

// ---- Lixeira (SHFileOperation) -----------------------------------------------

var (
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
)

const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040 // sem isto, "Lixeira" seria só um rótulo bonito
	fofNoErrorUI      = 0x0400
	fofNoConfirmMkDir = 0x0200
)

// shFileOpStructW espelha SHFILEOPSTRUCTW. O alinhamento natural do Go em x64
// coincide com o do C aqui (uintptr 8, uint32 4 + 4 de padding, ponteiros 8,
// uint16 2 + 2 de padding antes do int32) — por isso nao ha campo de padding
// explicito.
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// mandarParaLixeira usa a API do Shell, que e a unica forma de um arquivo ir
// para a Lixeira de verdade (apagar o arquivo e escrever no $Recycle.Bin na mao
// gera lixo que o Windows nao sabe restaurar).
func mandarParaLixeira(caminho string) error {
	// SHFileOperation e API do Shell: precisa de COM inicializado na thread, e a
	// thread nao pode trocar no meio (o Go move goroutine entre threads).
	erroChan := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil {
			defer ole.CoUninitialize()
		}
		erroChan <- shFileOpDelete(caminho)
	}()
	return <-erroChan
}

func shFileOpDelete(caminho string) error {
	// pFrom e uma lista terminada em DOIS nulos — um do texto, outro da lista.
	// Passar so um nulo faz a API ler memoria adiante procurando o fim.
	buf, err := windows.UTF16FromString(caminho)
	if err != nil {
		return err
	}
	buf = append(buf, 0)

	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &buf[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofNoErrorUI | fofSilent | fofNoConfirmMkDir,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	runtime.KeepAlive(buf)
	if r != 0 {
		return fmt.Errorf("o Windows recusou mandar para a Lixeira (código %d)", r)
	}
	if op.fAnyOperationsAborted != 0 {
		return errors.New("a operação foi interrompida pelo Windows")
	}
	return nil
}

// apagarDeVez remove sem passar pela Lixeira. Usa o prefixo de caminho longo
// para dar conta de caminho acima de 260 caracteres — cache de node_modules
// estoura esse limite com facilidade, e o SHFileOperation nao aceita.
func apagarDeVez(caminho string) error {
	return os.RemoveAll(caminhoLongo(caminho))
}

// caminhoLongo prefixa \\?\ para escapar do limite MAX_PATH.
func caminhoLongo(p string) string {
	if strings.HasPrefix(p, `\\?\`) {
		return p
	}
	if strings.HasPrefix(p, `\\`) { // rede: \\servidor\share -> \\?\UNC\servidor\share
		return `\\?\UNC` + p[1:]
	}
	if len(p) > 1 && p[1] == ':' {
		return `\\?\` + p
	}
	return p
}
