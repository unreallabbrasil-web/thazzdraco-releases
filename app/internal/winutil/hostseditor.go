//go:build windows

package winutil

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// HostEntry representa uma entrada no arquivo hosts.
type HostEntry struct {
	IP       string `json:"ip"`
	Host     string `json:"host"`
	Comment  string `json:"comment,omitempty"`
	Disabled bool   `json:"disabled"`
	Managed  bool   `json:"managed"`
}

const tzHostsTag = "# [ThazzDraco]"

func hostsPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// HostsList lê o arquivo hosts e retorna as entradas ativas e gerenciadas.
func HostsList() ([]HostEntry, error) {
	f, err := os.Open(hostsPath())
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []HostEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		disabled := false
		body := trimmed
		if strings.HasPrefix(trimmed, "#") {
			candidate := strings.TrimSpace(trimmed[1:])
			parts := strings.Fields(candidate)
			if len(parts) < 2 || !isIPLike(parts[0]) {
				continue // comentário puro, não uma entrada desativada
			}
			disabled = true
			body = candidate
		}

		parts := strings.Fields(body)
		if len(parts) < 2 || !isIPLike(parts[0]) {
			continue
		}
		ip := parts[0]
		host := parts[1]

		// extrair comentário inline (somente entradas ativas)
		comment := ""
		if !disabled {
			if idx := strings.Index(body, "#"); idx >= 0 {
				comment = strings.TrimSpace(body[idx+1:])
				comment = strings.TrimPrefix(comment, "[ThazzDraco]")
				comment = strings.TrimSpace(comment)
			}
		}

		managed := strings.Contains(line, tzHostsTag)
		entries = append(entries, HostEntry{
			IP: ip, Host: host, Comment: comment,
			Disabled: disabled, Managed: managed,
		})
	}
	return entries, scanner.Err()
}

// HostsAdd adiciona uma nova entrada ao arquivo hosts.
func HostsAdd(ip, host, comment string) error {
	ip = strings.TrimSpace(ip)
	// Validação REAL do IP (net.ParseIP) — o antigo isIPLike aceitava "1.1.1.1 evil"
	// e injetava um segundo mapeamento na linha.
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("IP inválido: %s", ip)
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, " \t\r\n#") {
		return fmt.Errorf("hostname inválido")
	}
	// Comentário não pode conter quebra de linha (injetaria linhas arbitrárias no
	// hosts) nem '#' (fecharia/abriria comentário).
	if strings.ContainsAny(comment, "\r\n#") {
		return fmt.Errorf("comentário inválido")
	}
	// verificar duplicata
	existing, _ := HostsList()
	for _, e := range existing {
		if strings.EqualFold(e.IP, ip) && strings.EqualFold(e.Host, host) {
			return fmt.Errorf("entrada já existe: %s %s", ip, host)
		}
	}
	entry := fmt.Sprintf("%s\t%s", ip, host)
	if comment != "" {
		entry += "\t# " + comment + " " + tzHostsTag
	} else {
		entry += "\t" + tzHostsTag
	}
	// Reusa a mesma normalizacao de hostsRewrite (le, normaliza \r\n->\n, reescreve
	// tudo com \r\n uniforme) em vez de O_APPEND bruto — evita misturar \r\n novo
	// com \n pre-existente se o arquivo tiver sido editado por outra ferramenta.
	return hostsRewrite(func(line string) (string, bool) { return line, true }, entry)
}

// HostsRemove remove uma entrada pelo par ip+host.
func HostsRemove(ip, host string) error {
	return hostsRewrite(func(line string) (string, bool) {
		if hostsLineMatches(line, ip, host) {
			return "", false
		}
		return line, true
	})
}

// HostsToggle comenta ou descomenta uma entrada.
func HostsToggle(ip, host string) error {
	return hostsRewrite(func(line string) (string, bool) {
		trimmed := strings.TrimSpace(line)
		if !hostsLineMatches(line, ip, host) {
			return line, true
		}
		if strings.HasPrefix(trimmed, "#") {
			// desativar → ativar: remove o "#" inicial
			restored := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			return restored, true
		}
		return "# " + trimmed, true
	})
}

// hostsLineMatches verifica se uma linha (ativa ou comentada) corresponde ao par ip+host.
func hostsLineMatches(line, ip, host string) bool {
	trimmed := strings.TrimSpace(line)
	body := trimmed
	if strings.HasPrefix(trimmed, "#") {
		body = strings.TrimSpace(trimmed[1:])
	}
	parts := strings.Fields(body)
	return len(parts) >= 2 &&
		strings.EqualFold(parts[0], ip) &&
		strings.EqualFold(parts[1], host)
}

// hostsRewrite lê, transforma e reescreve o arquivo hosts. Normaliza para \n
// antes de processar e escreve de volta com \r\n (padrão Windows). appendLines,
// se fornecido, é acrescentado ao final (usado por HostsAdd, já normalizado).
func hostsRewrite(transform func(string) (string, bool), appendLines ...string) error {
	p := hostsPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	// Backup antes de qualquer alteração — o hosts pode conter entradas de licença/
	// software de terceiros; um erro deve ser restaurável (copie o .bak de volta).
	_ = os.WriteFile(p+".thazzdraco.bak", data, 0644)

	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	var out []string
	for _, l := range lines {
		nl, keep := transform(l)
		if keep {
			out = append(out, nl)
		}
	}
	out = append(out, appendLines...)
	// Escrita atômica (tmp + rename): um crash no meio não deixa o hosts truncado.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(out, "\r\n")), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func isIPLike(s string) bool {
	return len(s) >= 7 && len(s) <= 39 && (strings.Contains(s, ".") || strings.Contains(s, ":"))
}
