//go:build windows

// Package fps mede o FPS real de um jogo usando o PresentMon (Intel, MIT),
// que captura eventos de apresentacao de quadros via ETW da Microsoft — sem
// hook nem injecao (nao toma ban). O binario e embutido e extraido em runtime,
// mantendo o app como um unico arquivo portatil.
package fps

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// O PresentMon e embutido COMPACTADO (gzip). Alem de encolher o binario, isso
// evita que o cabecalho PE ("MZ...") de um .exe apareca cru dentro do nosso .exe
// — um sinal forte de "dropper" para a heuristica de antivirus. Descompactamos
// uma vez, sob demanda, antes de extrair para o disco.
//
//go:embed bin/PresentMon.exe.gz
var presentMonGz []byte

//go:embed bin/PresentMon-LICENSE.txt
var presentMonLicense []byte

var (
	presentMonOnce sync.Once
	presentMonBin  []byte
	presentMonErr  error
)

// presentMon descompacta (uma vez) o PresentMon embutido.
func presentMon() ([]byte, error) {
	presentMonOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(presentMonGz))
		if err != nil {
			presentMonErr = err
			return
		}
		defer zr.Close()
		presentMonBin, presentMonErr = io.ReadAll(zr)
	})
	return presentMonBin, presentMonErr
}

const sessionName = "ThazzDracoFPS"

// ---- Extracao do PresentMon -------------------------------------------------

func ensurePresentMon() (string, error) {
	bin, err := presentMon()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "ThazzDraco")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "PresentMon.exe")
	if st, err := os.Stat(p); err != nil || st.Size() != int64(len(bin)) {
		if err := os.WriteFile(p, bin, 0o755); err != nil {
			return "", err
		}
		// licenca MIT do PresentMon junto ao binario (conformidade Intel/MIT)
		os.WriteFile(filepath.Join(dir, "PresentMon-LICENSE.txt"), presentMonLicense, 0o644)
	}
	return p, nil
}

// ---- Gerencia de captura (assincrona) ---------------------------------------

// Manager mantem o estado da captura atual. Uma de cada vez.
type Manager struct {
	mu      sync.Mutex
	state   string // "idle" | "capturando" | "pronto" | "erro"
	proc    string
	started time.Time
	durS    int
	result  *FrameStats
	errMsg  string
}

var mgr = &Manager{state: "idle"}

// Mgr devolve o gerenciador global.
func Mgr() *Manager { return mgr }

// Start inicia uma captura do processo proc por seconds segundos.
func (m *Manager) Start(proc string, seconds int) error {
	if seconds < 5 {
		seconds = 5
	} else if seconds > 120 {
		seconds = 120
	}
	proc = strings.TrimSpace(proc)
	if proc == "" {
		return fmt.Errorf("informe o processo do jogo")
	}
	if !strings.HasSuffix(strings.ToLower(proc), ".exe") {
		proc += ".exe"
	}

	m.mu.Lock()
	if m.state == "capturando" {
		m.mu.Unlock()
		return fmt.Errorf("ja existe uma captura em andamento")
	}
	m.state = "capturando"
	m.proc = proc
	m.started = time.Now()
	m.durS = seconds
	m.result = nil
	m.errMsg = ""
	m.mu.Unlock()

	go m.run(proc, seconds)
	return nil
}

func (m *Manager) fail(msg string) {
	m.mu.Lock()
	m.state = "erro"
	m.errMsg = msg
	m.mu.Unlock()
}

func (m *Manager) run(proc string, seconds int) {
	pm, err := ensurePresentMon()
	if err != nil {
		m.fail("nao consegui preparar o PresentMon: " + err.Error())
		return
	}
	csvPath := filepath.Join(os.TempDir(), "thazzdraco_fps.csv")
	os.Remove(csvPath)
	defer os.Remove(csvPath)

	// -process_name: captura por nome (independe de estar em foco)
	// -timed N -terminate_after_timed: para sozinho apos N segundos
	// -stop_existing_session: evita conflito se sobrou sessao anterior
	// Timeout de guarda: se o PresentMon travar (ETW preso), o contexto mata o
	// processo e a captura falha — em vez de prender o motor em "capturando".
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds+20)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pm,
		"-process_name", proc,
		"-output_file", csvPath,
		"-timed", strconv.Itoa(seconds),
		"-terminate_after_timed",
		"-no_top",
		"-stop_existing_session",
		"-session_name", sessionName,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, _ := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		m.fail("o medidor de FPS demorou demais e foi interrompido. Tente de novo com o jogo aberto.")
		return
	}

	stats, perr := parseCSV(csvPath, proc)
	if perr != nil {
		outStr := strings.ToLower(string(out))
		if strings.Contains(outStr, "access denied") || strings.Contains(outStr, "performance log users") {
			m.fail("o medidor de FPS precisa de privilegio de administrador (ETW). Abra o ThazzDraco como administrador.")
			return
		}
		msg := perr.Error()
		if s := strings.TrimSpace(string(out)); s != "" {
			msg += " · " + lastLine(s)
		}
		m.fail(msg)
		return
	}
	if stats.FrameCount == 0 {
		m.fail("nenhum quadro capturado — confira se o jogo (" + proc + ") estava aberto e rodando durante a medicao")
		return
	}
	m.mu.Lock()
	m.state = "pronto"
	m.result = &stats
	m.mu.Unlock()
}

