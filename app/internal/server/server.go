// Package server expoe a UI web embutida e a API JSON local que dirige o motor.
// Tudo em localhost, porta efemera. Um watchdog encerra o processo quando a
// janela do app para de mandar "ping" (ou seja, quando o usuario fecha a janela).
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"thazzdraco/internal/engine"
	"thazzdraco/internal/fps"
	"thazzdraco/internal/winutil"
)

type Server struct {
	rules   []engine.Rule
	presets []engine.Preset
	web     http.Handler
	version string

	mu       sync.Mutex // serializa operacoes do motor (scan/apply/registro)
	opMu     sync.Mutex // serializa operacoes destrutivas pesadas (debloat/limpeza/driver)
	lastPing time.Time
	pingMu   sync.Mutex
	inFlight int32 // requisicoes em andamento — o watchdog nunca encerra com >0

	// F1: Modo Game ao Vivo
	gameMu        sync.Mutex
	gameAutoMode  bool
	gameCacheTime time.Time
	gameCache     []winutil.Game
}

// New monta o servidor com as regras/presets carregados e o FS da UI.
func New(rules []engine.Rule, presets []engine.Preset, webFS fs.FS, version string) *Server {
	return &Server{
		rules:    rules,
		presets:  presets,
		web:      http.FileServer(http.FS(webFS)),
		version:  version,
		lastPing: time.Now(),
	}
}

