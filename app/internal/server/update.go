package server

import (
	"net/http"

	"thazzdraco/internal/winutil"
)

// GET /api/update/check — retorna info da versão mais recente disponível.
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	info := winutil.GetUpdate()
	if info == nil {
		writeJSON(w, 200, map[string]any{"available": false})
		return
	}
	writeJSON(w, 200, info)
}
