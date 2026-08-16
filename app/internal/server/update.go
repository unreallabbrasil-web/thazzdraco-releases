package server

import (
	"net/http"
	"sync"
	"time"

	"thazzdraco/internal/winutil"
)

// GET /api/update/check — retorna info da versão mais recente disponível.
// ?force=1 força uma checagem imediata em vez de usar o cache.
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	var info *winutil.UpdateInfo
	if r.URL.Query().Get("force") == "1" {
		info = winutil.ForceCheck(r.Context(), s.version)
	} else {
		info = winutil.GetUpdate()
	}
	if info == nil {
		// "pending" = ainda não terminou a checagem inicial; JS não deve sobrescrever o estado
		writeJSON(w, 200, map[string]any{"available": false, "pending": true})
		return
	}
	writeJSON(w, 200, info)
}

/* ---------- auto-update (download + self-replace) -------------------------*/

type dlState struct {
	mu         sync.Mutex
	status     string // idle | downloading | ready | installing | error
	downloaded int64
	total      int64
	tmpExe     string
	errMsg     string
}

var dl = &dlState{status: "idle"}

// POST /api/update/install — inicia o download da nova versão em background.
func (s *Server) handleUpdateInstall(w http.ResponseWriter, r *http.Request) {
	info := winutil.GetUpdate()
	if info == nil || !info.Available || info.DownloadURL == "" {
		writeJSON(w, 400, map[string]any{"error": "sem update disponível"})
		return
	}
	dl.mu.Lock()
	if dl.status == "downloading" {
		dl.mu.Unlock()
		writeJSON(w, 409, map[string]any{"error": "já baixando"})
		return
	}
	dl.status = "downloading"
	dl.downloaded = 0
	dl.total = 0
	dl.tmpExe = ""
	dl.errMsg = ""
	dl.mu.Unlock()

	go func() {
		// Baixa E VERIFICA A ASSINATURA antes de marcar como pronto. O apply troca
		// o próprio exe e relança COMO ADMINISTRADOR — instalar binário não
		// verificado seria entregar a máquina do cliente a quem controlar o
		// repositório de releases.
		tmpExe, err := winutil.BaixarAtualizacaoVerificada(info, func(downloaded, total int64) {
			dl.mu.Lock()
			dl.downloaded = downloaded
			dl.total = total
			dl.mu.Unlock()
		})
		dl.mu.Lock()
		if err != nil {
			dl.status = "error"
			dl.errMsg = err.Error()
		} else {
			dl.status = "ready"
			dl.tmpExe = tmpExe
		}
		dl.mu.Unlock()
	}()

	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/update/progress — retorna o estado atual do download.
func (s *Server) handleUpdateProgress(w http.ResponseWriter, r *http.Request) {
	dl.mu.Lock()
	resp := map[string]any{
		"status":     dl.status,
		"downloaded": dl.downloaded,
		"total":      dl.total,
		"error":      dl.errMsg,
	}
	dl.mu.Unlock()
	writeJSON(w, 200, resp)
}

// POST /api/update/apply — aplica o update baixado: substitui o exe e reinicia.
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	dl.mu.Lock()
	if dl.status != "ready" {
		dl.mu.Unlock()
		writeJSON(w, 400, map[string]any{"error": "download não concluído"})
		return
	}
	tmpExe := dl.tmpExe
	dl.status = "installing"
	dl.mu.Unlock()

	writeJSON(w, 200, map[string]any{"ok": true})

	go func() {
		// Espera qualquer operação do motor (mu) ou destrutiva pesada (opMu) terminar
		// antes de trocar o exe e sair — senão o os.Exit do self-replace abortaria um
		// backup/reparo/limpeza no meio. Nenhum caminho segura os dois locks juntos,
		// então adquiri-los aqui em sequência não gera deadlock.
		s.mu.Lock()
		s.opMu.Lock()
		time.Sleep(150 * time.Millisecond)
		winutil.SelfReplaceAndRestart(tmpExe)
	}()
}
