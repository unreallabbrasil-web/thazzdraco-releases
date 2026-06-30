//go:build windows

package winutil

import (
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GPUInfo: dados de uma GPU. Campos -1 = N/A (nao lido de sensor real).
type GPUInfo struct {
	Nome    string `json:"nome"`
	Vendor  string `json:"vendor"` // NVIDIA | AMD | Intel | Outro
	TempC   int    `json:"temp_c"`
	UsoPct  int    `json:"uso_pct"`
	VramUso int    `json:"vram_uso_mb"`
	VramTot int    `json:"vram_tot_mb"`
	Fonte   string `json:"fonte"`              // NVML (sensor real) | driver (so nome) | PDH
	Throttle string `json:"throttle,omitempty"` // "" desconhecido | ok | termico | potencia
	Nota    string `json:"nota,omitempty"`
}

// ---- NVIDIA (NVML — sensor real do driver) ----------------------------------

var (
	nvmlDLL      = windows.NewLazySystemDLL("nvml.dll")
	pNvmlInit    = nvmlDLL.NewProc("nvmlInit_v2")
	pNvmlCount   = nvmlDLL.NewProc("nvmlDeviceGetCount_v2")
	pNvmlHandle  = nvmlDLL.NewProc("nvmlDeviceGetHandleByIndex_v2")
	pNvmlName    = nvmlDLL.NewProc("nvmlDeviceGetName")
	pNvmlTemp    = nvmlDLL.NewProc("nvmlDeviceGetTemperature")
	pNvmlUtil    = nvmlDLL.NewProc("nvmlDeviceGetUtilizationRates")
	pNvmlMem      = nvmlDLL.NewProc("nvmlDeviceGetMemoryInfo")
	pNvmlDriver   = nvmlDLL.NewProc("nvmlSystemGetDriverVersion")
	pNvmlThrottle = nvmlDLL.NewProc("nvmlDeviceGetCurrentClocksThrottleReasons")

	gpuOnce     sync.Once
	nvmlHandles []uintptr
	nvmlNames   []string
)

func nvmlInit() {
	if pNvmlInit.Find() != nil {
		return // driver NVIDIA ausente
	}
	if r, _, _ := pNvmlInit.Call(); r != 0 {
		return
	}
	var count uint32
	pNvmlCount.Call(uintptr(unsafe.Pointer(&count)))
	for i := uint32(0); i < count; i++ {
		var h uintptr
		if r, _, _ := pNvmlHandle.Call(uintptr(i), uintptr(unsafe.Pointer(&h))); r != 0 {
			continue
		}
		buf := make([]byte, 96)
		pNvmlName.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		nvmlHandles = append(nvmlHandles, h)
		nvmlNames = append(nvmlNames, cstr(buf))
	}
}

// nvmlDriverVersion devolve a versão marketing do driver NVIDIA (ex.: "596.36").
func nvmlDriverVersion() string {
	gpuOnce.Do(nvmlInit)
	if pNvmlDriver.Find() != nil {
		return ""
	}
	buf := make([]byte, 80)
	if r, _, _ := pNvmlDriver.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r != 0 {
		return ""
	}
	return cstr(buf)
}

func cstr(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// ---- Nome da GPU (qualquer marca) via EnumDisplayDevices --------------------

type displayDeviceW struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

var procEnumDisplayDevices = user32.NewProc("EnumDisplayDevicesW")

func displayAdapters() []string {
	seen := map[string]bool{}
	var out []string
	for i := uint32(0); ; i++ {
		var dd displayDeviceW
		dd.Cb = uint32(unsafe.Sizeof(dd))
		r, _, _ := procEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0)
		if r == 0 {
			break
		}
		name := windows.UTF16ToString(dd.DeviceString[:])
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		if i > 16 {
			break
		}
	}
	return out
}

// isVirtualGPU diz se o nome e de um adaptador virtual (Parsec/TeamViewer/IDD/
// RDP/etc.). Esses ficam instalados mesmo sem o app estar rodando — nao sao GPUs
// fisicas e nao devem poluir a tela de desempenho. Usa a mesma lista do driver.go.
func isVirtualGPU(name string) bool {
	low := strings.ToLower(name)
	for _, m := range virtualGPUMarks {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

func vendorOf(name string) string {
	n := strings.ToUpper(name)
	switch {
	case strings.Contains(n, "NVIDIA"), strings.Contains(n, "GEFORCE"):
		return "NVIDIA"
	case strings.Contains(n, "AMD"), strings.Contains(n, "RADEON"):
		return "AMD"
	case strings.Contains(n, "INTEL"):
		return "Intel"
	}
	return "Outro"
}

// PrimaryGPUVendor retorna o vendor da primeira GPU física do sistema ("NVIDIA","AMD","Intel","Outro","").
func PrimaryGPUVendor() string {
	gpuOnce.Do(nvmlInit)
	if len(nvmlHandles) > 0 {
		return "NVIDIA"
	}
	for _, name := range displayAdapters() {
		if !isVirtualGPU(name) {
			return vendorOf(name)
		}
	}
	return ""
}

// GPUs devolve as GPUs do sistema. NVIDIA traz temp/uso/VRAM reais (NVML); outras
// marcas trazem o nome (temp/uso de AMD via ADL serao habilitados apos validacao
// em hardware AMD — ate la, N/A em vez de numero inventado).
func GPUs() []GPUInfo {
	gpuOnce.Do(nvmlInit)
	var out []GPUInfo
	nvSeen := false

	for i, h := range nvmlHandles {
		nvSeen = true
		g := GPUInfo{Nome: nvmlNames[i], Vendor: "NVIDIA", Fonte: "NVML", TempC: -1, UsoPct: -1, VramUso: -1, VramTot: -1}
		var t uint32
		if r, _, _ := pNvmlTemp.Call(h, 0 /*NVML_TEMPERATURE_GPU*/, uintptr(unsafe.Pointer(&t))); r == 0 {
			g.TempC = int(t)
		}
		var u struct{ G, M uint32 }
		if r, _, _ := pNvmlUtil.Call(h, uintptr(unsafe.Pointer(&u))); r == 0 {
			g.UsoPct = int(u.G)
		}
		var m struct{ Total, Free, Used uint64 }
		if r, _, _ := pNvmlMem.Call(h, uintptr(unsafe.Pointer(&m))); r == 0 {
			g.VramUso = int(m.Used / (1 << 20))
			g.VramTot = int(m.Total / (1 << 20))
		}
		// Razoes de throttling (so significativas sob carga): termico vs potencia.
		if pNvmlThrottle.Find() == nil {
			var tr uint64
			if r, _, _ := pNvmlThrottle.Call(h, uintptr(unsafe.Pointer(&tr))); r == 0 {
				const swThermal, hwThermal = 0x20, 0x40
				const swPower, hwPowerBrake = 0x04, 0x80
				switch {
				case tr&(swThermal|hwThermal) != 0:
					g.Throttle = "termico"
				case tr&(swPower|hwPowerBrake) != 0:
					g.Throttle = "potencia"
				default:
					g.Throttle = "ok"
				}
			}
		}
		out = append(out, g)
	}

	// Outras GPUs (AMD/Intel): nome via driver + USO real universal (contadores
	// PDH do Windows, qualquer marca). Temperatura continua N/A (exige sensor do
	// fabricante — não inventamos número).
	uniUso, uniOK := GpuUsageUniversal()
	for _, name := range displayAdapters() {
		if isVirtualGPU(name) {
			continue // adaptador virtual (Parsec/TeamViewer/IDD) — nao e GPU fisica
		}
		v := vendorOf(name)
		if v == "NVIDIA" && nvSeen {
			continue // ja listada com sensor real (NVML)
		}
		g := GPUInfo{Nome: name, Vendor: v, Fonte: "driver", TempC: -1, UsoPct: -1, VramUso: -1, VramTot: -1}
		if uniOK {
			g.UsoPct = uniUso // uso 3D real (PDH) — funciona em AMD/Intel
			g.Fonte = "PDH"
		}
		// F8: VRAM via PDH — funciona para AMD e Intel (WDDM)
		if vramUso, vramTot, vok := VramMB(); vok {
			g.VramUso = vramUso
			g.VramTot = vramTot
		}
		if g.TempC < 0 {
			g.Nota = "Temperatura: requer sensor do fabricante (N/A nesta marca)."
		}
		out = append(out, g)
	}
	if out == nil {
		return []GPUInfo{}
	}
	return out
}

// ---- Detalhamento de memoria (GetPerformanceInfo) ---------------------------

var procGetPerformanceInfo = psapi.NewProc("GetPerformanceInfo")

type performanceInformation struct {
	Cb                uint32
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

// MemBreakdown devolve o detalhamento de memoria (em MB) — confirma se RAM alta e
// de apps (Confirmada) ou de vazamento de driver (Pool nao-paginado alto).
func MemBreakdown() map[string]any {
	var pi performanceInformation
	pi.Cb = uint32(unsafe.Sizeof(pi))
	r, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), uintptr(pi.Cb))
	if r == 0 {
		return map[string]any{}
	}
	ps := uint64(pi.PageSize)
	mb := func(pages uintptr) int { return int(uint64(pages) * ps / (1 << 20)) }
	return map[string]any{
		"confirmada_mb":     mb(pi.CommitTotal),
		"limite_mb":         mb(pi.CommitLimit),
		"cache_mb":          mb(pi.SystemCache),
		"pool_paginado_mb":  mb(pi.KernelPaged),
		"pool_naopag_mb":    mb(pi.KernelNonpaged),
		"processos":         int(pi.ProcessCount),
		"threads":           int(pi.ThreadCount),
	}
}