// Status devolve o estado atual para a UI consultar (poll).
func (m *Manager) Status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	resp := map[string]any{"estado": m.state, "processo": m.proc, "duracao_s": m.durS}
	if m.state == "capturando" {
		el := time.Since(m.started).Seconds()
		rest := float64(m.durS) - el
		if rest < 0 {
			rest = 0
		}
		resp["decorrido_s"] = round1(el)
		resp["restante_s"] = round1(rest)
		// margem extra: o PresentMon leva ~1-2s pra fechar e gravar o CSV
		resp["processando"] = rest <= 0
	}
	if m.state == "erro" {
		resp["erro"] = m.errMsg
	}
	if m.state == "pronto" && m.result != nil {
		resp["resultado"] = m.result
	}
	return resp
}

// ---- Parsing do CSV do PresentMon (por nome de coluna) ----------------------

func parseCSV(path, proc string) (FrameStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return FrameStats{}, fmt.Errorf("nao gerou dados de captura")
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return FrameStats{}, fmt.Errorf("captura vazia")
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	// Frametime: msBetweenDisplayChange (o que a tela mostrou) e o ideal;
	// cai para msBetweenPresents (taxa de present) se nao houver display tracking.
	ftIdx, ok := col["msBetweenDisplayChange"]
	if !ok {
		ftIdx, ok = col["msBetweenPresents"]
	}
	if !ok {
		return FrameStats{}, fmt.Errorf("formato de CSV inesperado")
	}
	dropIdx, hasDrop := col["Dropped"]

	var frametimes []float64
	dropped := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || ftIdx >= len(rec) {
			continue
		}
		if hasDrop && dropIdx < len(rec) {
			if d := strings.TrimSpace(rec[dropIdx]); d == "1" {
				dropped++
				continue // quadro descartado: nao entra no frametime exibido
			}
		}
		v, e := strconv.ParseFloat(strings.TrimSpace(rec[ftIdx]), 64)
		if e == nil {
			frametimes = append(frametimes, v)
		}
	}
	return computeStats(proc, frametimes, dropped), nil
}

func lastLine(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(parts[len(parts)-1])
}

// ---- Enumeracao de jogos (janelas visiveis -> processo) ---------------------

// GameProc descreve um candidato a captura (processo com janela visivel).
type GameProc struct {
	PID        uint32 `json:"pid"`
	Exe        string `json:"exe"`
	Titulo     string `json:"titulo"`
	Foreground bool   `json:"foreground"`
}

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows     = user32.NewProc("EnumWindows")
	procIsWindowVisible = user32.NewProc("IsWindowVisible")
	procGetWindowTextW  = user32.NewProc("GetWindowTextW")
	procGetWindowTextLn = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThread = user32.NewProc("GetWindowThreadProcessId")
	procGetForeground   = user32.NewProc("GetForegroundWindow")
	procGetWindowLongW  = user32.NewProc("GetWindowLongW")
)

const wsExToolWindow = 0x00000080

// ownExe e o nome do nosso proprio executavel, para nao se listar.
var ownExe = strings.ToLower(filepath.Base(os.Args[0]))

// shellExes: janelas do sistema que nao sao jogos.
var shellExes = map[string]bool{
	"explorer.exe": true, "applicationframehost.exe": true, "textinputhost.exe": true,
	"systemsettings.exe": true, "searchhost.exe": true, "startmenuexperiencehost.exe": true,
	"shellexperiencehost.exe": true, "msedge.exe": true, "msedgewebview2.exe": true,
}

// Games lista processos com janela de topo visivel (candidatos a jogo).
func Games() []GameProc {
	fg, _, _ := procGetForeground.Call()
	var fgPID uint32
	if fg != 0 {
		procGetWindowThread.Call(fg, uintptr(unsafe.Pointer(&fgPID)))
	}

	seen := map[uint32]bool{}
	var list []GameProc
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if vis, _, _ := procIsWindowVisible.Call(hwnd); vis == 0 {
			return 1
		}
		// ignora tool windows (sem barra de tarefas). GWL_EXSTYLE = -20;
		// convertido via variavel pois uintptr de constante negativa nao compila.
		gwlExStyle := int32(-20)
		if ex, _, _ := procGetWindowLongW.Call(hwnd, uintptr(gwlExStyle)); ex != 0 && uint32(ex)&wsExToolWindow != 0 {
			return 1
		}
		ln, _, _ := procGetWindowTextLn.Call(hwnd)
		if ln == 0 {
			return 1 // sem titulo: ignora
		}
		buf := make([]uint16, ln+1)
		procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		title := windows.UTF16ToString(buf)
		if strings.TrimSpace(title) == "" {
			return 1
		}

		var pid uint32
		procGetWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 || seen[pid] {
			return 1
		}
		exe := processExe(pid)
		le := strings.ToLower(exe)
		if exe == "" || le == ownExe || shellExes[le] {
			return 1
		}
		seen[pid] = true
		list = append(list, GameProc{PID: pid, Exe: exe, Titulo: title, Foreground: pid == fgPID})
		return 1 // continua
	})
	procEnumWindows.Call(cb, 0)
	return list
}

// processExe devolve o nome do executavel (base) de um PID.
func processExe(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}
