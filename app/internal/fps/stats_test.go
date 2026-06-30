//go:build windows

package fps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeStatsConstant(t *testing.T) {
	// 100 quadros a 10ms = 100 FPS perfeitos e estaveis.
	ft := make([]float64, 100)
	for i := range ft {
		ft[i] = 10
	}
	s := computeStats("game.exe", ft, 0)
	if s.FPSAvg != 100 {
		t.Errorf("FPSAvg = %v, esperado 100", s.FPSAvg)
	}
	if s.Low1 != 100 || s.Low01 != 100 {
		t.Errorf("lows = %v/%v, esperado 100/100 (estavel)", s.Low1, s.Low01)
	}
	if s.StutterPct != 0 {
		t.Errorf("stutter = %v, esperado 0", s.StutterPct)
	}
	if s.FrameCount != 100 {
		t.Errorf("frames = %d, esperado 100", s.FrameCount)
	}
}

func TestComputeStatsWithSpikes(t *testing.T) {
	// 990 quadros a 5ms (200 FPS) + 10 quadros a 50ms (20 FPS) = engasgos.
	var ft []float64
	for i := 0; i < 990; i++ {
		ft = append(ft, 5)
	}
	for i := 0; i < 10; i++ {
		ft = append(ft, 50)
	}
	s := computeStats("game.exe", ft, 3)
	// 1% low deve refletir os quadros ruins (perto de 20 FPS), nao a media.
	if s.Low1 > 60 {
		t.Errorf("Low1 = %v, esperado refletir os engasgos (<=~60)", s.Low1)
	}
	if s.FPSAvg < 150 {
		t.Errorf("FPSAvg = %v, esperado alto (~190)", s.FPSAvg)
	}
	if s.StutterPct <= 0 {
		t.Errorf("StutterPct = %v, esperado > 0", s.StutterPct)
	}
	if s.Dropped != 3 {
		t.Errorf("Dropped = %d, esperado 3", s.Dropped)
	}
	if s.FPSMin > 25 {
		t.Errorf("FPSMin = %v, esperado ~20 (pior quadro)", s.FPSMin)
	}
}

func TestComputeStatsFiltersWarmup(t *testing.T) {
	// primeiro quadro com 5000ms (warm-up) deve ser descartado.
	ft := []float64{5000, 10, 10, 10, 10}
	s := computeStats("g.exe", ft, 0)
	if s.FrameCount != 4 {
		t.Errorf("FrameCount = %d, esperado 4 (warm-up filtrado)", s.FrameCount)
	}
}

func TestParseCSVHeaderDriven(t *testing.T) {
	// CSV sintetico no formato do PresentMon (colunas fora de ordem de proposito).
	csv := "Application,Dropped,msBetweenPresents,TimeInSeconds\n" +
		"game.exe,0,10.0,0.01\n" +
		"game.exe,1,10.0,0.02\n" + // descartado
		"game.exe,0,20.0,0.03\n" +
		"game.exe,0,10.0,0.04\n"
	p := filepath.Join(t.TempDir(), "f.csv")
	if err := os.WriteFile(p, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := parseCSV(p, "game.exe")
	if err != nil {
		t.Fatal(err)
	}
	if s.FrameCount != 3 {
		t.Errorf("FrameCount = %d, esperado 3 (1 dropped)", s.FrameCount)
	}
	if s.Dropped != 1 {
		t.Errorf("Dropped = %d, esperado 1", s.Dropped)
	}
}
