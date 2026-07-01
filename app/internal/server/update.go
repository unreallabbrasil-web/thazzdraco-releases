package server

import (
	"net/http"

	"thazzdraco/internal/winutil"
)

// GET /api/update/check — retorna info da versão mais recente disponível.
// ?force=1 força uma checagem imediata em vez de usar o cache.
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	var info *winutil.UpdateInfo
	if r.URL.Query().Get("force") == "1" {
		info = winutil.ForceCheck(s.version)
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
