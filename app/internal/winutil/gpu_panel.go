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

	// Só a PRIMEIRA linha (GPU primária). Com 2+ placas, o nvidia-smi emite uma
	// linha CSV por GPU; usar a saída inteira misturava o campo 9 de uma linha com
	// o nome da próxima e zerava o clock.
	first := strings.TrimSpace(out)
	if i := strings.IndexAny(first, "\r\n"); i >= 0 {
		first = strings.TrimSpace(first[:i])
	}
	fields := strings.SplitN(first, ",", 11)
	get := func(i int) string {
		if i < len(fields) {
			return strings.TrimSpace(fields[i])
		}
		return ""
	}
	// parseI devolve -1 quando o campo não é numérico (ex.: "[N/A]" em power.draw de
	// GPUs mobile) — -1 = N/A honesto, em vez de fingir 0 W / 0 °C.
	parseI := func(i int) int {
		v, err := strconv.Atoi(get(i))
		if err != nil {
			return -1
		}
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
// Requer privilégio de administrador. Valida o valor contra o range real da
// placa (via NvidiaPanel) antes de repassar pro driver — nvidia-smi rejeita
// valores fora da faixa, mas com uma mensagem de erro pouco amigável.
func NvidiaSetPower(watts int) map[string]any {
	smi := findNvidiaSmi()
	if smi == "" {
		return map[string]any{"ok": false, "erro": "nvidia-smi não encontrado (driver NVIDIA não instalado?)"}
	}
	if watts <= 0 {
		return map[string]any{"ok": false, "erro": "valor de watts inválido"}
	}
	if panel := NvidiaPanel(); panel["disponivel"] == true {
		if max, ok := panel["power_max_w"].(int); ok && max > 0 && watts > max {
			return map[string]any{"ok": false, "erro": "watts acima do máximo suportado pela placa (" + strconv.Itoa(max) + " W)"}
		}
	}
	_, err := runCapture(smi, "-pl", strconv.Itoa(watts))
	if err != nil {
		return map[string]any{"ok": false, "erro": "falha ao definir power limit (precisa de administrador)"}
	}
	return map[string]any{"ok": true, "watts": watts, "mensagem": "Limite de energia configurado para " + strconv.Itoa(watts) + " W."}
}

// NvidiaResetPower restaura o limite de potência ao padrão do modelo. O nvidia-smi
// NÃO tem flag de "reset" de power limit (--reset-power-limit/-rpl não existem);
// o reset correto é reescrever o limite com o valor padrão da placa (-pl <default>),
// que o próprio NvidiaPanel já lê em power.default_limit.
func NvidiaResetPower() map[string]any {
	smi := findNvidiaSmi()
	if smi == "" {
		return map[string]any{"ok": false, "erro": "nvidia-smi não encontrado"}
	}
	panel := NvidiaPanel()
	def, ok := panel["power_default_w"].(int)
	if !ok || def <= 0 {
		return map[string]any{"ok": false, "erro": "não foi possível obter o power limit padrão da placa"}
	}
	if _, err := runCapture(smi, "-pl", strconv.Itoa(def)); err != nil {
		return map[string]any{"ok": false, "erro": "não consegui resetar o power limit (precisa de administrador)"}
	}
	return map[string]any{"ok": true, "watts": def, "mensagem": "Power limit restaurado ao padrão do modelo (" + strconv.Itoa(def) + " W)."}
}
