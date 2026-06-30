//go:build windows

package winutil

import (
	"regexp"
	"strconv"
	"strings"
)

// PingResult contém as estatísticas de latência para um host.
type PingResult struct {
	Host      string `json:"host"`
	Enviados  int    `json:"enviados"`
	Recebidos int    `json:"recebidos"`
	Perdidos  int    `json:"perdidos"`
	PctPerda  int    `json:"pct_perda"`
	MinMs     int    `json:"min_ms"`
	MaxMs     int    `json:"max_ms"`
	AvgMs     int    `json:"avg_ms"`
	Jitter    int    `json:"jitter"`
	Erro      string `json:"erro,omitempty"`
}

// PingPreset é um alvo de latência pré-configurado.
type PingPreset struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`
	Host string `json:"host"`
}

// reLoss captura a porcentagem de perda tanto em inglês quanto em português.
var reLoss = regexp.MustCompile(`\((\d+)%\s*(?:loss|de perda)\)`)

// PingHost executa ping -n 5 no host e retorna as estatísticas.
func PingHost(host string) PingResult {
	res := PingResult{Host: host, Enviados: 5}
	out, _ := runCapture("ping", "-n", "5", host)

	if m := reLoss.FindStringSubmatch(out); len(m) == 2 {
		if pct, e := strconv.Atoi(m[1]); e == nil {
			res.PctPerda = pct
			res.Perdidos = (pct * res.Enviados) / 100
			res.Recebidos = res.Enviados - res.Perdidos
		}
	} else {
		res.Recebidos = res.Enviados
	}

	// Extrair min/max/avg da linha de estatísticas
	// Formato inglês:  Minimum = 1ms, Maximum = 2ms, Average = 1ms
	// Formato PT-BR:   Mínimo = 1ms, Máximo = 2ms, Média = 1ms
	for _, line := range strings.Split(out, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "ms,") && !strings.Contains(lower, "ms") {
			continue
		}
		re := regexp.MustCompile(`(\d+)ms[^\d]+(\d+)ms[^\d]+(\d+)ms`)
		if m := re.FindStringSubmatch(line); len(m) == 4 {
			res.MinMs, _ = strconv.Atoi(m[1])
			res.MaxMs, _ = strconv.Atoi(m[2])
			res.AvgMs, _ = strconv.Atoi(m[3])
			res.Jitter = res.MaxMs - res.MinMs
			break
		}
	}

	if res.Recebidos == 0 && res.MinMs == 0 {
		res.Erro = "Host inacessível ou sem resposta ICMP"
	}
	return res
}

// PingPresets retorna os alvos de latência pré-configurados.
func PingPresets() []PingPreset {
	return []PingPreset{
		{ID: "cloudflare", Nome: "Cloudflare DNS (1.1.1.1)", Host: "1.1.1.1"},
		{ID: "google", Nome: "Google DNS (8.8.8.8)", Host: "8.8.8.8"},
		{ID: "riot-br", Nome: "Riot Games BR", Host: "br1.pvp.net"},
		{ID: "steam", Nome: "Steam (Valve CDN)", Host: "content.steampowered.com"},
		{ID: "epic", Nome: "Epic Games", Host: "launcher-public-service-prod06.ol.epicgames.com"},
		{ID: "microsoft", Nome: "Microsoft (Azure BR)", Host: "azure.microsoft.com"},
	}
}
