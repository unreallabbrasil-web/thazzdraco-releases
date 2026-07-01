package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"thazzdraco/internal/engine"
	"thazzdraco/internal/server"
	"thazzdraco/internal/winutil"
)

//go:embed VERSION
var _versionRaw string

// Version e injetado em tempo de compilação a partir do arquivo VERSION.
var Version = strings.TrimSpace(_versionRaw)

//go:embed all:web
var webFiles embed.FS

func main() {
	profileMode := flag.Bool("profile", false, "imprime o perfil de hardware e sai")
	scanMode := flag.Bool("scan", false, "roda a varredura e imprime JSON e sai")
	headless := flag.Bool("headless", false, "sobe o servidor sem abrir janela nem elevar (teste)")
	port := flag.Int("port", 0, "porta fixa (0 = efemera)")
	flag.Parse()

	switch {
	case *profileMode:
		dumpJSON(map[string]any{"admin": winutil.IsAdmin(), "sid": winutil.RealUserSid(), "perfil": winutil.BuildProfile()})
		return
	case *scanMode:
		rules, _ := engine.LoadRules()
		dumpJSON(engine.Scan(rules, engine.BuildCtx()))
		return
	}

	// Modo app: precisa de admin para escrever HKLM/servicos/powercfg.
	if !*headless && !winutil.IsAdmin() {
		if err := winutil.RelaunchElevated(); err != nil {
			fmt.Println("Este programa precisa ser executado como administrador.")
		}
		return
	}

	rules, err := engine.LoadRules()
	if err != nil {
		fatal(err)
	}
	presets, _ := engine.LoadPresets()
	webFS, _ := fs.Sub(webFiles, "web")

	srv := server.New(rules, presets, webFS, Version)

	// inicia checagem de atualizacoes em background (primeira em 45s)
	winutil.StartUpdateChecker(Version)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fatal(err)
	}
	url := fmt.Sprintf("http://%s/", ln.Addr().String())

	httpSrv := &http.Server{Handler: srv.Handler()}
	go httpSrv.Serve(ln)

	if *headless {
		fmt.Println("ThazzDraco headless em", url)
		select {} // mantem rodando para testes manuais
	}

	openWindow(url)
	watchdog(srv) // encerra quando a janela fecha (sem ping)
}

// watchdog encerra o processo quando a janela do app fecha (para de dar sinal de
// vida). Cuidados importantes:
//   - NUNCA encerra com requisicao em andamento (backup/reparo/benchmark seguram
//     varios segundos sem ping) — senao mata o backend no meio e a UI ve "Failed
//     to fetch".
//   - Janela de inatividade FOLGADA (90s): o navegador estrangula os timers (o
//     ping) quando a janela perde o foco, chegando a 1x/min. 12s matava o backend
//     com a janela so em segundo plano. 90s sobrevive ao estrangulamento e ainda
//     limpa o processo orfao logo apos o usuario fechar a janela.
func watchdog(srv *server.Server) {
	time.Sleep(5 * time.Second) // carencia inicial ate a UI carregar
	for {
		time.Sleep(3 * time.Second)
		if srv.InFlight() == 0 && srv.IdleSince() > 90*time.Second {
			os.Exit(0)
		}
	}
}

// openWindow abre a UI numa janela tipo-app do Edge (sem abas/barra), com
// fallback para o navegador padrao. Sem dependencias externas.
// Usa um --user-data-dir isolado para garantir uma janela-app standalone
// (nao abrir como aba numa janela do Edge ja existente).
func openWindow(url string) {
	if edge := findEdge(); edge != "" {
		profile := filepath.Join(os.Getenv("LOCALAPPDATA"), "ThazzDraco", "edge")
		cmd := exec.Command(edge,
			"--app="+url,
			"--window-size=1380,940",
			"--user-data-dir="+profile,
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-translate",
			"--disable-features=Translate,TranslateUI,msEdgeTranslate")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if cmd.Start() == nil {
			return
		}
	}
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func findEdge() string {
	cands := []string{
		os.Getenv("ProgramFiles(x86)") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("ProgramFiles") + `\Microsoft\Edge\Application\msedge.exe`,
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func dumpJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "erro:", err)
	os.Exit(1)
}
