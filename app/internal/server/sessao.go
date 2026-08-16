package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"thazzdraco/internal/sessao"
	"thazzdraco/internal/winutil"
)

// API da Sessao — o trilho do atendimento (docs/SESSAO.md §6).
//
// Fatia S1: criar/retomar/listar sessao e andar pelas etapas. As rotas de plano,
// evidencia e execucao entram nas fatias seguintes.
//
// Diferente do resto da API (que ainda aceita qualquer metodo — achado 🟠 do
// CHECKUP), aqui tudo que muda estado exige POST. Codigo novo ja nasce no
// padrao certo.

// requirePOST rejeita metodo diferente de POST em rota que muda estado. GET e o
// mais facil de disparar sem querer (navegacao, <img>, prefetch).
func requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"erro": "use POST"})
		return false
	}
	return true
}

// decodeBody le o corpo JSON. Corpo vazio e aceito (o handler decide se faltou
// campo obrigatorio); JSON quebrado vira 400 em vez de struct zerada aplicada em
// silencio.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(r.Body).Decode(dst)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "corpo inválido: " + err.Error()})
	return false
}

// envSessao e a resposta padrao das rotas de sessao: a sessao (ou null) mais o
// catalogo de etapas. Mandar o catalogo junto evita a UI ter uma segunda copia
// dos nomes/resumos das etapas para sair do lugar com o tempo.
func envSessao(s *sessao.Sessao) map[string]any {
	env := map[string]any{"sessao": s, "etapas": sessao.Etapas}
	if s != nil {
		// A comparacao e derivada (nao e gravada): calcular aqui garante que a UI
		// nunca mostre um antes×depois que ela mesma montou por conta propria.
		env["comparacao"] = s.Comparacao()
	}
	return env
}

// handleSessaoAtual devolve a sessao aberta neste PC (ou null). Se havia uma
// sessao e ela nao abre, isso vira aviso — some da tela em silencio, nao.
func (s *Server) handleSessaoAtual(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, err := sessao.AtualDetalhado()
	sess = s.reconciliar(sess) // fase órfã e reinício já executado viram estado real
	resp := envSessao(sess)
	if err != nil {
		resp["aviso"] = "Havia uma sessão aberta, mas não foi possível abri-la: " + err.Error()
	}
	writeJSON(w, 200, resp)
}

type sessaoNovaReq struct {
	Cliente  sessao.Cliente `json:"cliente"`
	Intencao string         `json:"intencao"`
}

// handleSessaoNova abre uma sessao e roda a etapa 0 (Preparar).
func (s *Server) handleSessaoNova(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req sessaoNovaReq
	if !decodeBody(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if atual := sessao.Atual(); atual != nil && !atual.Encerrada {
		writeJSON(w, http.StatusConflict, map[string]any{
			"erro":   "já existe uma sessão aberta neste PC — retome ou encerre antes de abrir outra",
			"sessao": atual, "etapas": sessao.Etapas,
		})
		return
	}
	pc, _ := os.Hostname()
	nova := sessao.Nova(pc, req.Cliente, req.Intencao, winutil.IsAdmin(), winutil.BuildProfile())
	if err := sessao.Salvar(nova); err != nil {
		writeJSON(w, 500, map[string]any{"erro": "não foi possível gravar a sessão: " + err.Error()})
		return
	}
	if err := sessao.DefinirAtual(nova.ID); err != nil {
		writeJSON(w, 500, map[string]any{"erro": "não foi possível marcar a sessão como atual: " + err.Error()})
		return
	}
	writeJSON(w, 200, envSessao(nova))
}

type sessaoIDReq struct {
	ID string `json:"id"`
}

// handleSessaoRetomar reabre uma sessao anterior deste PC.
func (s *Server) handleSessaoRetomar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req sessaoIDReq
	if !decodeBody(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	alvo, err := sessao.Carregar(req.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "sessão não encontrada"})
		return
	}
	if alvo.Encerrada {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "esta sessão já foi encerrada"})
		return
	}
	if err := sessao.DefinirAtual(alvo.ID); err != nil {
		writeJSON(w, 500, map[string]any{"erro": err.Error()})
		return
	}
	writeJSON(w, 200, envSessao(alvo))
}

type sessaoEtapaReq struct {
	Etapa  string `json:"etapa"`
	Acao   string `json:"acao"` // "concluir" | "pular"
	Motivo string `json:"motivo"`
}

