//go:build windows

package winutil

import (
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetLogicalDrives   = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW      = kernel32.NewProc("GetDriveTypeW")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
	// procGetTickCount64 e ramInfo() vivem em metrics.go (mesmo pacote).
)

const ioctlStoragePredictFailure = 0x2D1100

type DiskSpace struct {
	Letra    string  `json:"letra"`
	TotalGB  float64 `json:"total_gb"`
	LivreGB  float64 `json:"livre_gb"`
	UsadoPct int     `json:"usado_pct"`
}

type DiskHealth struct {
	Numero     int    `json:"numero"`
	Tipo       string `json:"tipo"`
	Saude      string `json:"saude"`       // Saudável | Atenção | Desconhecido
	RiscoFalha int    `json:"risco_falha"` // C3: 0=ok 1-100=risco estimado -1=desconhecido
}

type Battery struct {
	Presente         bool `json:"presente"`
	Pct              int  `json:"pct"`
	NaTomada         bool `json:"na_tomada"`
	MinutosRestantes int  `json:"minutos_restantes"`
}

type Health struct {
	Discos    []DiskSpace    `json:"discos"`
	Smart     []DiskHealth   `json:"smart"`
	RAM       map[string]any `json:"ram"`
	Bateria   *Battery       `json:"bateria"`
	UptimeMin int            `json:"uptime_min"`
	// Temperatura nao e exposta de forma confiavel pelo Windows sem driver de
	// terceiro (HWiNFO/LibreHardwareMonitor); por isso nao inventamos um valor.
	TempNota string `json:"temp_nota"`
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// BuildHealth coleta indicadores de saude do PC (tudo via API nativa).
func BuildHealth() Health {
	ramTotal, ramUsed, ramPct := ramInfo()
	h := Health{
		Discos:   diskSpaces(),
		Smart:    diskHealth(),
		RAM:      map[string]any{"total_gb": ramTotal, "usado_gb": ramUsed, "pct": ramPct},
		Bateria:  batteryInfo(),
		TempNota: "Temperatura requer sensor externo (ex.: HWiNFO).",
	}
	ms, _, _ := procGetTickCount64.Call()
	h.UptimeMin = int(uint64(ms) / 60000)
	return h
}

func diskSpaces() []DiskSpace {
	var out []DiskSpace
	mask, _, _ := procGetLogicalDrives.Call()
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letra := string(rune('A'+i)) + ":"
		root := letra + `\`
		p, _ := windows.UTF16PtrFromString(root)
		dt, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		if dt != 3 { // DRIVE_FIXED
			continue
		}
		var avail, total, free uint64
		r, _, _ := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
		if r == 0 || total == 0 {
			continue
		}
		used := total - free
		out = append(out, DiskSpace{
			Letra:    letra,
			TotalGB:  round1(float64(total) / (1 << 30)),
			LivreGB:  round1(float64(free) / (1 << 30)),
			UsadoPct: int(used * 100 / total),
		})
	}
	return out
}

type storagePredictFailure struct {
	PredictFailure uint32
	VendorSpecific [512]byte
}

func diskHealth() []DiskHealth {
	var out []DiskHealth
	for n := 0; n < 16; n++ {
		path := fmt.Sprintf(`\\.\PhysicalDrive%d`, n)
		tipo, ok := seekPenalty(path)
		if !ok {
			if n == 0 {
				continue
			}
			break
		}
		saude, risco := predictFailure(path)
		out = append(out, DiskHealth{Numero: n, Tipo: tipo, Saude: saude, RiscoFalha: risco})
	}
	return out
}

// C3: retorna (saude, riscoFalha) — risco 0=ok, 85=falha prevista, -1=desconhecido
func predictFailure(path string) (string, int) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "Desconhecido", -1
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, fileShareReadWrite, nil, openExisting, 0, 0)
	if err != nil {
		return "Desconhecido", -1
	}
	defer windows.CloseHandle(h)
	var pf storagePredictFailure
	var br uint32
	err = windows.DeviceIoControl(h, ioctlStoragePredictFailure, nil, 0,
		(*byte)(unsafe.Pointer(&pf)), uint32(unsafe.Sizeof(pf)), &br, nil)
	if err != nil {
		return "Desconhecido", -1
	}
	if pf.PredictFailure != 0 {
		return "Atenção", 85
	}
	return "Saudável", 0
}

func batteryInfo() *Battery {
	var s systemPowerStatus
	r, _, _ := procGetSysPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	if r == 0 {
		return nil
	}
	present := s.BatteryFlag != 128 && s.BatteryFlag != 255
	b := &Battery{Presente: present, NaTomada: s.ACLineStatus == 1, Pct: -1, MinutosRestantes: -1}
	if present {
		if s.BatteryLifePercent <= 100 {
			b.Pct = int(s.BatteryLifePercent)
		}
		if s.BatteryLifeTime != 0xFFFFFFFF {
			b.MinutosRestantes = int(s.BatteryLifeTime / 60)
		}
	}
	return b
}
