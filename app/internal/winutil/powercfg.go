//go:build windows

package winutil

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// CREATE_NO_WINDOW evita que uma janela de console pisque ao chamar powercfg.exe
// (importante no build sem console / janela-app).
const createNoWindow = 0x08000000

var (
	reGUID = regexp.MustCompile(`[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}`)
	reHex  = regexp.MustCompile(`0x[0-9a-fA-F]+`)
)

// RunPowercfg executa powercfg.exe sem abrir janela e devolve a saida combinada.
func RunPowercfg(args ...string) (string, error) {
	cmd := exec.Command("powercfg", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ActiveScheme retorna o GUID do plano de energia ativo.
func ActiveScheme() string {
	out, _ := RunPowercfg("/getactivescheme")
	return reGUID.FindString(out)
}

// ActiveSchemeName retorna o NOME do plano ativo (ex.: "High performance",
// "Alto desempenho", "Ultimate Performance"). Vem entre parênteses na saída.
func ActiveSchemeName() string {
	out, _ := RunPowercfg("/getactivescheme")
	if i := strings.IndexByte(out, '('); i >= 0 {
		if j := strings.IndexByte(out[i:], ')'); j > 0 {
			return strings.TrimSpace(out[i+1 : i+j])
		}
	}
	return ""
}

// FindSchemeGUIDByName procura um plano cujo NOME contenha alguma palavra-chave
// (ex.: "ultimate", "máximo") e devolve o GUID dele. "" se não achar.
func FindSchemeGUIDByName(keywords ...string) string {
	out, _ := RunPowercfg("/list")
	for _, line := range strings.Split(out, "\n") {
		low := strings.ToLower(line)
		for _, k := range keywords {
			if strings.Contains(low, strings.ToLower(k)) {
				return reGUID.FindString(line)
			}
		}
	}
	return ""
}

// PowercfgAcValue le o valor AC (na tomada) ATUAL de uma configuracao do plano.
// A saida do "powercfg /q" lista primeiro os "Possible Settings" (Min/Max/
// incremento) e SO NO FIM os indices atuais: "...AC Power Setting Index" e
// depois "...DC Power Setting Index". Logo o valor AC atual e o PENULTIMO hex
// (independe do idioma do Windows). Pegar o primeiro hex pegava o "Minimo".
func PowercfgAcValue(sub, setting string) (int64, bool) {
	out, _ := RunPowercfg("/q", "SCHEME_CURRENT", sub, setting)
	m := reHex.FindAllString(out, -1)
	if len(m) == 0 {
		return 0, false
	}
	idx := len(m) - 2 // penultimo = AC atual; ultimo = DC atual
	if idx < 0 {
		idx = 0
	}
	v, err := strconv.ParseInt(strings.TrimPrefix(m[idx], "0x"), 16, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