// handleSessaoEtapa conclui ou pula a etapa corrente.
func (s *Server) handleSessaoEtapa(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req sessaoEtapaReq
	if !decodeBody(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	atual := sessao.Atual()
	if atual == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "nenhuma sessão aberta"})
		return
	}
	var err error
	switch req.Acao {
	case "concluir":
		err = atual.Concluir(req.Etapa)
	case "pular":
		err = atual.Pular(req.Etapa, req.Motivo)
	default:
		err = errors.New(`ação inválida (use "concluir" ou "pular")`)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": err.Error()})
		return
	}
	if err := sessao.Salvar(atual); err != nil {
		writeJSON(w, 500, map[string]any{"erro": "não foi possível gravar a sessão: " + err.Error()})
		return
	}
	// Concluir a ultima etapa encerra a sessao: solta o ponteiro para o proximo
	// atendimento comecar limpo. O arquivo da sessao fica.
	if atual.Encerrada {
		sessao.LimparAtual()
	}
	writeJSON(w, 200, envSessao(atual))
}

// handleSessaoEncerrar fecha a sessao antes do fim do trilho.
func (s *Server) handleSessaoEncerrar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	atual := sessao.Atual()
	if atual == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "nenhuma sessão aberta"})
		return
	}
	atual.Encerrar()
	if err := sessao.Salvar(atual); err != nil {
		writeJSON(w, 500, map[string]any{"erro": "não foi possível gravar a sessão: " + err.Error()})
		return
	}
	sessao.LimparAtual()
	writeJSON(w, 200, envSessao(atual))
}

type planoGerarReq struct {
	Intencao string `json:"intencao"`
}

// handleSessaoPlanoGerar monta a fila do atendimento a partir da varredura + do
// diagnóstico + do perfil escolhido. É a operação mais cara da sessão (varre
// tudo e roda as sondas), por isso é explícita: o usuário pede.
func (s *Server) handleSessaoPlanoGerar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req planoGerarReq
	if !decodeBody(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	atual := sessao.Atual()
	if atual == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "nenhuma sessão aberta"})
		return
	}
	if atual.Encerrada {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "sessão já encerrada"})
		return
	}
	// Trocar a intenção aqui é legítimo: é em Planejar que se descobre que o
	// perfil escolhido no início não era o certo para aquela máquina.
	if req.Intencao != "" {
		atual.Intencao = sessao.NormalizaIntencao(req.Intencao)
	}

	scan := s.scan()
	atual.Plano = sessao.GerarPlano(sessao.Entradas{
		Intencao: atual.Intencao,
		Regras:   scan.Regras,
		Gargalos: winutil.Gargalos(winutil.RealUserSid()),
		Preset:   s.presetIDs(atual.Intencao),
	})
	if err := sessao.Salvar(atual); err != nil {
		writeJSON(w, 500, map[string]any{"erro": "não foi possível gravar o plano: " + err.Error()})
		return
	}
	writeJSON(w, 200, envSessao(atual))
}

type planoSelecionarReq struct {
	Itens []struct {
		ID          string `json:"id"`
		Selecionado bool   `json:"selecionado"`
	} `json:"itens"`
}

// handleSessaoPlanoSelecionar aplica a marcação vinda da UI e recalcula o resumo.
func (s *Server) handleSessaoPlanoSelecionar(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req planoSelecionarReq
	if !decodeBody(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	atual := sessao.Atual()
	if atual == nil || atual.Plano == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"erro": "nenhum plano gerado nesta sessão"})
		return
	}
	marcas := make(map[string]bool, len(req.Itens))
	for _, it := range req.Itens {
		marcas[it.ID] = it.Selecionado
	}
	atual.Plano.Selecionar(marcas)
	if err := sessao.Salvar(atual); err != nil {
		writeJSON(w, 500, map[string]any{"erro": "não foi possível gravar a seleção: " + err.Error()})
		return
	}
	writeJSON(w, 200, envSessao(atual))
}

// presetIDs devolve os ids do preset da intenção ("livre" não semeia nada).
func (s *Server) presetIDs(intencao string) []string {
	for _, p := range s.presets {
		if p.ID == intencao {
			return p.IDs
		}
	}
	return nil
}

// handleSessoes lista os atendimentos gravados (gancho do histórico por cliente).
func (s *Server) handleSessoes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"sessoes": sessao.Listar(50)})
}
