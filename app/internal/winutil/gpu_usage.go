//go:build windows

package winutil

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Uso de GPU universal (NVIDIA/AMD/Intel) via contadores de desempenho do Windows
// (PDH, "\GPU Engine(*)\Utilization Percentage") — a mesma fonte do Gerenciador de
// Tarefas. Funciona em qualquer placa, sem SDK do fabricante. Mantém uma query
// persistente: a 1ª amostra inicializa o contador (taxa), as seguintes valem.
var (
	pdhDLL        = windows.NewLazySystemDLL("pdh.dll")
	pPdhOpen      = pdhDLL.NewProc("PdhOpenQueryW")
	pPdhAddEng    = pdhDLL.NewProc("PdhAddEnglishCounterW")
	pPdhCollect   = pdhDLL.NewProc("PdhCollectQueryData")
	pPdhGetArray  = pdhDLL.NewProc("PdhGetFormattedCounterArrayW")

	gpuPdhOnce    sync.Once
	gpuPdhQuery   uintptr
	gpuPdhCounter uintptr
	gpuPdhOK      bool

	// F4: thermal zone (CPU temp)
	thermalOnce    sync.Once
	thermalQuery   uintptr
	thermalCounter uintptr
	thermalOK      bool

	// F8: VRAM AMD/Intel via PDH
	vramOnce        sync.Once
	vramQuery       uintptr
	vramUsedCounter uintptr
	vramTotCounter  uintptr
	vramOK          bool
)

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

type pdhFmtCounterValueDouble struct {
	CStatus uint32
	_       uint32 // padding para alinhar o double em 8
	Double  float64
}

type pdhFmtCounterValueItemW struct {
	SzName *uint16
	Value  pdhFmtCounterValueDouble
}

func gpuPdhInit() {
	// Garante que TODOS os procs do pdh.dll resolveram antes de chamar qualquer
	// um — um LazyProc nao resolvido entra em panic no .Call (simetria que faltava).
	if pPdhOpen.Find() != nil || pPdhAddEng.Find() != nil || pPdhCollect.Find() != nil || pPdhGetArray.Find() != nil {
		return
	}
	if r, _, _ := pPdhOpen.Call(0, 0, uintptr(unsafe.Pointer(&gpuPdhQuery))); r != 0 {
		return
	}
	pPdhClose := pdhDLL.NewProc("PdhCloseQuery")
	// Soma a utilizacao do motor 3D (jogos) de todos os processos/placas.
	path, err := windows.UTF16PtrFromString(`\GPU Engine(*engtype_3D)\Utilization Percentage`)
	if err != nil {
		pPdhClose.Call(gpuPdhQuery) // nao vaza o handle da query
		return
	}
	if r, _, _ := pPdhAddEng.Call(gpuPdhQuery, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&gpuPdhCounter))); r != 0 {
		pPdhClose.Call(gpuPdhQuery)
		return
	}
	pPdhCollect.Call(gpuPdhQuery) // 1ª amostra (o contador de taxa precisa de duas)
	gpuPdhOK = true
}

func thermalInit() {
	if pPdhOpen.Find() != nil || pPdhAddEng.Find() != nil || pPdhCollect.Find() != nil || pPdhGetArray.Find() != nil {
		return
	}
	if r, _, _ := pPdhOpen.Call(0, 0, uintptr(unsafe.Pointer(&thermalQuery))); r != 0 {
		return
	}
	path, err := windows.UTF16PtrFromString(`\Thermal Zone Information(*)\Temperature`)
	if err != nil {
		return
	}
	if r, _, _ := pPdhAddEng.Call(thermalQuery, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&thermalCounter))); r != 0 {
		return
	}
	pPdhCollect.Call(thermalQuery)
	thermalOK = true
}