// Handler devolve o roteador HTTP completo (API + arquivos estaticos).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ping", s.handlePing)
	mux.HandleFunc("/api/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/api/info", s.handleInfo)
	mux.HandleFunc("/api/rebuild", s.handleRebuild)
	mux.HandleFunc("/api/escanear", s.handleScan)
	mux.HandleFunc("/api/aplicar", s.handleApply)
	mux.HandleFunc("/api/aplicar-verdes", s.handleApplyGreens)
	mux.HandleFunc("/api/aplicar-preset", s.handleApplyPreset)
	mux.HandleFunc("/api/desfazer", s.handleUndo)
	mux.HandleFunc("/api/historico", s.handleHistory)
	mux.HandleFunc("/api/presets", s.handlePresets)
	mux.HandleFunc("/api/reiniciar", s.handleReboot)
	mux.HandleFunc("/api/saude", s.handleHealth)
	mux.HandleFunc("/api/inicializacao", s.handleStartup)
	mux.HandleFunc("/api/inicializacao/set", s.handleStartupSet)
	mux.HandleFunc("/api/metricas", s.handleMetrics)
	mux.HandleFunc("/api/benchmark", s.handleBenchmark)
	mux.HandleFunc("/api/fps/jogos", s.handleFPSGames)
	mux.HandleFunc("/api/fps/iniciar", s.handleFPSStart)
	mux.HandleFunc("/api/fps/status", s.handleFPSStatus)
	mux.HandleFunc("/api/diagnostico", s.handleDiagnose)
	mux.HandleFunc("/api/driver", s.handleDriver)
	mux.HandleFunc("/api/driver/limpar", s.handleDriverClean)
	mux.HandleFunc("/api/reparo/iniciar", s.handleRepairStart)
	mux.HandleFunc("/api/reparo/status", s.handleRepairStatus)
	mux.HandleFunc("/api/debloat/lista", s.handleBloatList)
	mux.HandleFunc("/api/debloat/remover", s.handleBloatRemove)
	mux.HandleFunc("/api/limpeza-profunda/scan", s.handleDeepScan)
	mux.HandleFunc("/api/limpeza-profunda/limpar", s.handleDeepClean)
	mux.HandleFunc("/api/ferramentas/rede", s.handleToolNetwork)
	mux.HandleFunc("/api/ferramentas/trim", s.handleToolTrim)
	mux.HandleFunc("/api/ferramentas/energia-max", s.handleToolUltimate)
	mux.HandleFunc("/api/ferramentas/hibernacao", s.handleToolHibernation)
	mux.HandleFunc("/api/ferramentas/hibernacao/status", s.handleToolHibStatus)
	mux.HandleFunc("/api/ferramentas/defender", s.handleToolDefender)
	mux.HandleFunc("/api/backup/lista", s.handleBackupList)
	mux.HandleFunc("/api/backup/criar", s.handleBackupCreate)
	mux.HandleFunc("/api/backup/restaurar", s.handleBackupRestore)
	mux.HandleFunc("/api/backup/excluir", s.handleBackupDelete)
	mux.HandleFunc("/api/jogos", s.handleGames)
	mux.HandleFunc("/api/jogos/tweak", s.handleGameTweak)
	mux.HandleFunc("/api/jogos/exportar", s.handleGamesExport)
	mux.HandleFunc("/api/jogos/importar", s.handleGamesImport)
	mux.HandleFunc("/api/jogos/config", s.handleGameConfig)
	mux.HandleFunc("/api/jogos/config/set", s.handleGameConfigSet)
	mux.HandleFunc("/api/jogos/cover", s.handleGameCover)
	mux.HandleFunc("/api/update/check", s.handleUpdateCheck)
	mux.HandleFunc("/api/update/install", s.handleUpdateInstall)
	mux.HandleFunc("/api/update/progress", s.handleUpdateProgress)
	mux.HandleFunc("/api/update/apply", s.handleUpdateApply)
	mux.HandleFunc("/api/turbo", s.handleTurbo)
	mux.HandleFunc("/api/modo-game/status", s.handleModoGameStatus)
	mux.HandleFunc("/api/modo-game/set", s.handleModoGameSet)
	mux.HandleFunc("/api/servicos", s.handleServicos)
	mux.HandleFunc("/api/servicos/parar", s.handleServicoParar)
	mux.HandleFunc("/api/drivers/verificar", s.handleDriversVerificar)
	mux.HandleFunc("/api/ferramentas/dns", s.handleToolDNS)
	mux.HandleFunc("/api/ferramentas/dns/providers", s.handleDNSProviders)
	mux.HandleFunc("/api/gpu/painel", s.handleGPUPanel)
	mux.HandleFunc("/api/gpu/poder", s.handleGPUPower)
	// Sprint 3: AMD panel, affinity, DPC, custom presets
	mux.HandleFunc("/api/gpu/amd", s.handleGPUAmd)
	mux.HandleFunc("/api/ferramentas/afinidade", s.handleListProcesses)
	mux.HandleFunc("/api/ferramentas/afinidade/set", s.handleSetAffinity)
	mux.HandleFunc("/api/ferramentas/dpc", s.handleDPC)
	mux.HandleFunc("/api/presets/custom", s.handleCustomPresets)
	mux.HandleFunc("/api/presets/custom/salvar", s.handleCustomPresetSalvar)
	mux.HandleFunc("/api/presets/custom/excluir", s.handleCustomPresetExcluir)
	mux.HandleFunc("/api/presets/custom/aplicar", s.handleCustomPresetAplicar)
	// Sprint 2: browser cleaner, hosts editor, pinger, Windows Update
	mux.HandleFunc("/api/browser/scan", s.handleBrowserScan)
	mux.HandleFunc("/api/browser/limpar", s.handleBrowserLimpar)
	mux.HandleFunc("/api/hosts", s.handleHostsList)
	mux.HandleFunc("/api/hosts/adicionar", s.handleHostsAdicionar)
	mux.HandleFunc("/api/hosts/remover", s.handleHostsRemover)
	mux.HandleFunc("/api/hosts/toggle", s.handleHostsToggle)
	mux.HandleFunc("/api/ferramentas/ping/presets", s.handlePingPresets)
	mux.HandleFunc("/api/ferramentas/ping", s.handlePing2)
	mux.HandleFunc("/api/ferramentas/wu/status", s.handleWUStatus)
	mux.HandleFunc("/api/ferramentas/wu/pausar", s.handleWUPausar)
	mux.HandleFunc("/api/ferramentas/wu/retomar", s.handleWURetomar)
	mux.Handle("/", s.web)
	return s.instrument(sameOriginGuard(mux))
}

