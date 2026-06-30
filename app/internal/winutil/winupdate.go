//go:build windows

package winutil

import (
	"strings"
)

// WUStatus retorna o estado atual do serviço Windows Update.
func WUStatus() map[string]any {
	out, _ := runCapture("sc", "query", "wuauserv")
	running := strings.Contains(out, "RUNNING")

	outCfg, _ := runCapture("sc", "qc", "wuauserv")
	disabled := strings.Contains(outCfg, "DISABLED")

	status := "ativo"
	if disabled {
		status = "pausado"
	} else if !running {
		status = "parado"
	}

	return map[string]any{
		"ok":         true,
		"rodando":    running,
		"desativado": disabled,
		"status":     status,
	}
}

// WUPausar para e desativa o serviço Windows Update.
func WUPausar() map[string]any {
	runCapture("sc", "stop", "wuauserv")
	runCapture("sc", "config", "wuauserv", "start=", "disabled")
	// UsoSvc = Update Orchestrator (Win10/11)
	runCapture("sc", "stop", "UsoSvc")
	runCapture("sc", "config", "UsoSvc", "start=", "disabled")

	return map[string]any{
		"ok":       true,
		"mensagem": "Windows Update pausado. Para atualizar manualmente, clique em Retomar ou vá em Configurações → Windows Update.",
		"status":   "pausado",
	}
}

// WURetomar reativa e inicia o serviço Windows Update.
func WURetomar() map[string]any {
	runCapture("sc", "config", "wuauserv", "start=", "auto")
	runCapture("sc", "start", "wuauserv")
	runCapture("sc", "config", "UsoSvc", "start=", "auto")
	runCapture("sc", "start", "UsoSvc")

	return map[string]any{
		"ok":       true,
		"mensagem": "Windows Update reativado. O sistema voltará a buscar e instalar atualizações automaticamente.",
		"status":   "ativo",
	}
}
