//go:build windows

package winutil

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// findNvidiaSmi localiza o executável nvidia-smi (incluso nos drivers NVIDIA).
func findNvidiaSmi() string {
	if p, err := exec.LookPath("nvidia-smi"); err == nil {
		return p
	}
	candidates := []string{
		`C:\Windows\System32\nvidia-smi.exe`,
		`C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// NvidiaPanel retorna os dados lidos via nvidia-smi: power limit, fan, estado.
// Retorna {"disponivel": false} se a GPU não for NVIDIA ou o nvidia-smi faltar.
func NvidiaPanel() map[string]any {
	smi := findNvidiaSmi()
	if smi == "" {
		return map[string]any{"disponivel": false}
	}
	out, err := runCapture(smi,
		"--query-gpu=name,power.limit,power.max_limit,power.default_limit,power.draw,fan.speed,pstate,temperature.gpu,clocks.gr,clocks.max.gr",
		"--format=csv,noheader,nounits")
	if err != nil || strings.TrimSpace(out) == "" {
		return map[string]any{"disponivel": false, "erro": "nvidia-smi não respondeu"}
	}

	fields := strings.SplitN(strings.TrimSpace(out), ",", 11)
	get := func(i int) string {
		if i < len(fields) {
			return strings.TrimSpace(fields[i])
		}
		return ""
	}
	parseI := func(i int) int {
		v, _ := strconv.Atoi(get(i))
		return v
	}

	nome := get(0)
	powerLim := parseI(1)
	powerMax := parseI(2)
	powerDef := parseI(3)
	powerDraw := parseI(4)
	fan := get(5)  // pode ser "[N/A]" se fan não for controlável
	pstate := get(6)
	temp := parseI(7)
	clock := parseI(8)
	clockMax := parseI(9)

	fanVal := -1
	if v, e := strconv.Atoi(fan); e == nil {
		fanVal = v
	}

	return map[string]any{
		"disponivel":      true,
		"nome":            nome,
		"power_limit_w":   powerLim,
		"power_max_w":     powerMax,
		"power_default_w": powerDef,
		"power_draw_w":    powerDraw,
		"fan_pct":         fanVal,
		"pstate":          pstate,
		"temp_c":          temp,
		"clock_mhz":       clock,
		"clock_max_mhz":   clockMax,
	}
}

// NvidiaSetPower define o limite de potência da GPU (em watts) via nvidia-smi.
// Requer privilégio de administrador.
func NvidiaSetPower(watts int) map[string]any {
	smi := findNvidiaSmi()
	if smi == "" {
		return map[string]any{"ok": false, "erro": "nvidia-smi não encontrado (driver NVIDIA não instalado?)"}
	}
	_, err := runCapture(smi, "-pl", strconv.Itoa(watts))
	if err != nil {
		return map[string]any{"ok": false, "erro": "falha ao definir power limit (precisa de administrador)"}
	}
	return map[string]any{"ok": true, "watts": watts, "mensagem": "Limite de energia configurado para " + strconv.Itoa(watts) + " W."}
}

// NvidiaResetPower restaura o limite de potência ao padrão do modelo.
func NvidiaResetPower() map[string]any {
	smi := findNvidiaSmi()
	if smi == "" {
		return map[string]any{"ok": false, "erro": "nvidia-smi não encontrado"}
	}
	// -pm 1 = Persistence Mode on; então --power-limit sem valor = restaura padrão
	// nvidia-smi --reset-gpu-clocks / --reset-power-limit
	_, err := runCapture(smi, "--reset-power-limit")
	if err != nil {
		// fallback: tenta via flag alternativa
		_, err2 := runCapture(smi, "-rpl")
		if err2 != nil {
			return map[string]any{"ok": false, "erro": "não consegui resetar o power limit"}
		}
	}
	return map[string]any{"ok": true, "mensagem": "Power limit restaurado ao padrão do modelo."}
}
