//go:build windows

package winutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GPUDriver descreve o driver de uma placa de vídeo real (dado lido do registro;
// nada estimado). Princípio firmado: informar versão/data, sem alarme de
// "desatualizado".
type GPUDriver struct {
	Nome       string `json:"nome"`
	Vendor     string `json:"vendor"`
	Dedicada   bool   `json:"dedicada"`
	Versao     string `json:"versao"`      // marketing (NVIDIA/NVML) ou versão do driver Windows
	Data       string `json:"data"`        // dd/mm/aaaa
	IdadeMeses int    `json:"idade_meses"` // -1 se desconhecida
}

const displayClassKey = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

// adaptadores virtuais (não são GPUs físicas) — TeamViewer/Parsec/IDD/etc.
var virtualGPUMarks = []string{"virtual", "basic", "remote", "meta ", "parsec", "idd",
	"mirror", "citrix", "vmware", "oray", "sunshine", "spacedesk", "rdp", "render only"}

// GPUDrivers devolve os drivers das GPUs físicas (NVIDIA/AMD/Intel), ignorando
// adaptadores virtuais.
func GPUDrivers() []GPUDriver {
	var out []GPUDriver
	subs, _ := ListSubkeys("HKLM", "", false, displayClassKey)
	for _, s := range subs {
		if len(s) != 4 || !allDigits(s) {
			continue // só 0000, 0001... (ignora Properties/Configuration)
		}
		path := displayClassKey + `\` + s
		desc, _ := ReadString("HKLM", "", false, path, "DriverDesc")
		if desc == "" {
			continue
		}
		low := strings.ToLower(desc)
		skip := false
		for _, m := range virtualGPUMarks {
			if strings.Contains(low, m) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		ver, _ := ReadString("HKLM", "", false, path, "DriverVersion")
		date, _ := ReadString("HKLM", "", false, path, "DriverDate")
		vendor := vendorOf(desc)
		d := GPUDriver{
			Nome: desc, Vendor: vendor,
			Versao: ver, Data: fmtDriverDate(date), IdadeMeses: driverAgeMonths(date),
			Dedicada: vendor == "NVIDIA" || vendor == "AMD",
		}
		if vendor == "NVIDIA" {
			if mk := nvmlDriverVersion(); mk != "" {
				d.Versao = mk // versão marketing (ex.: 596.36)
			}
		}
		out = append(out, d)
	}
	if out == nil {
		return []GPUDriver{}
	}
	return out
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// fmtDriverDate converte "M-D-YYYY" do registro para "dd/mm/aaaa".
func fmtDriverDate(s string) string {
	m, d, y, ok := parseDriverDate(s)
	if !ok {
		return ""
	}
	return strconv.Itoa(d) + "/" + strconv.Itoa(m) + "/" + strconv.Itoa(y)
}

func parseDriverDate(s string) (month, day, year int, ok bool) {
	p := strings.Split(strings.TrimSpace(s), "-")
	if len(p) != 3 {
		return 0, 0, 0, false
	}
	month, e1 := strconv.Atoi(p[0])
	day, e2 := strconv.Atoi(p[1])
	year, e3 := strconv.Atoi(p[2])
	if e1 != nil || e2 != nil || e3 != nil || year < 1990 {
		return 0, 0, 0, false
	}
	return month, day, year, true
}

// driverAgeMonths devolve a idade do driver em meses (-1 se desconhecida).
func driverAgeMonths(s string) int {
	m, d, y, ok := parseDriverDate(s)
	if !ok {
		return -1
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.Local)
	months := int(time.Since(t).Hours() / 24 / 30.44)
	if months < 0 {
		return 0
	}
	return months
}

// ---- F12: Auditoria de drivers (GPU, Áudio, Rede, Chipset) ------------------

// DriverInfo descreve um driver de hardware auditado.
type DriverInfo struct {
	Nome       string `json:"nome"`
	Cat        string `json:"cat"`
	Versao     string `json:"versao"`
	Data       string `json:"data"`
	IdadeMeses int    `json:"idade_meses"`
	Status     string `json:"status"` // ok | antigo | muito_antigo | desconhecido
}

var driverClasses = []struct {
	guid string
	cat  string
}{
	{`{4d36e968-e325-11ce-bfc1-08002be10318}`, "GPU"},
	{`{4d36e96c-e325-11ce-bfc1-08002be10318}`, "Áudio"},
	{`{4d36e972-e325-11ce-bfc1-08002be10318}`, "Rede"},
	{`{4d36e97d-e325-11ce-bfc1-08002be10318}`, "Chipset"},
}

// DriversAudit varre GPU, Áudio, Rede e Chipset e retorna cada driver com sua
// idade calculada. Status: ok (<12m), antigo (12-24m), muito_antigo (>24m).
func DriversAudit() []DriverInfo {
	seen := map[string]bool{}
	var out []DriverInfo
	for _, cls := range driverClasses {
		classKey := `SYSTEM\CurrentControlSet\Control\Class\` + cls.guid
		subs, _ := ListSubkeys("HKLM", "", false, classKey)
		for _, s := range subs {
			if len(s) != 4 || !allDigits(s) {
				continue
			}
			path := classKey + `\` + s
			desc, _ := ReadString("HKLM", "", false, path, "DriverDesc")
			if desc == "" {
				continue
			}
			low := strings.ToLower(desc)
			skip := false
			for _, m := range virtualGPUMarks {
				if strings.Contains(low, m) {
					skip = true
					break
				}
			}
			if skip || seen[desc] {
				continue
			}
			seen[desc] = true
			ver, _ := ReadString("HKLM", "", false, path, "DriverVersion")
			date, _ := ReadString("HKLM", "", false, path, "DriverDate")
			age := driverAgeMonths(date)
			status := "desconhecido"
			if age >= 0 {
				switch {
				case age >= 24:
					status = "muito_antigo"
				case age >= 12:
					status = "antigo"
				default:
					status = "ok"
				}
			}
			out = append(out, DriverInfo{
				Nome: desc, Cat: cls.cat,
				Versao: ver, Data: fmtDriverDate(date),
				IdadeMeses: age, Status: status,
			})
		}
	}
	if out == nil {
		return []DriverInfo{}
	}
	return out
}

// ---- Resíduos de instalador de driver (limpeza segura) ----------------------

// DriverLeftovers procura pastas de EXTRAÇÃO do instalador (puro lixo após a
// instalação) — NUNCA o DriverStore ativo. Devolve (pasta, MB) ou ("",0).
func DriverLeftovers() (string, int) {
	for _, c := range []string{`C:\NVIDIA`, `C:\AMD`} {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			if mb := dirSizeMB(c); mb > 0 {
				return c, mb
			}
		}
	}
	return "", 0
}

// CleanDriverLeftovers apaga a pasta de extração do instalador (seguro: é temp
// de instalação, recriado no próximo install). Só aceita C:\NVIDIA ou C:\AMD.
func CleanDriverLeftovers(folder string) error {
	folder = filepath.Clean(folder)
	if !strings.EqualFold(folder, `C:\NVIDIA`) && !strings.EqualFold(folder, `C:\AMD`) {
		return os.ErrInvalid
	}
	return os.RemoveAll(folder)
}

func dirSizeMB(root string) int {
	var total int64
	// C5: pula symlinks e reparse points (junctions) para evitar dupla contagem
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return filepath.SkipDir // nunca seguir junction/symlink filho
		}
		if !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return int(total / (1 << 20))
}
