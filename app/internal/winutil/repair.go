//go:build windows

package winutil

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Reparo do Windows "nivel formatacao" (sem formatar): orquestra SFC, DISM e o
// reset do Windows Update, com saida ao vivo. Tudo sao operacoes de reparo do
// proprio Windows (nao destrutivas a dados) — exigem administrador.

// repairTitles: passos disponiveis e seus rotulos.
var repairTitles = map[string]string{
	"sfc":          "Verificador de Arquivos do Sistema (SFC)",
	"dism-restore": "Reparar a imagem do Windows (DISM RestoreHealth)",
	"dism-cleanup": "Limpar componentes antigos do Windows (DISM)",
	"chkdsk":       "Verificar o disco do sistema (CHKDSK)",
	"wureset":      "Resetar o Windows Update",
}

var repairOrder = []string{"sfc", "dism-restore", "dism-cleanup", "chkdsk", "wureset"}
var rePct = regexp.MustCompile(`(\d{1,3}(?:[.,]\d)?)\s*%`)
var reProgressOnly = regexp.MustCompile(`^[\[\]=\s.\d%-]+$`)

type RepairManager struct {
	mu        sync.Mutex
	state     string // idle | rodando | concluido | erro
	steps     []string
	idxAtual  int
	tituloEt  string
	pct       float64
	log       []string
	started   time.Time
	errMsg    string
	lastIntPct int
}

var repairMgr = &RepairManager{state: "idle"}

func RepairMgr() *RepairManager { return repairMgr }

// Start inicia o reparo com os passos selecionados (na ordem canonica).
func (m *RepairManager) Start(steps []string) error {
	var ord []string
	for _, s := range repairOrder {
		for _, sel := range steps {
			if sel == s {
				ord = append(ord, s)
			}
		}
	}
	if len(ord) == 0 {
		return errRepair("selecione ao menos uma etapa de reparo")
	}
	m.mu.Lock()
	if m.state == "rodando" {
		m.mu.Unlock()
		return errRepair("ja existe um reparo em andamento")
	}
	m.state = "rodando"
	m.steps = ord
	m.idxAtual = 0
	m.pct = 0
	m.log = nil
	m.errMsg = ""
	m.started = time.Now()
	m.mu.Unlock()
	go m.run(ord)
	return nil
}

type repairErr string

func errRepair(s string) error  { return repairErr(s) }
func (e repairErr) Error() string { return string(e) }

func (m *RepairManager) appendLog(line string) {
	line = strings.TrimRight(line, " \t")
	if line == "" {
		return
	}
	m.mu.Lock()
	m.log = append(m.log, line)
	if len(m.log) > 500 {
		m.log = m.log[len(m.log)-500:]
	}
	m.mu.Unlock()
}

func (m *RepairManager) setStep(i int, title string) {
	m.mu.Lock()
	m.idxAtual = i
	m.tituloEt = title
	m.pct = 0
	m.lastIntPct = -1
	m.mu.Unlock()
}

func (m *RepairManager) setPct(p float64) {
	m.mu.Lock()
	m.pct = p
	m.mu.Unlock()
}

func (m *RepairManager) run(steps []string) {
	for i, s := range steps {
		m.setStep(i, repairTitles[s])
		m.appendLog("=== " + repairTitles[s] + " ===")
		switch s {
		case "sfc":
			m.stream("sfc", "/scannow")
		case "dism-restore":
			m.stream("dism", "/Online", "/Cleanup-Image", "/RestoreHealth")
		case "dism-cleanup":
			m.stream("dism", "/Online", "/Cleanup-Image", "/StartComponentCleanup")
		case "chkdsk":
			// /scan = verificação ONLINE (sem reinício, não-destrutiva).
			m.stream("chkdsk", systemDriveLetter(), "/scan")
		case "wureset":
			m.windowsUpdateReset()
		}
		m.setPct(100)
	}
	m.mu.Lock()
	m.state = "concluido"
	m.tituloEt = "Concluído"
	m.mu.Unlock()
}

// stream roda um comando e transmite a saida (linha a linha, tratando \r de
// barras de progresso) para o log + atualiza a porcentagem.
func (m *RepairManager) stream(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.StdoutPipe()
	if err != nil {
		m.appendLog("erro ao iniciar: " + err.Error())
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		m.appendLog("falha ao executar (precisa de administrador?): " + err.Error())
		return
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	sc.Split(scanLinesCR)
	for sc.Scan() {
		chunk := strings.TrimSpace(sc.Text())
		if chunk == "" {
			continue
		}
		if mt := rePct.FindStringSubmatch(chunk); mt != nil {
			if v, e := strconv.ParseFloat(strings.Replace(mt[1], ",", ".", 1), 64); e == nil {
				m.setPct(v)
				// barras de progresso "puras" nao poluem o log; so o numero conta
				m.mu.Lock()
				ip := int(v)
				changed := ip != m.lastIntPct
				m.lastIntPct = ip
				m.mu.Unlock()
				if reProgressOnly.MatchString(chunk) {
					continue
				}
				if !changed {
					continue
				}
			}
		}
		if reProgressOnly.MatchString(chunk) {
			continue
		}
		m.appendLog(chunk)
	}
	cmd.Wait()
}

// windowsUpdateReset para os servicos, renomeia os caches e reinicia (padrao
// de troubleshooting; o Windows recria os caches). Nao toca em dados do usuario.
func (m *RepairManager) windowsUpdateReset() {
	svcs := []string{"wuauserv", "bits", "cryptsvc", "msiserver"}
	for _, s := range svcs {
		m.appendLog("Parando serviço " + s + "…")
		runQuiet("net", "stop", s)
	}
	win := os.Getenv("SystemRoot")
	if win == "" {
		win = `C:\Windows`
	}
	stamp := time.Now().Format("20060102-150405")
	renames := map[string]string{
		filepath.Join(win, "SoftwareDistribution"):    filepath.Join(win, "SoftwareDistribution.bak-"+stamp),
		filepath.Join(win, "System32", "catroot2"):    filepath.Join(win, "System32", "catroot2.bak-"+stamp),
	}
	for src, dst := range renames {
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				m.appendLog("aviso: não consegui renomear " + filepath.Base(src) + " (" + err.Error() + ")")
			} else {
				m.appendLog("Cache renomeado: " + filepath.Base(src) + " → " + filepath.Base(dst))
			}
		}
	}
	for _, s := range svcs {
		m.appendLog("Iniciando serviço " + s + "…")
		runQuiet("net", "start", s)
	}
	m.appendLog("Windows Update resetado. Caches antigos preservados como .bak (apague depois se quiser).")
}

func systemDriveLetter() string {
	d := os.Getenv("SystemDrive")
	if d == "" {
		return "C:"
	}
	return d
}

func runQuiet(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Run()
}

// Status devolve o andamento do reparo para a UI consultar (poll).
func (m *RepairManager) Status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	tail := m.log
	if len(tail) > 60 {
		tail = tail[len(tail)-60:]
	}
	resp := map[string]any{
		"estado":      m.state,
		"etapa":       m.tituloEt,
		"etapa_idx":   m.idxAtual,
		"etapas_tot":  len(m.steps),
		"pct":         int(m.pct + 0.5),
		"log":         append([]string(nil), tail...),
	}
	if m.state == "rodando" {
		resp["decorrido_s"] = int(time.Since(m.started).Seconds())
	}
	if m.state == "erro" {
		resp["erro"] = m.errMsg
	}
	return resp
}

// scanLinesCR: SplitFunc que quebra em \n OU \r (barras de progresso usam \r).
func scanLinesCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
