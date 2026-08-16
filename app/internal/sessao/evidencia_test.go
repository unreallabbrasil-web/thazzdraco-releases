package sessao

import (
	"strings"
	"testing"

	"thazzdraco/internal/fps"
	"thazzdraco/internal/winutil"
)

// Este é o teste que protege a promessa central do produto: o ganho é MEDIDO.
// Um antes×depois montado sobre cenários diferentes seria pior que não medir —
// daria ao cliente um número com cara de prova.

func evFPS(exe, jogo, res string, hz, dur int, avg, low1 float64) *Evidencia {
	return &Evidencia{FPS: &MedidaFPS{
		Cenario: Cenario{Exe: exe, Jogo: jogo, Resolucao: res, RefreshHz: hz, DuracaoS: dur},
		Res:     fps.FrameStats{Processo: exe, FPSAvg: avg, Low1: low1, DurationS: float64(dur)},
	}}
}

func evBench(indice int, single float64) *Evidencia {
	return &Evidencia{Bench: &MedidaBench{
		Res: winutil.BenchResult{Indice: indice, CPUSingle: single, CPUMulti: 3000, MemBW: 20, DiskWrite: 500},
	}}
}

func acha(b *BlocoComparacao, rotulo string) *Delta {
	for i := range b.Metricas {
		if b.Metricas[i].Rotulo == rotulo {
			return &b.Metricas[i]
		}
	}
	return nil
}

func TestJogoDiferenteNaoEComparacao(t *testing.T) {
	a := evFPS("valorant.exe", "Valorant", "1920x1080", 165, 30, 240, 180)
	b := evFPS("fortnite.exe", "Fortnite", "1920x1080", 165, 30, 300, 220)
	c := Comparar(a, b)
	if c.FPS == nil {
		t.Fatal("com FPS dos dois lados tem que haver bloco de FPS")
	}
	if c.FPS.Comparavel {
		t.Fatal("jogos diferentes não podem ser comparados — isso viraria um ganho inventado")
	}
	if !strings.Contains(c.FPS.Motivo, "Valorant") || !strings.Contains(c.FPS.Motivo, "Fortnite") {
		t.Errorf("o motivo tem que dizer o que mudou; veio %q", c.FPS.Motivo)
	}
	if len(c.FPS.Metricas) != 0 {
		t.Error("cenário incomparável não pode devolver métricas — a UI mostraria os números assim mesmo")
	}
}

func TestResolucaoDiferenteNaoEComparacao(t *testing.T) {
	a := evFPS("valorant.exe", "Valorant", "1920x1080", 165, 30, 240, 180)
	b := evFPS("valorant.exe", "Valorant", "2560x1440", 165, 30, 190, 150)
	c := Comparar(a, b)
	if c.FPS.Comparavel {
		t.Fatal("resolução diferente é outro trabalho para a GPU")
	}
	if !strings.Contains(c.FPS.Motivo, "resolução") {
		t.Errorf("motivo = %q", c.FPS.Motivo)
	}
}

func TestRefreshDiferenteComparaEVirouObservacao(t *testing.T) {
	// A taxa mudou porque a sessão corrigiu isso — é o ganho, não um impedimento.
	a := evFPS("valorant.exe", "Valorant", "1920x1080", 120, 30, 200, 150)
	b := evFPS("valorant.exe", "Valorant", "1920x1080", 165, 30, 240, 180)
	c := Comparar(a, b)
	if !c.FPS.Comparavel {
		t.Fatalf("mesma máquina, mesmo jogo, mesma resolução: tem que comparar. Motivo dado: %q", c.FPS.Motivo)
	}
	if !strings.Contains(c.FPS.Observacao, "120 Hz") || !strings.Contains(c.FPS.Observacao, "165 Hz") {
		t.Errorf("a mudança de Hz tem que aparecer como observação; veio %q", c.FPS.Observacao)
	}
	if d := acha(c.FPS, "FPS médio"); d == nil || d.Pct != 20 || !d.Melhorou {
		t.Errorf("FPS médio 200→240 devia ser +20%% e melhoria; veio %+v", d)
	}
}

