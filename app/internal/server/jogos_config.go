package server

import (
	"encoding/json"
	"net/http"

	"thazzdraco/internal/winutil"
)

// GET /api/jogos/cover?name=<nome> — resolve cover via Steam Search API (com cache)
func (s *Server) handleGameCover(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "name obrigatório"})
		return
	}
	u := winutil.ResolveCoverURL(name)
	writeJSON(w, 200, map[string]any{"ok": true, "url": u})
}

// GET /api/jogos/config?exe=<caminho>
func (s *Server) handleGameConfig(w http.ResponseWriter, r *http.Request) {
	exe := r.URL.Query().Get("exe")
	if exe == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "exe obrigatório"})
		return
	}
	result := winutil.ReadGameConfig(exe)
	writeJSON(w, 200, result)
}

type gameCfgSetReq struct {
	Exe    string            `json:"exe"`
	Values map[string]string `json:"values"`
}

// POST /api/jogos/config/set
func (s *Server) handleGameConfigSet(w http.ResponseWriter, r *http.Request) {
	var req gameCfgSetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "body inválido"})
		return
	}
	if req.Exe == "" || len(req.Values) == 0 {
		writeJSON(w, 400, map[string]any{"ok": false, "erro": "exe e values obrigatórios"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := winutil.WriteGameConfig(req.Exe, req.Values); err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
