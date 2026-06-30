//go:build windows

package winutil

import (
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPCResult contém o resultado da medição de latência DPC.
type DPCResult struct {
	DpcTimePct    float64  `json:"dpc_time_pct"`    // % de tempo da CPU gasto em DPCs
	DpcRatePerSec float64  `json:"dpc_rate_per_sec"` // DPCs por segundo
	Avaliacao     string   `json:"avaliacao"`        // "ok" | "moderada" | "alta"
	Mensagem      string   `json:"mensagem"`
	Dicas         []string `json:"dicas,omitempty"`
}

var (
	dpcOnce        sync.Once
	dpcQuery       uintptr
	dpcTimeCounter uintptr
	dpcRateCounter uintptr
	dpcOK          bool
)

func dpcInit() {
	if pPdhOpen.Find() != nil || pPdhAddEng.Find() != nil || pPdhCollect.Find() != nil {
		return
	}
	if r, _, _ := pPdhOpen.Call(0, 0, uintptr(unsafe.Pointer(&dpcQuery))); r != 0 {
		return
	}
	pPdhClose := pdhDLL.NewProc("PdhCloseQuery")

	pathTime, e1 := windows.UTF16PtrFromString(`\Processor Information(_Total)\% DPC Time`)
	pathRate, e2 := windows.UTF16PtrFromString(`\Processor(_Total)\DPC Queued/sec`)
	if e1 != nil || e2 != nil {
		pPdhClose.Call(dpcQuery)
		return
	}

	r1, _, _ := pPdhAddEng.Call(dpcQuery, uintptr(unsafe.Pointer(pathTime)), 0, uintptr(unsafe.Pointer(&dpcTimeCounter)))
	r2, _, _ := pPdhAddEng.Call(dpcQuery, uintptr(unsafe.Pointer(pathRate)), 0, uintptr(unsafe.Pointer(&dpcRateCounter)))
	if r1 != 0 || r2 != 0 {
		pPdhClose.Call(dpcQuery)
		return
	}
	pPdhCollect.Call(dpcQuery) // 1ª amostra
	dpcOK = true
}

// dpcCounterSingle lê um único valor double de um counter PDH.
func dpcCounterSingle(counter uintptr) (float64, bool) {
	type pdhFmtCounterValueSingle struct {
		CStatus uint32
		_       uint32
		Double  float64
	}
	pPdhGetFmt := pdhDLL.NewProc("PdhGetFormattedCounterValue")
	var val pdhFmtCounterValueSingle
	r, _, _ := pPdhGetFmt.Call(counter, pdhFmtDouble, 0, uintptr(unsafe.Pointer(&val)))
	if r != 0 || val.CStatus != 0 {
		return 0, false
	}
	return val.Double, true
}

// MeasureDPC coleta 2 amostras com 1s de intervalo e retorna a latência DPC.
func MeasureDPC() DPCResult {
	dpcOnce.Do(dpcInit)

	// Se PDH não disponível, fallback via PowerShell
	if !dpcOK {
		return measureDPCPowerShell()
	}

	// 1ª coleta já foi feita no init. Aguarda 1s e coleta a 2ª.
	// Usamos runCapture com ping para aguardar sem travar o goroutine do servidor.
	runCapture("ping", "-n", "1", "-w", "1000", "127.0.0.1")
	pPdhCollect.Call(dpcQuery)

	timePct, tokTime := dpcCounterSingle(dpcTimeCounter)
	rate, tokRate := dpcCounterSingle(dpcRateCounter)

	if !tokTime && !tokRate {
		return measureDPCPowerShell()
	}

	return buildDPCResult(timePct, rate)
}

func measureDPCPowerShell() DPCResult {
	// Fallback: usa PowerShell para medir % DPC time
	out, err := runCapture("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-Counter '\Processor Information(_Total)\% DPC Time' -MaxSamples 2 -SampleInterval 1).CounterSamples | `+
			`Measure-Object -Property CookedValue -Average | Select-Object -ExpandProperty Average`)
	if err != nil {
		return DPCResult{Avaliacao: "desconhecida", Mensagem: "Não foi possível medir a latência DPC."}
	}
	val, e := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if e != nil {
		val = 0
	}
	return buildDPCResult(val, -1)
}

func buildDPCResult(dpcTimePct, dpcRate float64) DPCResult {
	res := DPCResult{DpcTimePct: round2(dpcTimePct)}
	if dpcRate >= 0 {
		res.DpcRatePerSec = round2(dpcRate)
	}

	switch {
	case dpcTimePct < 2:
		res.Avaliacao = "ok"
		res.Mensagem = "Latência DPC normal — CPU gasta menos de 2% do tempo em interrupções diferidas. Bom para áudio e jogos."
	case dpcTimePct < 5:
		res.Avaliacao = "moderada"
		res.Mensagem = "Latência DPC moderada. Pode causar micro-engasgos em áudio ou jogos em situações específicas."
		res.Dicas = dpcDicas()
	default:
		res.Avaliacao = "alta"
		res.Mensagem = "Latência DPC alta — a CPU está gastando mais de 5% do tempo em interrupções. Causa provável de engasgos, quedas de FPS e glitches de áudio."
		res.Dicas = dpcDicas()
	}
	return res
}

func dpcDicas() []string {
	return []string{
		"Atualize os drivers da GPU, rede e chipset — drivers antigos são a causa #1 de DPC alto.",
		"Desative dispositivos USB desnecessários no Gerenciador de Dispositivos.",
		"No BIOS: desative Wi-Fi e Bluetooth se estiver usando cabo de rede.",
		"Desative o recurso 'Interrupt Moderation' no driver da placa de rede (avançado nas propriedades do adaptador).",
		"Use LatencyMon (gratuito) para identificar qual driver está causando o problema.",
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