// CPUTempC devolve a temperatura mais alta entre as zonas térmicas (°C).
// Retorna -1 se indisponível (requer driver ACPI adequado).
func CPUTempC() int {
	thermalOnce.Do(thermalInit)
	if !thermalOK {
		return -1
	}
	if r, _, _ := pPdhCollect.Call(thermalQuery); r != 0 {
		return -1
	}
	var size, count uint32
	r, _, _ := pPdhGetArray.Call(thermalCounter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != uint32(pdhMoreData) || size == 0 {
		return -1
	}
	buf := make([]byte, size)
	r, _, _ = pPdhGetArray.Call(thermalCounter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 || count == 0 {
		return -1
	}
	items := unsafe.Slice((*pdhFmtCounterValueItemW)(unsafe.Pointer(&buf[0])), int(count))
	var max float64 = -1
	for i := range items {
		if items[i].Value.CStatus != 0 {
			continue
		}
		v := items[i].Value.Double
		c := v/10.0 - 273.15 // deciKelvin → °C
		if c < 0 || c > 150 {
			c = v - 273.15 // tentar como Kelvin direto
		}
		if c >= 0 && c <= 150 && c > max {
			max = c
		}
	}
	if max < 0 {
		return -1
	}
	return int(max + 0.5)
}

// GpuUsageUniversal devolve o uso do motor 3D da GPU (0–100) somando todos os
// processos, ou (0,false) se indisponível. Vale para qualquer fabricante.
func GpuUsageUniversal() (int, bool) {
	gpuPdhOnce.Do(gpuPdhInit)
	if !gpuPdhOK {
		return 0, false
	}
	if r, _, _ := pPdhCollect.Call(gpuPdhQuery); r != 0 {
		return 0, false
	}
	var size, count uint32
	// 1ª chamada: descobre o tamanho do buffer.
	r, _, _ := pPdhGetArray.Call(gpuPdhCounter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != uint32(pdhMoreData) || size == 0 {
		return 0, false
	}
	buf := make([]byte, size)
	r, _, _ = pPdhGetArray.Call(gpuPdhCounter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 || count == 0 {
		return 0, false
	}
	items := unsafe.Slice((*pdhFmtCounterValueItemW)(unsafe.Pointer(&buf[0])), int(count))
	var sum float64
	for i := range items {
		if items[i].Value.CStatus == 0 {
			sum += items[i].Value.Double
		}
	}
	if sum < 0 {
		sum = 0
	} else if sum > 100 {
		sum = 100
	}
	return int(sum + 0.5), true
}

// ---- F8: VRAM AMD/Intel via PDH ---------------------------------------------
// Contadores de memória do adaptador (Windows WDDM — funciona para AMD e Intel).

func vramInit() {
	if pPdhOpen.Find() != nil || pPdhAddEng.Find() != nil || pPdhCollect.Find() != nil || pPdhGetArray.Find() != nil {
		return
	}
	if r, _, _ := pPdhOpen.Call(0, 0, uintptr(unsafe.Pointer(&vramQuery))); r != 0 {
		return
	}
	pPdhClose := pdhDLL.NewProc("PdhCloseQuery")
	pathUsed, e1 := windows.UTF16PtrFromString(`\GPU Adapter Memory(*)\Dedicated Usage`)
	pathTot, e2 := windows.UTF16PtrFromString(`\GPU Adapter Memory(*)\Total Committed`)
	if e1 != nil || e2 != nil {
		pPdhClose.Call(vramQuery)
		return
	}
	r1, _, _ := pPdhAddEng.Call(vramQuery, uintptr(unsafe.Pointer(pathUsed)), 0, uintptr(unsafe.Pointer(&vramUsedCounter)))
	r2, _, _ := pPdhAddEng.Call(vramQuery, uintptr(unsafe.Pointer(pathTot)), 0, uintptr(unsafe.Pointer(&vramTotCounter)))
	if r1 != 0 || r2 != 0 {
		pPdhClose.Call(vramQuery)
		return
	}
	pPdhCollect.Call(vramQuery)
	vramOK = true
}

func pdhSumCounter(counter uintptr) (int, bool) {
	var size, count uint32
	r, _, _ := pPdhGetArray.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != uint32(pdhMoreData) || size == 0 {
		return 0, false
	}
	buf := make([]byte, size)
	r, _, _ = pPdhGetArray.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 || count == 0 {
		return 0, false
	}
	items := unsafe.Slice((*pdhFmtCounterValueItemW)(unsafe.Pointer(&buf[0])), int(count))
	var sum float64
	for i := range items {
		if items[i].Value.CStatus == 0 {
			sum += items[i].Value.Double
		}
	}
	return int(sum / (1 << 20)), true // bytes → MB
}

// VramMB retorna (usadoMB, totalMB, ok). Funciona para AMD e Intel via WDDM.
func VramMB() (int, int, bool) {
	vramOnce.Do(vramInit)
	if !vramOK {
		return 0, 0, false
	}
	if r, _, _ := pPdhCollect.Call(vramQuery); r != 0 {
		return 0, 0, false
	}
	used, uok := pdhSumCounter(vramUsedCounter)
	tot, tok := pdhSumCounter(vramTotCounter)
	if !uok && !tok {
		return 0, 0, false
	}
	return used, tot, true
}