// instrument conta as requisicoes em andamento e marca atividade a CADA chamada.
// Assim o watchdog (a) nunca encerra no meio de uma operacao e (b) trata qualquer
// request como sinal de vida — nao so o /api/ping, que o navegador estrangula
// quando a janela perde o foco (a causa raiz do "Failed to fetch" em operacoes
// longas como o backup).
func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.inFlight, 1)
		s.touchPing()
		defer func() {
			s.touchPing()
			atomic.AddInt32(&s.inFlight, -1)
		}()
		next.ServeHTTP(w, r)
	})
}

// InFlight informa quantas requisicoes estao em andamento agora.
func (s *Server) InFlight() int32 { return atomic.LoadInt32(&s.inFlight) }

// sameOriginGuard bloqueia chamadas a /api/ vindas de OUTRA origem (um site
// malicioso aberto no navegador do usuario nao pode disparar nossas operacoes,
// mesmo descobrindo a porta). Baseia-se no Sec-Fetch-Site (que o JS nao consegue
// forjar) e no Origin. Como o app roda elevado, isto fecha o vetor de CSRF.
func sameOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if sfs := r.Header.Get("Sec-Fetch-Site"); sfs == "cross-site" || sfs == "same-site" {
				http.Error(w, "origem nao permitida", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || u.Host != r.Host {
					http.Error(w, "origem nao permitida", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Watchdog de janela -----------------------------------------------------

func (s *Server) touchPing() {
	s.pingMu.Lock()
	s.lastPing = time.Now()
	s.pingMu.Unlock()
}

// IdleSince retorna ha quanto tempo nao chega um ping da janela.
func (s *Server) IdleSince() time.Duration {
	s.pingMu.Lock()
	defer s.pingMu.Unlock()
	return time.Since(s.lastPing)
}

// ---- Helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) scan() engine.ScanResult {
	return engine.Scan(s.rules, engine.BuildCtx())
}

// ---- Handlers ---------------------------------------------------------------

func (s *Server) handlePing(w http.ResponseWriter, _ *http.Request) {
	s.touchPing()
	writeJSON(w, 200, map[string]any{"ok": true})
}

// C4: SSE heartbeat — mantém uma conexão de longa duração com a janela.
// Enquanto conectado, inFlight>0 impede o watchdog de encerrar o processo.
// Quando a janela fecha, a conexão cai e inFlight volta a zero.
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprintf(w, "data: ok\n\n")
	flusher.Flush()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			s.touchPing()
			fmt.Fprintf(w, "data: ok\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"versao": s.version,
		"admin":  winutil.IsAdmin(),
		"app":    "ThazzDraco Optimizer",
	})
}

func (s *Server) handleRebuild(w http.ResponseWriter, _ *http.Request) {
	// Localiza CONSTRUIR.ps1 dois níveis acima do executável
	exe, err := os.Executable()
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	script := filepath.Join(filepath.Dir(exe), "..", "CONSTRUIR.ps1")
	script = filepath.Clean(script)
	if _, err := os.Stat(script); err != nil {
		writeJSON(w, 404, map[string]any{"ok": false, "erro": "CONSTRUIR.ps1 não encontrado em " + script})
		return
	}
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-File", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010} // CREATE_NEW_CONSOLE
	if err := cmd.Start(); err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "script": script})
}

func (s *Server) handleScan(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, 200, s.scan())
}

type applyReq struct {
	IDs       []string `json:"ids"`
	Confirmar bool     `json:"confirmar"`
	PresetID  string   `json:"preset_id"`
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	var req applyReq
	json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := engine.BuildCtx()
	rep := engine.ApplyRules(s.rules, req.IDs, ctx, req.Confirmar, "selecao")
	writeJSON(w, 200, map[string]any{"relatorio": rep, "scan": engine.Scan(s.rules, ctx)})
}

