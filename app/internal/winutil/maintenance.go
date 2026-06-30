//go:build windows

package winutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// runCapture roda um comando SEM janela e devolve a saída combinada.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// lastNonEmpty devolve a última linha não-vazia (resumo de um comando).
func lastNonEmpty(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

// Ferramentas de manutenção do "Manual Mestre": ações pontuais, nativas e
// seguras (faxina de rede, TRIM, plano máximo, Defender, hibernação).

// NetworkFlush faz a faxina de rede: limpa o cache DNS e reseta a pilha
// TCP/IP + Winsock. NÃO faz release/renew (isso derrubaria uma sessão remota
// tipo TeamViewer). O reset do Winsock/IP só vale após reiniciar.
func NetworkFlush() map[string]any {
	log := []string{}
	add := func(label string, name string, args ...string) {
		out, err := runCapture(name, args...)
		s := strings.TrimSpace(lastNonEmpty(out))
		if err != nil && s == "" {
			s = "falhou"
		}
		log = append(log, label+": "+s)
	}
	add("Cache DNS", "ipconfig", "/flushdns")
	add("Winsock", "netsh", "winsock", "reset")
	add("Pilha TCP/IP", "netsh", "int", "ip", "reset")
	return map[string]any{"ok": true, "log": log, "requer_reboot": true}
}

// OptimizeSSDs roda TRIM (re-trim) nos SSDs — a manutenção correta de SSD
// (NUNCA desfragmenta SSD). HDDs o Windows desfragmenta sozinho.
func OptimizeSSDs() map[string]any {
	// defrag /C /L = re-trim em TODOS os volumes (no-op em HDD, ideal p/ SSD).
	out, err := runCapture("defrag", "/C", "/L")
	msg := "TRIM concluído nos SSDs."
	if err != nil {
		msg = "TRIM solicitado (algumas unidades podem ter sido puladas)."
	}
	_ = out
	return map[string]any{"ok": err == nil, "mensagem": msg}
}

const highPerfGUID = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"

// UltimatePerformance ATIVA o plano "Desempenho Máximo Final" (Ultimate). O
// template é oculto e não ativável direto: reusa uma cópia visível se já existe,
// senão cria uma (com GUID próprio). Cai para Alto Desempenho se não rolar.
func UltimatePerformance() map[string]any {
	const tmpl = "e9a42b02-d5df-448d-aa00-03f14749eb61"
	// 1) reusa um plano "Ultimate/Máximo" já criado (evita acumular cópias).
	guid := FindSchemeGUIDByName("ultimate", "máximo", "maximo", "desempenho máximo")
	if guid == "" {
		// 2) cria uma cópia visível do template e pega o GUID novo da saída.
		out, _ := RunPowercfg("-duplicatescheme", tmpl)
		guid = reGUID.FindString(out)
	}
	if guid != "" && guid != tmpl {
		if _, err := RunPowercfg("/setactive", guid); err == nil {
			return map[string]any{"ok": true, "mensagem": "Plano Desempenho Máximo ativado."}
		}
	}
	// fallback seguro: Alto Desempenho clássico
	RunPowercfg("/setactive", highPerfGUID)
	return map[string]any{"ok": true, "mensagem": "Plano de alto desempenho ativado."}
}

// HibernationStatus diz se a hibernação está ligada (hiberfil.sys existe) e o
// tamanho aproximado que seria liberado ao desligá-la.
func HibernationStatus() map[string]any {
	sysDrive := os.Getenv("SystemDrive")
	if sysDrive == "" {
		sysDrive = "C:"
	}
	p := filepath.Join(sysDrive+`\`, "hiberfil.sys")
	st, err := os.Stat(p)
	if err != nil {
		return map[string]any{"ligada": false, "mb": 0}
	}
	return map[string]any{"ligada": true, "mb": int(st.Size() / (1 << 20))}
}

// SetHibernation liga/desliga a hibernação. Desligar libera o hiberfil.sys
// (vários GB) mas também desativa a Inicialização Rápida.
func SetHibernation(on bool) map[string]any {
	arg := "off"
	if on {
		arg = "on"
	}
	out, err := RunPowercfg("/hibernate", arg)
	_ = out
	return map[string]any{"ok": err == nil}
}

// DefenderQuickScan dispara uma varredura rápida do Microsoft Defender em
// segundo plano (não bloqueia; o resultado aparece no próprio Defender).
func DefenderQuickScan() map[string]any {
	cmd := psHidden("Start-MpScan -ScanType QuickScan")
	if err := cmd.Start(); err != nil {
		return map[string]any{"ok": false, "erro": err.Error()}
	}
	go cmd.Wait() // não vaza o processo; roda em segundo plano
	return map[string]any{"ok": true}
}
