//go:build windows

package winutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type BenchResult struct {
	CPUSingle float64 `json:"cpu_single"`
	CPUMulti  float64 `json:"cpu_multi"`
	Threads   int     `json:"threads"`
	MemBW     float64 `json:"mem_bw"`
	DiskWrite float64 `json:"disk_write"`
	Indice    int     `json:"indice"`
	DurMs     int64   `json:"dur_ms"`
	Aviso     string  `json:"aviso,omitempty"` // C2: mensagem se ambiente limita resultado
}

// C2: sessão RDP não tem GPU real; disk/mem podem estar mapeados via rede.
func isRDPSession() bool {
	sn := strings.ToUpper(os.Getenv("SESSIONNAME"))
	return strings.HasPrefix(sn, "RDP-") || strings.HasPrefix(sn, "ICA-")
}

// benchSink impede o compilador de eliminar o kernel de CPU como codigo morto.
var benchSink float64
var benchMu sync.Mutex

// cpuKernel roda um kernel misto (inteiro + ponto flutuante) por iters voltas
// e devolve um acumulador. Trabalho deterministico, sem alocacao, sem I/O.
func cpuKernel(iters int) float64 {
	x := 1.0
	var acc float64
	h := uint64(0x9e3779b97f4a7c15)
	for i := 0; i < iters; i++ {
		h ^= h << 13
		h ^= h >> 7
		h ^= h << 17 // xorshift (inteiro)
		x = x*1.0000000123 + 0.5
		if x > 1e6 {
			x -= 1e6
		}
		acc += x*0.5 + float64(h&0xffff)*1e-6
	}
	return acc
}

// benchCPU mede CPU em 1 thread e em todos os threads, em Mops/s.
func benchCPU() (single, multi float64, threads int) {
	const iters = 120_000_000
	threads = runtime.NumCPU()

	// 1 thread
	t0 := time.Now()
	benchSink += cpuKernel(iters)
	dt := time.Since(t0).Seconds()
	if dt > 0 {
		single = float64(iters) / dt / 1e6
	}

	// multi: um kernel por thread logico
	var wg sync.WaitGroup
	var mu sync.Mutex
	t1 := time.Now()
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := cpuKernel(iters)
			mu.Lock()
			benchSink += r
			mu.Unlock()
		}()
	}
	wg.Wait()
	dt2 := time.Since(t1).Seconds()
	if dt2 > 0 {
		multi = float64(iters) * float64(threads) / dt2 / 1e6
	}
	return
}

// benchMem mede banda de memoria (GB/s) com copia + leitura sequencial.
func benchMem() float64 {
	const n = 64 << 20 // 64 MB
	src := make([]byte, n)
	dst := make([]byte, n)
	for i := range src {
		src[i] = byte(i)
	}
	const rounds = 6
	t0 := time.Now()
	var sum uint64
	for r := 0; r < rounds; r++ {
		copy(dst, src) // memmove: 1 leitura + 1 escrita
		for i := 0; i < n; i += 4096 {
			sum += uint64(dst[i])
		}
	}
	dt := time.Since(t0).Seconds()
	benchSink += float64(sum & 0xff)
	if dt <= 0 {
		return 0
	}
	bytesMoved := float64(n) * 2 * rounds // copy move ~2x
	return bytesMoved / dt / (1 << 30)
}

// benchDisk mede a escrita sequencial (MB/s) num arquivo temporario, com
// flush real pro dispositivo (Sync = FlushFileBuffers) para nao medir cache.
// O arquivo e apagado ao final. Toca SO o %TEMP% (descartavel).
//
// Nao medimos leitura de proposito: ler logo apos escrever bateria no cache
// de pagina do Windows (RAM), nao no disco — seria um numero enganoso, e o
// projeto so mostra dado real/verificavel.
func benchDisk() float64 {
	const total = 96 << 20 // 96 MB
	const chunk = 1 << 20  // 1 MB
	buf := make([]byte, chunk)
	for i := range buf {
		buf[i] = byte(i * 7)
	}
	path := filepath.Join(os.TempDir(), "thazzdraco_bench.tmp")
	defer os.Remove(path)

	f, err := os.Create(path)
	if err != nil {
		return 0
	}
	t0 := time.Now()
	for w := 0; w < total; w += chunk {
		if _, err := f.Write(buf); err != nil {
			f.Close()
			return 0
		}
	}
	f.Sync() // forca a escrita real no dispositivo antes de medir
	dw := time.Since(t0).Seconds()
	f.Close()
	if dw <= 0 {
		return 0
	}
	return float64(total) / dw / (1 << 20)
}

func Benchmark() BenchResult {
	benchMu.Lock()
	defer benchMu.Unlock()

	var aviso string
	if isRDPSession() {
		aviso = "Sessão RDP detectada — disco e memória podem ser mapeados via rede; resultado pode ser inferior ao local."
	}

	start := time.Now()
	runtime.GC()

	single, multi, threads := benchCPU()
	memBW := benchMem()
	dw := benchDisk()

	// Indice composto (transparente): cada metrica normalizada por uma
	// referencia de PC mediano e somada. ~1000 = mediano, maior = melhor.
	const (
		refSingle = 350.0  // Mops/s
		refMulti  = 3000.0 // Mops/s
		refMem    = 16.0   // GB/s
		refDisk   = 450.0  // MB/s escrita
	)
	idx := single/refSingle*250 +
		multi/refMulti*350 +
		memBW/refMem*200 +
		dw/refDisk*200

	return BenchResult{
		CPUSingle: round1(single),
		CPUMulti:  round1(multi),
		Threads:   threads,
		MemBW:     round1(memBW),
		DiskWrite: round1(dw),
		Indice:    int(idx + 0.5),
		DurMs:     time.Since(start).Milliseconds(),
		Aviso:     aviso,
	}
}