func (s *Server) handleApplyGreens(w http.ResponseWriter, r *http.Request) {
	var req applyReq
	json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := engine.BuildCtx()
	pre := engine.Scan(s.rules, ctx)
	var ids []string
	for _, rv := range pre.Regras {
		if rv.Tier == "verde" && rv.Aplicavel && !rv.RequerConsentimento {
			ids = append(ids, rv.ID)
		}
	}
	rep := engine.ApplyRules(s.rules, ids, ctx, req.Confirmar, "verdes")
	writeJSON(w, 200, map[string]any{"relatorio": rep, "scan": engine.Scan(s.rules, ctx)})
}

func (s *Server) handleApplyPreset(w http.ResponseWriter, r *http.Request) {
	var req applyReq
	json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for _, p := range s.presets {
		if p.ID == req.PresetID {
			ids = p.IDs
			break
		}
	}
	ctx := engine.BuildCtx()
	rep := engine.ApplyRules(s.rules, ids, ctx, req.Confirmar, "preset:"+req.PresetID)
	writeJSON(w, 200, map[string]any{"relatorio": rep, "scan": engine.Scan(s.rules, ctx)})
}

type undoReq struct {
	BatchID string `json:"batch_id"`
	RuleID  string `json:"rule_id"`
	Tudo    bool   `json:"tudo"`
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	var req undoReq
	json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	switch {
	case req.Tudo:
		engine.UndoAll()
	case req.BatchID != "":
		_, err = engine.UndoBatch(req.BatchID)
	case req.RuleID != "":
		err = engine.UndoRule(req.RuleID)
	}
	resp := map[string]any{"scan": s.scan()}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleHistory(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, engine.LoadHistory())
}

func (s *Server) handlePresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.presets)
}

// handleStartup lista as entradas de inicializacao do Windows.
func (s *Server) handleStartup(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.ListStartup(winutil.RealUserSid()))
}

type startupSetReq struct {
	Kind    string `json:"kind"`
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

// handleStartupSet habilita/desabilita uma entrada de inicializacao (reversivel).
func (s *Server) handleStartupSet(w http.ResponseWriter, r *http.Request) {
	var req startupSetReq
	json.NewDecoder(r.Body).Decode(&req)
	err := winutil.SetStartupEnabled(req.Kind, req.Key, req.Enabled, winutil.RealUserSid())
	resp := map[string]any{"ok": err == nil, "lista": winutil.ListStartup(winutil.RealUserSid())}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// handleMetrics devolve metricas ao vivo (CPU/RAM/processos/uptime).
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.Metrics())
}

// handleHealth devolve a saude do PC (discos, S.M.A.R.T., RAM, bateria, uptime).
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.BuildHealth())
}

// handleBenchmark roda o benchmark sintetico (CPU/memoria/disco) e devolve
// numeros reais medidos. Leva ~1-2s; nao precisa de admin.
func (s *Server) handleBenchmark(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.Benchmark())
}

// ---- FPS in-game (PresentMon/ETW) -------------------------------------------

// handleFPSGames lista processos com janela visivel (candidatos a jogo).
func (s *Server) handleFPSGames(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, fps.Games())
}

type fpsStartReq struct {
	Processo string `json:"processo"`
	Segundos int    `json:"segundos"`
}

// handleFPSStart inicia uma captura assincrona de FPS. A UI faz poll em /status.
func (s *Server) handleFPSStart(w http.ResponseWriter, r *http.Request) {
	var req fpsStartReq
	json.NewDecoder(r.Body).Decode(&req)
	if req.Segundos == 0 {
		req.Segundos = 30
	}
	err := fps.Mgr().Start(req.Processo, req.Segundos)
	resp := map[string]any{"ok": err == nil}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// handleFPSStatus devolve o andamento/resultado da captura atual.
func (s *Server) handleFPSStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, fps.Mgr().Status())
}

// handleDiagnose roda o diagnostico de gargalos de desempenho (dados reais).
func (s *Server) handleDiagnose(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.Diagnose(winutil.RealUserSid()))
}

