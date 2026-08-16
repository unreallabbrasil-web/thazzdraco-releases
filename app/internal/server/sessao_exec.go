package server

import (
	"net/http"
	"strconv"
	"sync/atomic"

	"thazzdraco/internal/engine"
	"thazzdraco/internal/sessao"
	"thazzdraco/internal/winutil"
)

// Execucao de uma fase do plano (docs/SESSAO.md §6.1).
//
// Divisao de trabalho: o pacote sessao decide o QUE entra na fila e como julgar
// cada resultado; aqui fica o que precisa do mundo real — o cadeado do motor, o
// engine.ApplyRules e a varredura de conferencia.
//
// A fase roda numa goroutine segurando s.mu (o mesmo cadeado de /api/aplicar),
// para nunca haver duas escritas no sistema ao mesmo tempo. Por isso o poll de
// status NAO pega s.mu: ele so le o arquivo da sessao, e continuaria respondendo
// mesmo com a fase em andamento.

type aplicarFaseReq struct {
	Fase      int  `json:"fase"`
	Confirmar bool `json:"confirmar"`
}

// handleSessaoAplicar dispara a execução de uma fase e volta na hora. O
// andamento sai por /api/sessao/status.
func (s *Server) handleSessaoAplicar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req aplicarFaseReq
	if !decodeBody(w, r, &req) {
		return
	}
	if atomic.LoadInt32(&s.execAtiva) == 1 {
		writeJSON(w, http.StatusConflict, map[string]any{"erro": sessao.ErrJaRodando.Error()})
		return
	}

	s.mu.Lock()
	var ids []string
	sess, err := sessao.Transacao(func(sess *sessao.Sessao) error {
		var e error
		ids, e = sess.IniciarExecucao(req.Fase, req.Confirmar)
		return e
	})
	s.mu.Unlock()
	if err != nil {
		code := http.StatusBadRequest
		if err == sessao.ErrJaRodando {
			code = http.StatusConflict
		}
		writeJSON(w, code, map[string]any{"erro": err.Error()})
		return
	}

	atomic.StoreInt32(&s.execAtiva, 1)
	go s.rodarFase(req.Fase, ids, req.Confirmar)
	writeJSON(w, 200, envSessao(sess))
}

// rodarFase aplica a fase inteira em UM lote (um ponto de restauração, um
// registro no histórico → o undo por lote continua fazendo sentido) e depois
// RELÊ o sistema para conferir item a item o que de fato pegou.
func (s *Server) rodarFase(fase int, ids []string, confirmar bool) {
	defer atomic.StoreInt32(&s.execAtiva, 0)
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := engine.BuildCtx()
	rep := engine.ApplyRulesProgresso(s.rules, ids, ctx, confirmar, "sessao:fase"+strconv.Itoa(fase),
		func(_, titulo string, feitos, total int) {
			sessao.Transacao(func(sess *sessao.Sessao) error {
				sess.Andamento(titulo, feitos, total)
				return nil
			})
		})

	// Conferência: sem esta releitura, "aplicado" seria só a palavra do motor —
	// e uma política de grupo (ou o próprio Windows) reverte valor sem avisar.
	depois := engine.Scan(s.rules, ctx)
	estados := make(map[string]string, len(depois.Regras))
	for _, rv := range depois.Regras {
		estados[rv.ID] = rv.Estado
	}

	sessao.Transacao(func(sess *sessao.Sessao) error {
		sess.ConcluirExecucao(sessao.ResultadoFase{
			Aplicadas: rep.Aplicadas, Puladas: rep.Puladas, Erros: rep.Erros,
			EstadoDepois: estados, BatchID: rep.BatchID, RequerReboot: rep.RequerReboot,
			LimpezaMB: rep.LimpezaMB, Confirmou: confirmar,
		})
		return nil
	})
}