func TestDuracaoDiferenteVirouRessalva(t *testing.T) {
	a := evFPS("cs2.exe", "CS2", "1920x1080", 240, 10, 300, 200)
	b := evFPS("cs2.exe", "CS2", "1920x1080", 240, 60, 310, 210)
	c := Comparar(a, b)
	if !c.FPS.Comparavel {
		t.Fatal("duração diferente não invalida a comparação, só reduz a confiança")
	}
	if !strings.Contains(c.FPS.Observacao, "confiança") {
		t.Errorf("observação = %q", c.FPS.Observacao)
	}
}

func TestSoUmLadoMedidoDizOQueFalta(t *testing.T) {
	a := evFPS("cs2.exe", "CS2", "1920x1080", 240, 30, 300, 200)
	c := Comparar(a, nil)
	if c.FPS.Comparavel {
		t.Fatal("sem o depois não há comparação")
	}
	if !strings.Contains(c.FPS.Motivo, "depois") {
		t.Errorf("motivo = %q, queria explicar que falta medir de novo", c.FPS.Motivo)
	}
	if !c.TemBaseline || c.TemDepois {
		t.Errorf("flags = baseline %v / depois %v", c.TemBaseline, c.TemDepois)
	}

	c2 := Comparar(nil, a)
	if !strings.Contains(c2.FPS.Motivo, "não houve") {
		t.Errorf("sem baseline o motivo tem que dizer isso; veio %q", c2.FPS.Motivo)
	}
}

func TestRuidoNaoEGanho(t *testing.T) {
	a := evBench(1000, 350)
	b := evBench(1004, 350.7) // +0,4%: dentro do ruído de medição
	c := Comparar(a, b)
	d := acha(c.Bench, "Índice")
	if d == nil {
		t.Fatal("faltou o índice")
	}
	if !d.Igual {
		t.Errorf("+%.1f%% tinha que ser tratado como igual, não como ganho", d.Pct)
	}
	if d.Melhorou {
		t.Error("anunciar ruído como melhoria é exatamente o que o produto não faz")
	}
}

func TestPioraEReconhecidaComoPiora(t *testing.T) {
	a := evBench(1000, 350)
	b := evBench(880, 300)
	c := Comparar(a, b)
	d := acha(c.Bench, "Índice")
	if d.Melhorou || d.Igual {
		t.Errorf("1000 → 880 é piora; veio %+v", d)
	}
	if d.Pct != -12 {
		t.Errorf("pct = %v, queria -12", d.Pct)
	}
}

func TestEngasgoMenorEMelhor(t *testing.T) {
	a := evFPS("cs2.exe", "CS2", "1920x1080", 240, 30, 300, 200)
	b := evFPS("cs2.exe", "CS2", "1920x1080", 240, 30, 300, 200)
	a.FPS.Res.StutterPct, b.FPS.Res.StutterPct = 8, 3
	c := Comparar(a, b)
	d := acha(c.FPS, "Engasgo")
	if d == nil {
		t.Fatal("faltou o engasgo")
	}
	if !d.Melhorou {
		t.Error("engasgo caindo de 8%% para 3%% é melhoria — a métrica é invertida")
	}
	if d.MaiorMelhor {
		t.Error("engasgo é das métricas em que menor é melhor")
	}
}

func TestGuardarPreencheOQueFaltaNoCenario(t *testing.T) {
	s := Nova("PC", Cliente{}, "equilibrado", true, nil)
	s.GuardarFPS("baseline", fps.FrameStats{Processo: "cs2.exe", FPSAvg: 300, DurationS: 29.6}, Cenario{Resolucao: "1920x1080"})
	m := s.Baseline.FPS
	if m == nil {
		t.Fatal("não guardou")
	}
	if m.Cenario.Exe != "cs2.exe" {
		t.Errorf("exe do cenário = %q — devia vir da própria captura", m.Cenario.Exe)
	}
	if m.Cenario.DuracaoS != 30 {
		t.Errorf("duração = %d, queria 30 (arredondada da captura)", m.Cenario.DuracaoS)
	}
	if m.Quando == "" {
		t.Error("medição sem carimbo de hora não serve para relatório")
	}

	s.GuardarBench("depois", winutil.BenchResult{Indice: 1200})
	if s.Depois == nil || s.Depois.Bench == nil {
		t.Fatal("bench do depois não foi guardado")
	}
	if !s.Comparacao().TemDepois {
		t.Error("a sessão devia reconhecer que já há medição do depois")
	}
}
