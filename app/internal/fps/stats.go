//go:build windows

package fps

import "sort"

// FrameStats e o resultado de uma captura de FPS de um jogo. Todos os numeros
// sao derivados de quadros reais medidos pelo PresentMon (ETW), sem estimativa.
type FrameStats struct {
	Processo   string    `json:"processo"`
	FPSAvg     float64   `json:"fps_avg"`     // FPS medio
	Low1       float64   `json:"low1"`        // 1% low (percentil 99 do frametime)
	Low01      float64   `json:"low01"`       // 0.1% low (percentil 99.9)
	FPSMin     float64   `json:"fps_min"`     // pior quadro (frametime maximo)
	FPSMax     float64   `json:"fps_max"`     // melhor quadro
	FrameCount int       `json:"frames"`      // quadros validos medidos
	Dropped    int       `json:"dropped"`     // quadros descartados (nao exibidos)
	DurationS  float64   `json:"duracao_s"`   // duracao medida
	StutterPct float64   `json:"stutter_pct"` // % de quadros > 2x a mediana (engasgo)
	Frametimes []float64 `json:"frametimes"`  // serie (ms) reamostrada p/ o grafico
}

// computeStats recebe os frametimes brutos (ms, em ordem temporal) e o numero
// de quadros descartados, e devolve as estatisticas profissionais.
func computeStats(proc string, raw []float64, dropped int) FrameStats {
	// Filtra invalidos: frametime <= 0 ou absurdo (>1000ms = <1 FPS, ex.: o
	// primeiro quadro/warm-up que carrega o delta desde o inicio da sessao).
	ft := make([]float64, 0, len(raw))
	for _, v := range raw {
		if v > 0 && v <= 1000 {
			ft = append(ft, v)
		}
	}
	st := FrameStats{Processo: proc, FrameCount: len(ft), Dropped: dropped}
	if len(ft) == 0 {
		return st
	}

	// Duracao e FPS medio (soma dos frametimes = tempo decorrido medido).
	var total float64
	for _, v := range ft {
		total += v
	}
	st.DurationS = round2(total / 1000)
	st.FPSAvg = round1(1000 * float64(len(ft)) / total)

	// Ordena uma copia para extremos e os "lows".
	sorted := append([]float64(nil), ft...)
	sort.Float64s(sorted) // crescente: frametimes menores = FPS maiores
	st.FPSMax = round1(1000 / sorted[0])
	st.FPSMin = round1(1000 / sorted[len(sorted)-1])
	// 1% low / 0.1% low = MEDIA dos piores 1% (e 0.1%) dos quadros — a definicao
	// que os reviewers/gamers usam (mais representativa do engasgo do que um
	// quadro isolado). Quadros mais lentos ficam no fim do slice ordenado.
	st.Low1 = round1(1000 / worstMean(sorted, 0.01))
	st.Low01 = round1(1000 / worstMean(sorted, 0.001))

	// Engasgo: quadros acima de 2x a mediana do frametime.
	med := percentile(sorted, 0.50)
	stut := 0
	for _, v := range ft {
		if v > 2*med {
			stut++
		}
	}
	st.StutterPct = round1(100 * float64(stut) / float64(len(ft)))

	st.Frametimes = downsample(ft, 240)
	return st
}

// percentile devolve o valor no percentil p (0..1) de um slice JA ORDENADO.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p*float64(len(sorted)-1) + 0.5)
	if idx < 0 {
		idx = 0
	} else if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// worstMean devolve a media dos piores frac (0..1) frametimes de um slice
// ordenado de forma CRESCENTE (os piores = maiores ficam no fim).
func worstMean(sorted []float64, frac float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	k := int(float64(n)*frac + 0.5)
	if k < 1 {
		k = 1
	}
	var s float64
	for i := n - k; i < n; i++ {
		s += sorted[i]
	}
	return s / float64(k)
}

// downsample reduz a serie para no maximo max pontos, fazendo media por balde,
// preservando a forma do grafico de frametime.
func downsample(ft []float64, max int) []float64 {
	if len(ft) <= max {
		out := make([]float64, len(ft))
		for i, v := range ft {
			out[i] = round2(v)
		}
		return out
	}
	out := make([]float64, max)
	bucket := float64(len(ft)) / float64(max)
	for i := 0; i < max; i++ {
		lo := int(float64(i) * bucket)
		hi := int(float64(i+1) * bucket)
		if hi > len(ft) {
			hi = len(ft)
		}
		if hi <= lo {
			hi = lo + 1
		}
		var s float64
		for j := lo; j < hi; j++ {
			s += ft[j]
		}
		out[i] = round2(s / float64(hi-lo))
	}
	return out
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