// handleSessaoStatus é o poll do andamento. De propósito NÃO pega s.mu: durante
// a fase esse cadeado está com o executor, e o poll tem que continuar
// respondendo — é ele que mostra a barra andando.
func (s *Server) handleSessaoStatus(w http.ResponseWriter, _ *http.Request) {
	sess := s.reconciliar(sessao.Atual())
	if sess == nil {
		writeJSON(w, 200, map[string]any{"sessao": nil, "execucao": nil})
		return
	}
	writeJSON(w, 200, map[string]any{
		"sessao":   sess,
		"execucao": sess.Execucao,
		"ativa":    atomic.LoadInt32(&s.execAtiva) == 1,
	})
}

// reconciliar acerta o que o mundo mudou enquanto o app estava fora do ar: uma
// fase que ficou "rodando" sem ninguém rodando, e um reinício que a sessão pediu
// e o PC de fato executou. Roda em toda leitura da sessão.
func (s *Server) reconciliar(sess *sessao.Sessao) *sessao.Sessao {
	sess = s.reconciliaExecucao(sess)
	if sess == nil || sess.Reboot == nil || !sess.Reboot.Pendente {
		return sess
	}
	if !sess.ConfirmarReboot(winutil.UptimeSegundos()) {
		return sess
	}
	novo, err := sessao.Transacao(func(x *sessao.Sessao) error {
		x.ConfirmarReboot(winutil.UptimeSegundos())
		return nil
	})
	if err != nil || novo == nil {
		return sess
	}
	return novo
}

type reiniciarReq struct {
	VoltarPara string `json:"voltar_para"`
}

// handleSessaoReiniciar marca a intenção e SÓ ENTÃO manda reiniciar — numa
// chamada só, para não existir uma janela em que o app morre depois de mandar
// reiniciar e antes de gravar por que estava reiniciando.
func (s *Server) handleSessaoReiniciar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req reiniciarReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.VoltarPara == "" {
		req.VoltarPara = "provar" // depois do reinício, o passo natural é medir o resultado
	}
	if _, err := sessao.Transacao(func(x *sessao.Sessao) error {
		return x.MarcarReboot(req.VoltarPara, winutil.UptimeSegundos())
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": err.Error()})
		return
	}
	if err := reiniciarWindows(); err != nil {
		writeJSON(w, 500, map[string]any{"erro": "não foi possível reiniciar: " + err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleSessaoRebootVisto apaga o aviso de "você reiniciou" depois de mostrado.
func (s *Server) handleSessaoRebootVisto(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	sess, err := sessao.Transacao(func(x *sessao.Sessao) error {
		x.RebootVisto()
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": err.Error()})
		return
	}
	writeJSON(w, 200, envSessao(sess))
}

// reconciliaExecucao conserta o estado de uma fase que ficou marcada como
// "rodando" sem ninguém rodando — o app foi fechado (ou o PC reiniciou) no meio.
// É o que impede a sessão de mentir "aplicando…" para sempre.
func (s *Server) reconciliaExecucao(sess *sessao.Sessao) *sessao.Sessao {
	if sess == nil || sess.Execucao == nil || sess.Execucao.Estado != sessao.ExecRodando {
		return sess
	}
	if atomic.LoadInt32(&s.execAtiva) == 1 {
		return sess // é desta janela mesmo, está rodando de verdade
	}
	novo, err := sessao.Transacao(func(x *sessao.Sessao) error {
		x.MarcarInterrompida()
		return nil
	})
	if err != nil || novo == nil {
		return sess
	}
	return novo
}

type feitoNaMaoReq struct {
	ID    string `json:"id"`
	Feito bool   `json:"feito"`
}

// handleSessaoPlanoFeito marca um item de mão (fase 3) como feito. Quem afirma
// que a BIOS foi mexida é a pessoa — o app nunca marca isso sozinho.
func (s *Server) handleSessaoPlanoFeito(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req feitoNaMaoReq
	if !decodeBody(w, r, &req) {
		return
	}
	sess, err := sessao.Transacao(func(x *sessao.Sessao) error {
		return x.MarcarFeitoNaMao(req.ID, req.Feito)
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": err.Error()})
		return
	}
	writeJSON(w, 200, envSessao(sess))
}