// handleDriver devolve as informacoes dos drivers de GPU + residuos de instalador.
func (s *Server) handleDriver(w http.ResponseWriter, _ *http.Request) {
	folder, mb := winutil.DriverLeftovers()
	resp := map[string]any{"gpus": winutil.GPUDrivers()}
	if folder != "" {
		resp["residuos"] = map[string]any{"pasta": folder, "mb": mb}
	}
	writeJSON(w, 200, resp)
}

type repairStartReq struct {
	Etapas []string `json:"etapas"`
}

// handleRepairStart inicia o reparo do Windows (SFC/DISM/Windows Update).
func (s *Server) handleRepairStart(w http.ResponseWriter, r *http.Request) {
	var req repairStartReq
	json.NewDecoder(r.Body).Decode(&req)
	err := winutil.RepairMgr().Start(req.Etapas)
	resp := map[string]any{"ok": err == nil}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// handleRepairStatus devolve o andamento/saida do reparo (poll).
func (s *Server) handleRepairStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.RepairMgr().Status())
}

// handleBloatList lista os apps de bloatware removiveis (catalogo seguro).
func (s *Server) handleBloatList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.ListBloat())
}

// handleDeepScan calcula o tamanho de cada categoria da limpeza profunda.
func (s *Server) handleDeepScan(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.DeepCleanScan(winutil.RealUserSid()))
}

type deepCleanReq struct {
	Ids       []string `json:"ids"`
	Confirmar bool     `json:"confirmar"`
}

// handleDeepClean limpa as categorias selecionadas (regeneraveis; sem dados pessoais).
// As categorias irreversiveis (Lixeira, Windows.old) exigem confirmar=true.
func (s *Server) handleDeepClean(w http.ResponseWriter, r *http.Request) {
	var req deepCleanReq
	json.NewDecoder(r.Body).Decode(&req)
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.DeepClean(winutil.RealUserSid(), req.Ids, req.Confirmar))
}

// ---- Ferramentas de manutencao (Manual Mestre) ------------------------------

func (s *Server) handleToolNetwork(w http.ResponseWriter, _ *http.Request) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.NetworkFlush())
}

func (s *Server) handleToolTrim(w http.ResponseWriter, _ *http.Request) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.OptimizeSSDs())
}

func (s *Server) handleToolUltimate(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, 200, winutil.UltimatePerformance())
}

func (s *Server) handleToolHibStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.HibernationStatus())
}

type hibReq struct {
	On bool `json:"on"`
}

func (s *Server) handleToolHibernation(w http.ResponseWriter, r *http.Request) {
	var req hibReq
	json.NewDecoder(r.Body).Decode(&req)
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.SetHibernation(req.On))
}

func (s *Server) handleToolDefender(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.DefenderQuickScan())
}

type bloatRemoveReq struct {
	Pacotes []string `json:"pacotes"`
}

// handleBloatRemove remove os pacotes selecionados (reversivel via Store).
func (s *Server) handleBloatRemove(w http.ResponseWriter, r *http.Request) {
	var req bloatRemoveReq
	json.NewDecoder(r.Body).Decode(&req)
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.RemoveBloat(req.Pacotes))
}

type driverCleanReq struct {
	Pasta string `json:"pasta"`
}

// handleDriverClean apaga a pasta de residuos do instalador (seguro: temp).
func (s *Server) handleDriverClean(w http.ResponseWriter, r *http.Request) {
	var req driverCleanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "body inválido"})
		return
	}
	// Valida que a pasta é um caminho absoluto com letra de drive (evita paths arbitrários)
	if len(req.Pasta) < 3 || req.Pasta[1] != ':' {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "pasta inválida"})
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	err := winutil.CleanDriverLeftovers(req.Pasta)
	resp := map[string]any{"ok": err == nil}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// ---- Backup & Restauracao ---------------------------------------------------

func (s *Server) handleBackupList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, engine.ListBackups())
}

