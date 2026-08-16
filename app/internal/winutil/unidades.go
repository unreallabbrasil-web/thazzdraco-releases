//go:build windows

package winutil

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Unidades: o que o app aceita varrer.
//
// O diagnostico (health.go) lista so DRIVE_FIXED, e ali isso esta certo: "saude
// do PC" nao fala de pendrive. Mas a tela de Disco herdou aquela lista e o
// resultado era que HD externo, pendrive e cartao SD NAO APARECIAM — justamente
// onde mais mora arquivo esquecido.
//
// Aqui a regra e outra: lista tudo que da para varrer, dizendo o que cada coisa
// e. Unidade de rede aparece marcada, porque varrer a rede e lento e o tecnico
// precisa saber disso antes de clicar, nao depois.
var (
	procGetVolumeInformationW = kernel32.NewProc("GetVolumeInformationW")
)

// tipos de GetDriveType
const (
	driveRemovivel = 2
	driveFixo      = 3
	driveRede      = 4
	driveCD        = 5
	driveRAM       = 6
)

// Unidade e uma unidade varrivel.
type Unidade struct {
	Letra    string  `json:"letra"`    // "C:"
	Rotulo   string  `json:"rotulo"`   // nome dado pelo usuário ("Jogos")
	Tipo     string  `json:"tipo"`     // fixo | removivel | rede | cd | ram
	FS       string  `json:"fs"`       // NTFS, exFAT, FAT32...
	TotalGB  float64 `json:"total_gb"`
	LivreGB  float64 `json:"livre_gb"`
	UsadoPct int     `json:"usado_pct"`
	Sistema  bool    `json:"sistema"`  // é onde o Windows está instalado
	Pronta   bool    `json:"pronta"`   // tem mídia e respondeu
	SoLeitura bool   `json:"so_leitura"`
	Aviso    string  `json:"aviso,omitempty"` // por que varrer aqui pode incomodar
}

// Unidades devolve tudo que da para varrer, na ordem em que faz sentido ler:
// sistema primeiro, depois os outros discos, depois removivel, rede por ultimo.
func Unidades() []Unidade {
	sistema := strings.ToUpper(strings.TrimSpace(os.Getenv("SystemDrive")))
	if sistema == "" {
		sistema = "C:"
	}

	var out []Unidade
	mask, _, _ := procGetLogicalDrives.Call()
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letra := string(rune('A'+i)) + ":"
		root := letra + `\`
		p, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		dt, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))

		u := Unidade{Letra: letra, Sistema: strings.EqualFold(letra, sistema)}
		switch dt {
		case driveFixo:
			u.Tipo = "fixo"
		case driveRemovivel:
			u.Tipo = "removivel"
		case driveRede:
			u.Tipo = "rede"
			u.Aviso = "unidade de rede: a varredura depende da velocidade da rede e pode demorar bem mais"
		case driveCD:
			u.Tipo = "cd"
			u.SoLeitura = true
		case driveRAM:
			u.Tipo = "ram"
		default:
			continue // DRIVE_UNKNOWN / NO_ROOT_DIR: não é nada que dê para varrer
		}

		u.Rotulo, u.FS = rotuloEFS(p)

		var avail, total, free uint64
		r, _, _ := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
		// Leitor de cartão vazio e DVD sem disco existem como letra mas não
		// respondem. Aparecem apagados na UI em vez de sumirem sem explicação —
		// o técnico plugou o pendrive e quer entender por que não está aqui.
		if r == 0 || total == 0 {
			u.Pronta = false
			if u.Aviso == "" {
				u.Aviso = "sem mídia ou sem resposta"
			}
			out = append(out, u)
			continue
		}
		u.Pronta = true
		u.TotalGB = round1(float64(total) / (1 << 30))
		u.LivreGB = round1(float64(free) / (1 << 30))
		u.UsadoPct = int((total - free) * 100 / total)
		out = append(out, u)
	}

	ordenaUnidades(out)
	return out
}

// rotuloEFS le o nome do volume e o sistema de arquivos.
func rotuloEFS(root *uint16) (rotulo, fs string) {
	var nome [261]uint16
	var fsNome [64]uint16
	var serial, maxComp, flags uint32
	r, _, _ := procGetVolumeInformationW.Call(
		uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&nome[0])), uintptr(len(nome)),
		uintptr(unsafe.Pointer(&serial)),
		uintptr(unsafe.Pointer(&maxComp)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&fsNome[0])), uintptr(len(fsNome)),
	)
	if r == 0 {
		return "", ""
	}
	return windows.UTF16ToString(nome[:]), windows.UTF16ToString(fsNome[:])
}

func ordenaUnidades(u []Unidade) {
	peso := func(x Unidade) int {
		switch {
		case x.Sistema:
			return 0
		case x.Tipo == "fixo":
			return 1
		case x.Tipo == "removivel":
			return 2
		case x.Tipo == "ram":
			return 3
		case x.Tipo == "cd":
			return 4
		default:
			return 5 // rede
		}
	}
	for i := 1; i < len(u); i++ {
		for j := i; j > 0; j-- {
			a, b := u[j-1], u[j]
			if peso(a) < peso(b) || (peso(a) == peso(b) && a.Letra <= b.Letra) {
				break
			}
			u[j-1], u[j] = b, a
		}
	}
}

// AlvoDeVarredura valida o que o cliente pediu para varrer e devolve o caminho
// normalizado.
//
// Aceita unidade ("D:", "D:\") e tambem PASTA ("E:\Jogos"), porque varrer 4 TB
// para olhar uma pasta e desperdicio. O que nao aceita: caminho relativo, caminho
// com salto de pasta e caminho que nao existe — se entrasse lixo aqui, o resto do
// modulo passaria a operar sobre um caminho que ninguem conferiu.
func AlvoDeVarredura(bruto string) (string, error) {
	c := strings.TrimSpace(bruto)
	if c == "" {
		return "", os.ErrInvalid
	}
	c = strings.Trim(c, `"`)
	if len(c) == 2 && c[1] == ':' { // "D:" sozinho é a raiz da unidade
		c += `\`
	}
	if !filepath.IsAbs(c) {
		return "", os.ErrInvalid
	}
	limpo := filepath.Clean(c)
	if strings.Contains(limpo, "..") {
		return "", os.ErrInvalid
	}
	// Raiz de unidade perde o separador no Clean ("C:\" -> "C:"); devolve.
	if vol := filepath.VolumeName(limpo); strings.EqualFold(limpo, vol) {
		limpo = vol + `\`
	}
	st, err := os.Stat(limpo)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", os.ErrInvalid
	}
	return limpo, nil
}
