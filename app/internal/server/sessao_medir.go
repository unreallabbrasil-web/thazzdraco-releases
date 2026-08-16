package server

import (
	"fmt"
	"net/http"

	"thazzdraco/internal/fps"
	"thazzdraco/internal/sessao"
	"thazzdraco/internal/winutil"
)

// Medicao da sessao — etapas Medir e Provar (docs/SESSAO.md §4.2).
//
// Regra que vale mais que a comodidade: os NUMEROS NUNCA VEM DO CLIENTE. O
// benchmark e rodado aqui, e a captura de FPS e lida do gerenciador do
// PresentMon (estado do servidor). A UI so pede "guarde o que voce mediu" — se
// ela pudesse mandar valores, o "ganho comprovado" viraria o que a tela quisesse.

type medirReq struct {
	Fase string `json:"fase"` // "baseline" | "depois"
	Tipo string `json:"tipo"` // "bench" | "fps"
	Jogo string `json:"jogo"` // nome amigável do jogo (opcional, só rótulo)
}

// handleSessaoMedir roda (ou colhe) uma medição e guarda no lado pedido.
func (s *Server) handleSessaoMedir(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req medirReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !sessao.LadoValido(req.Fase) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": `fase inválida (use "baseline" ou "depois")`})
		return
	}

	switch req.Tipo {
	case "bench":
		s.mu.Lock()
		res := winutil.Benchmark() // roda aqui; o cliente não manda número
		s.mu.Unlock()
		sess, err := sessao.Transacao(func(x *sessao.Sessao) error {
			x.GuardarBench(req.Fase, res)
			return nil
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"erro": err.Error()})
			return
		}
		writeJSON(w, 200, envSessao(sess))

	case "fps":
		st := fps.Mgr().Status()
		if st["estado"] != "pronto" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"erro": "não há captura de FPS pronta — meça o FPS no jogo primeiro (Medição → FPS no jogo)"})
			return
		}
		res, ok := st["resultado"].(*fps.FrameStats)
		if !ok || res == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "a última captura não trouxe resultado"})
			return
		}
		larg, alt, hz := winutil.DisplayModo()
		cen := sessao.Cenario{
			Jogo: req.Jogo, Exe: res.Processo, RefreshHz: hz,
			DuracaoS: int(res.DurationS + 0.5),
		}
		if larg > 0 && alt > 0 {
			cen.Resolucao = fmt.Sprintf("%dx%d", larg, alt)
		}
		sess, err := sessao.Transacao(func(x *sessao.Sessao) error {
			x.GuardarFPS(req.Fase, *res, cen)
			return nil
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"erro": err.Error()})
			return
		}
		writeJSON(w, 200, envSessao(sess))

	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": `tipo inválido (use "bench" ou "fps")`})
	}
}

// handleSessaoFPSPronto conta se há uma captura de FPS esperando para ser
// guardada — é o que deixa o painel da sessão oferecer "guardar como baseline"
// logo depois de o técnico medir na tela de Medição, sem digitar nada.
func (s *Server) handleSessaoFPSPronto(w http.ResponseWriter, _ *http.Request) {
	st := fps.Mgr().Status()
	resp := map[string]any{"pronto": st["estado"] == "pronto"}
	if res, ok := st["resultado"].(*fps.FrameStats); ok && res != nil {
		resp["processo"] = res.Processo
		resp["fps_avg"] = res.FPSAvg
		resp["duracao_s"] = res.DurationS
	}
	writeJSON(w, 200, resp)
}