func (s *Server) handleBackupCreate(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := engine.BuildBackup(s.rules, engine.BuildCtx())
	err := engine.SaveBackup(b)
	resp := map[string]any{"ok": err == nil, "id": b.ID, "lista": engine.ListBackups()}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

type backupReq struct {
	ID string `json:"id"`
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	var req backupReq
	json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()
	n, falhas, err := engine.RestoreBackup(req.ID, winutil.RealUserSid())
	resp := map[string]any{"ok": err == nil, "itens": n, "falhas": falhas, "scan": engine.Scan(s.rules, engine.BuildCtx())}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleBackupDelete(w http.ResponseWriter, r *http.Request) {
	var req backupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "body inválido"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := engine.DeleteBackup(req.ID)
	resp := map[string]any{"ok": err == nil, "lista": engine.ListBackups()}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// ---- Otimização por jogo ----------------------------------------------------

func (s *Server) handleGames(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.DetectGames(winutil.RealUserSid()))
}

type gameTweakReq struct {
	Exe   string `json:"exe"`
	Pasta string `json:"pasta"`
	FSO   bool   `json:"fso"`
	GPU   bool   `json:"gpu"`
	Prio  bool   `json:"prio"`
	AV    bool   `json:"av"`
}

func (s *Server) handleGameTweak(w http.ResponseWriter, r *http.Request) {
	var req gameTweakReq
	json.NewDecoder(r.Body).Decode(&req)
	// C1: validar .exe antes de tocar no registro — rejeita paths fantasmas
	if req.Exe != "" {
		if fi, err := os.Stat(req.Exe); err != nil || fi.IsDir() {
			writeJSON(w, 200, map[string]any{"ok": false, "erro": fmt.Sprintf("arquivo não encontrado: %s", req.Exe)})
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := winutil.SetGameTweaks(winutil.RealUserSid(), req.Exe, req.Pasta, req.FSO, req.GPU, req.Prio, req.AV)
	resp := map[string]any{"ok": err == nil}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// F14: exporta a lista de jogos com estado de tweaks como JSON.
func (s *Server) handleGamesExport(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.DetectGames(winutil.RealUserSid()))
}

// F14: importa tweaks de jogos; aplica somente nos jogos cujo .exe existe na máquina.
func (s *Server) handleGamesImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Jogos []winutil.Game `json:"jogos"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()
	var aplicados, ignorados int
	for _, g := range req.Jogos {
		if g.Exe == "" {
			ignorados++
			continue
		}
		if fi, err := os.Stat(g.Exe); err != nil || fi.IsDir() {
			ignorados++ // jogo não instalado nesta máquina
			continue
		}
		if err := winutil.SetGameTweaks(winutil.RealUserSid(), g.Exe, g.Pasta, g.FSO, g.GPU, g.Prio, g.AV); err == nil {
			aplicados++
		} else {
			ignorados++
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "aplicados": aplicados, "ignorados": ignorados})
}

// F1: Modo Game ao Vivo — retorna jogos rodando agora + estado do auto-mode.
func (s *Server) handleModoGameStatus(w http.ResponseWriter, _ *http.Request) {
	s.gameMu.Lock()
	defer s.gameMu.Unlock()
	sid := winutil.RealUserSid()
	// cache da lista de jogos: atualiza a cada 60s (DetectGames é custoso)
	if time.Since(s.gameCacheTime) > 60*time.Second {
		s.gameCache = winutil.DetectGames(sid)
		s.gameCacheTime = time.Now()
	}
	live := winutil.LiveGames(s.gameCache)
	if live == nil {
		live = []winutil.Game{}
	}
	writeJSON(w, 200, map[string]any{
		"ativo":   s.gameAutoMode,
		"rodando": live,
	})
}

// F1: ativa/desativa o Modo Game ao Vivo.
func (s *Server) handleModoGameSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ativo bool `json:"ativo"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	s.gameMu.Lock()
	s.gameAutoMode = req.Ativo
	// força refresh do cache na próxima poll
	s.gameCacheTime = time.Time{}
	s.gameMu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true, "ativo": req.Ativo})
}

// F11: lista serviços não-essenciais para jogos com estado rodando/parado.
func (s *Server) handleServicos(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.HeavyServices())
}

