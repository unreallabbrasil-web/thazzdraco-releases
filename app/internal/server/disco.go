package server

import (
	"net/http"
	"strconv"

	"thazzdraco/internal/winutil"
)

// Explorador de disco — "onde foram parar os 400 GB".
//
// A varredura roda em goroutine e a UI faz poll, como o reparo e o medidor de
// FPS. Nao pega s.mu: e leitura pura, nao mexe em nada, e travar o motor por
// alguns minutos de varredura seria absurdo.
//
// A arvore NAO e devolvida inteira: um disco cheio viraria dezenas de MB de
// JSON. A UI pede um nivel por vez, ja ordenado do maior para o menor.

type discoVarrerReq struct {
	Raiz string `json:"raiz"`
}

// handleDiscoVarrer dispara a varredura de uma unidade.
func (s *Server) handleDiscoVarrer(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req discoVarrerReq
	if !decodeBody(w, r, &req) {
		return
	}
	if err := winutil.DiscoMgr().Iniciar(req.Raiz); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"erro": "não foi possível varrer " + req.Raiz + ": " + err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "raiz": req.Raiz})
}

// handleDiscoStatus devolve o andamento (ou o resumo, quando termina).
func (s *Server) handleDiscoStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, winutil.DiscoMgr().Status())
}

// handleDiscoCancelar interrompe a varredura em andamento.
func (s *Server) handleDiscoCancelar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	winutil.DiscoMgr().Cancelar()
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleDiscoAchados classifica o que a varredura encontrou: o que dá para
// limpar, o que é dado do usuário (só relatório) e o que parece lixo mas não é.
func (s *Server) handleDiscoAchados(w http.ResponseWriter, _ *http.Request) {
	ach, ok := winutil.DiscoMgr().Achados(winutil.RealUserSid())
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"erro": "varra uma unidade primeiro — a classificação usa os tamanhos já medidos"})
		return
	}
	writeJSON(w, 200, ach)
}

type discoLimparReq struct {
	IDs       []string `json:"ids"`
	Confirmar bool     `json:"confirmar"`
}

// handleDiscoLimpar apaga o conteúdo das categorias escolhidas.
//
// Recebe IDS DE CATEGORIA, nunca caminho: não existe caminho de código em que um
// caminho vindo do cliente chegue à exclusão. E só categoria da classe
// "limpável" é executada — o resto é recusado com o motivo, mesmo que a UI peça.
func (s *Server) handleDiscoLimpar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req discoLimparReq
	if !decodeBody(w, r, &req) {
		return
	}
	s.opMu.Lock() // serializa com as outras operações destrutivas pesadas
	defer s.opMu.Unlock()
	res, err := winutil.Limpar(winutil.RealUserSid(), req.IDs, req.Confirmar)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

type discoAbrirReq struct {
	Caminho string `json:"caminho"`
}

// handleDiscoAbrir abre no Explorer uma pasta/arquivo que a classificação
// encontrou. Só aceita caminho que o próprio backend acabou de listar — senão
// seria um endpoint que abre qualquer coisa que o cliente pedir.
func (s *Server) handleDiscoAbrir(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req discoAbrirReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !winutil.DiscoMgr().CaminhoDeAchado(winutil.RealUserSid(), req.Caminho) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"erro": "este caminho não veio da classificação do disco"})
		return
	}
	if err := winutil.AbrirNoExplorer(req.Caminho); err != nil {
		writeJSON(w, 500, map[string]any{"erro": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleDiscoArvore devolve os filhos de uma pasta, do maior para o menor.
func (s *Server) handleDiscoArvore(w http.ResponseWriter, r *http.Request) {
	caminho := r.URL.Query().Get("caminho")
	limite, _ := strconv.Atoi(r.URL.Query().Get("limite"))
	arv, ok := winutil.DiscoMgr().Arvore(caminho, limite)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"erro": "não há varredura pronta para este caminho — varra a unidade primeiro"})
		return
	}
	writeJSON(w, 200, arv)
}
