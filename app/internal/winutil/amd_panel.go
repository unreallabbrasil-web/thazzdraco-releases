//go:build windows

package winutil

import (
	"encoding/json"
	"strings"
)

// AMDPanelInfo contém os dados do painel AMD obtidos via WMI.
type AMDPanelInfo struct {
	Disponivel    bool   `json:"disponivel"`
	Nome          string `json:"nome,omitempty"`
	VramMB        int    `json:"vram_mb,omitempty"`
	DriverVersion string `json:"driver_version,omitempty"`
	ResolucaoW    int    `json:"resolucao_w,omitempty"`
	ResolucaoH    int    `json:"resolucao_h,omitempty"`
	RefreshHz     int    `json:"refresh_hz,omitempty"`
	Erro          string `json:"erro,omitempty"`
}

// wmiGPU é a estrutura que recebe o JSON do PowerShell WMI query.
type wmiGPU struct {
	Name                     string  `json:"Name"`
	AdapterRAM               float64 `json:"AdapterRAM"` // bytes (float64 porque PS serializa como Number)
	DriverVersion            string  `json:"DriverVersion"`
	CurrentRefreshRate       float64 `json:"CurrentRefreshRate"`
	CurrentHorizontalResolution float64 `json:"CurrentHorizontalResolution"`
	CurrentVerticalResolution   float64 `json:"CurrentVerticalResolution"`
}

// AMDPanel consulta a GPU AMD/Radeon via WMI (PowerShell one-liner).
// Retorna {disponivel:false} se nenhuma GPU AMD for encontrada.
func AMDPanel() AMDPanelInfo {
	// Consulta WMI — uma chamada de ~300ms, aceitável para um painel estático.
	out, err := runCapture("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Get-WmiObject Win32_VideoController | Where-Object {`+
			`$_.Name -like '*AMD*' -or $_.Name -like '*Radeon*' -or $_.Name -like '*RX *'} | `+
			`Select-Object -First 1 Name,AdapterRAM,DriverVersion,CurrentRefreshRate,`+
			`CurrentHorizontalResolution,CurrentVerticalResolution | ConvertTo-Json -Compress`)

	if err != nil || strings.TrimSpace(out) == "" {
		return AMDPanelInfo{Disponivel: false}
	}

	// Remove BOM ou lixo antes do JSON
	out = strings.TrimSpace(out)
	if idx := strings.Index(out, "{"); idx > 0 {
		out = out[idx:]
	}

	var g wmiGPU
	if err := json.Unmarshal([]byte(out), &g); err != nil || g.Name == "" {
		return AMDPanelInfo{Disponivel: false}
	}

	vramMB := int(g.AdapterRAM / (1 << 20))
	// Algumas GPUs AMD reportam AdapterRAM incorreto (4GB cap antigo). Se < 256MB, ignora.
	if vramMB < 256 {
		vramMB = 0
	}

	return AMDPanelInfo{
		Disponivel:    true,
		Nome:          g.Name,
		VramMB:        vramMB,
		DriverVersion: g.DriverVersion,
		RefreshHz:     int(g.CurrentRefreshRate),
		ResolucaoW:    int(g.CurrentHorizontalResolution),
		ResolucaoH:    int(g.CurrentVerticalResolution),
	}
}