// F11: para um serviço (sem desabilitar permanentemente).
func (s *Server) handleServicoParar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nome string `json:"nome"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Nome == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": "nome vazio"})
		return
	}
	err := winutil.StopServiceNow(req.Nome)
	resp := map[string]any{"ok": err == nil}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// F12: audita drivers instalados e detecta os mais antigos.
func (s *Server) handleDriversVerificar(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.DriversAudit())
}

// F2: Modo Turbo — energia max + rede limpa + TRIM em uma chamada.
func (s *Server) handleTurbo(w http.ResponseWriter, _ *http.Request) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	energia := winutil.UltimatePerformance()
	rede := winutil.NetworkFlush()
	trim := winutil.OptimizeSSDs()
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"energia": energia,
		"rede":    rede,
		"trim":    trim,
	})
}

// handleToolDNS aplica um provedor de DNS em todas as interfaces ativas.
func (s *Server) handleToolDNS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, 200, winutil.SetDNS(req.Provider))
}

// handleDNSProviders lista os provedores de DNS disponíveis.
func (s *Server) handleDNSProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.DNSProviders)
}

// handleGPUPanel retorna os dados NVIDIA via nvidia-smi.
func (s *Server) handleGPUPanel(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.NvidiaPanel())
}

// handleGPUPower define o power limit da GPU NVIDIA em watts.
func (s *Server) handleGPUPower(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Watts  int    `json:"watts"`
		Action string `json:"action"` // "set" | "reset"
	}
	json.NewDecoder(r.Body).Decode(&req)
	var result map[string]any
	if req.Action == "reset" {
		result = winutil.NvidiaResetPower()
	} else {
		result = winutil.NvidiaSetPower(req.Watts)
	}
	writeJSON(w, 200, result)
}

// ---- Sprint 2: Browser Cleaner -----------------------------------------------

func (s *Server) handleBrowserScan(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "browsers": winutil.BrowserScan(winutil.RealUserSid())})
}

type browserLimparReq struct {
	Reqs []winutil.BrowserCleanReq `json:"reqs"`
}

func (s *Server) handleBrowserLimpar(w http.ResponseWriter, r *http.Request) {
	var req browserLimparReq
	json.NewDecoder(r.Body).Decode(&req)
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.BrowserClean(winutil.RealUserSid(), req.Reqs))
}

// ---- Sprint 2: Hosts Editor --------------------------------------------------

func (s *Server) handleHostsList(w http.ResponseWriter, _ *http.Request) {
	entries, err := winutil.HostsList()
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "entries": entries})
}

type hostsEntryReq struct {
	IP      string `json:"ip"`
	Host    string `json:"host"`
	Comment string `json:"comment"`
}

func (s *Server) handleHostsAdicionar(w http.ResponseWriter, r *http.Request) {
	var req hostsEntryReq
	json.NewDecoder(r.Body).Decode(&req)
	err := winutil.HostsAdd(req.IP, req.Host, req.Comment)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleHostsRemover(w http.ResponseWriter, r *http.Request) {
	var req hostsEntryReq
	json.NewDecoder(r.Body).Decode(&req)
	err := winutil.HostsRemove(req.IP, req.Host)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleHostsToggle(w http.ResponseWriter, r *http.Request) {
	var req hostsEntryReq
	json.NewDecoder(r.Body).Decode(&req)
	err := winutil.HostsToggle(req.IP, req.Host)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- Sprint 2: Pinger --------------------------------------------------------

func (s *Server) handlePingPresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "presets": winutil.PingPresets()})
}

type pingReq struct {
	Host string `json:"host"`
}

func (s *Server) handlePing2(w http.ResponseWriter, r *http.Request) {
	var req pingReq
	json.NewDecoder(r.Body).Decode(&req)
	if req.Host == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": "host vazio"})
		return
	}
	result := winutil.PingHost(req.Host)
	writeJSON(w, 200, map[string]any{"ok": true, "resultado": result})
}

// ---- Sprint 2: Windows Update ------------------------------------------------

func (s *Server) handleWUStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.WUStatus())
}

func (s *Server) handleWUPausar(w http.ResponseWriter, _ *http.Request) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.WUPausar())
}

func (s *Server) handleWURetomar(w http.ResponseWriter, _ *http.Request) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	writeJSON(w, 200, winutil.WURetomar())
}

// ---- Sprint 3: AMD GPU Panel -------------------------------------------------

func (s *Server) handleGPUAmd(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.AMDPanel())
}

// ---- Sprint 3: Process Affinity Manager --------------------------------------

func (s *Server) handleListProcesses(w http.ResponseWriter, _ *http.Request) {
	procs := winutil.ListProcesses()
	writeJSON(w, 200, map[string]any{"ok": true, "processos": procs})
}

type affinitySetReq struct {
	PID  uint32 `json:"pid"`
	Mask uint64 `json:"mask"`
}

func (s *Server) handleSetAffinity(w http.ResponseWriter, r *http.Request) {
	var req affinitySetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "body inválido"})
		return
	}
	if req.PID == 0 || req.Mask == 0 {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "pid e mask obrigatórios"})
		return
	}
	err := winutil.SetProcessAffinity(req.PID, req.Mask)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- Sprint 3: DPC Latency ---------------------------------------------------

func (s *Server) handleDPC(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.MeasureDPC())
}

// ---- Sprint 3: Custom Presets ------------------------------------------------

func (s *Server) handleCustomPresets(w http.ResponseWriter, _ *http.Request) {
	sid := winutil.RealUserSid()
	presets := winutil.ListCustomPresets(sid)
	writeJSON(w, 200, map[string]any{"ok": true, "presets": presets})
}

type customPresetSalvarReq struct {
	ID        string   `json:"id"`
	Nome      string   `json:"nome"`
	Descricao string   `json:"descricao"`
	IDs       []string `json:"ids"`
}

func (s *Server) handleCustomPresetSalvar(w http.ResponseWriter, r *http.Request) {
	var req customPresetSalvarReq
	json.NewDecoder(r.Body).Decode(&req)
	sid := winutil.RealUserSid()
	p, err := winutil.SaveCustomPreset(sid, req.ID, req.Nome, req.Descricao, req.IDs)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "preset": p})
}

type customPresetExcluirReq struct {
	ID string `json:"id"`
}

func (s *Server) handleCustomPresetExcluir(w http.ResponseWriter, r *http.Request) {
	var req customPresetExcluirReq
	json.NewDecoder(r.Body).Decode(&req)
	sid := winutil.RealUserSid()
	err := winutil.DeleteCustomPreset(sid, req.ID)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleCustomPresetAplicar(w http.ResponseWriter, r *http.Request) {
	var req customPresetExcluirReq
	json.NewDecoder(r.Body).Decode(&req)
	sid := winutil.RealUserSid()
	ids := winutil.GetCustomPresetIDs(sid, req.ID)
	if ids == nil {
		writeJSON(w, 200, map[string]any{"ok": false, "erro": "preset não encontrado"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := engine.BuildCtx()
	rep := engine.ApplyRules(s.rules, ids, ctx, true, "preset-custom:"+req.ID)
	writeJSON(w, 200, map[string]any{"relatorio": rep, "scan": engine.Scan(s.rules, ctx)})
}

// handleReboot reinicia o Windows (acionado pelo usuario, com confirmacao na UI).
func (s *Server) handleReboot(w http.ResponseWriter, _ *http.Request) {
	cmd := exec.Command("shutdown", "/r", "/t", "3")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	err := cmd.Start()
	resp := map[string]any{"ok": err == nil}
	if err != nil {
		resp["erro"] = err.Error()
	}
	writeJSON(w, 200, resp)
}
