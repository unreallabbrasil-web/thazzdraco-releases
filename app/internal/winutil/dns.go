//go:build windows

package winutil

import (
	"fmt"
	"strings"
)

// DNSProvider representa um servidor DNS rápido pré-configurado.
type DNSProvider struct {
	ID      string   `json:"id"`
	Nome    string   `json:"nome"`
	Servers []string `json:"servers"`
	Nota    string   `json:"nota"`
}

// DNSProviders lista os provedores disponíveis (exportado para a API).
var DNSProviders = []DNSProvider{
	{ID: "cloudflare", Nome: "Cloudflare", Servers: []string{"1.1.1.1", "1.0.0.1"}, Nota: "Mais rápido globalmente, foco em privacidade"},
	{ID: "google", Nome: "Google", Servers: []string{"8.8.8.8", "8.8.4.4"}, Nota: "Confiável, boa cobertura mundial"},
	{ID: "quad9", Nome: "Quad9", Servers: []string{"9.9.9.9", "149.112.112.112"}, Nota: "Bloqueia domínios maliciosos (segurança)"},
	{ID: "opendns", Nome: "OpenDNS", Servers: []string{"208.67.222.222", "208.67.220.220"}, Nota: "Estável, opção com controle parental"},
}

// SetDNS aplica os servidores DNS do provedor em todas as interfaces ativas.
// provider = "cloudflare" | "google" | "quad9" | "opendns" | "dhcp"
func SetDNS(provider string) map[string]any {
	reset := strings.EqualFold(provider, "dhcp")
	var servers []string
	var provNome string
	if !reset {
		for _, p := range DNSProviders {
			if strings.EqualFold(p.ID, provider) {
				servers = p.Servers
				provNome = p.Nome
				break
			}
		}
		if len(servers) == 0 {
			return map[string]any{"ok": false, "erro": "provedor desconhecido: " + provider}
		}
	} else {
		provNome = "DHCP automático"
	}

	// lista interfaces conectadas via netsh
	out, _ := runCapture("netsh", "interface", "show", "interface")
	var ifaces []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "Connected") && !strings.Contains(line, "Conectado") {
			continue
		}
		// formato: "Enabled     Connected      3      Ethernet"
		// o nome começa depois da 3ª coluna de espaços
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			name := strings.Join(parts[3:], " ")
			if name != "" {
				ifaces = append(ifaces, name)
			}
		}
	}
	if len(ifaces) == 0 {
		// fallback: tenta os nomes mais comuns
		ifaces = []string{"Ethernet", "Wi-Fi", "Local Area Connection", "Wireless Network Connection"}
	}

	ok := 0
	for _, iface := range ifaces {
		if reset {
			r1, e1 := runCapture("netsh", "interface", "ip", "set", "dns", iface, "dhcp")
			_ = r1
			if e1 == nil {
				ok++
			}
		} else {
			_, e := runCapture("netsh", "interface", "ip", "set", "dns", iface, "static", servers[0], "primary")
			if e == nil {
				ok++
				if len(servers) > 1 {
					runCapture("netsh", "interface", "ip", "add", "dns", iface, servers[1], "index=2")
				}
			}
		}
	}
	if ok == 0 {
		return map[string]any{
			"ok":   false,
			"erro": fmt.Sprintf("não consegui configurar nenhuma interface (%d tentadas). Rode como administrador.", len(ifaces)),
		}
	}
	msg := fmt.Sprintf("DNS %s aplicado em %d interface(s). Efeito imediato.", provNome, ok)
	if reset {
		msg = fmt.Sprintf("DNS resetado para DHCP em %d interface(s).", ok)
	}
	return map[string]any{"ok": true, "mensagem": msg, "provider": provider}
}
