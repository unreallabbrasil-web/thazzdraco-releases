package sessao

import (
	"errors"
	"fmt"
	"strings"
)

// Execucao de uma fase do plano (docs/SESSAO.md §6.1).
//
// Aqui fica so a MAQUINA da execucao: o que entra na fila, como o resultado de
// cada item e julgado e como um andamento morto e reconciliado. Quem fala com o
// Windows e o server (que segura o cadeado do motor e chama engine.ApplyRules).
// Essa separacao e o que deixa a regra mais importante — "aplicado so depois de
// reler" — ser testada sem tocar no registro.

// Estados de um item do plano.
const (
	ItemPendente     = "pendente"
	ItemAplicando    = "aplicando"
	ItemAplicado     = "aplicado"
	ItemFalhou       = "falhou"
	ItemPulado       = "pulado"
	ItemFeitoNaMao   = "feito"        // item de mão que o técnico confirmou ter feito
	ItemDesconhecido = "desconhecido" // execução morreu no meio: não dá para afirmar nada
)

// Estados de uma execucao.
const (
	ExecRodando      = "rodando"
	ExecConcluida    = "concluida"
	ExecInterrompida = "interrompida"
)

// MsgNaoPersistiu e o texto de quando a escrita foi feita mas a releitura nao
// confirmou. E o coracao da verificacao pos-escrita: sem ela, o app poe um ✓ em
// cima de uma politica de grupo que reverteu o valor.
const MsgNaoPersistiu = "a escrita não persistiu — política de grupo, permissão ou o próprio Windows reverteu"

type Execucao struct {
	Fase         int     `json:"fase"`
	Estado       string  `json:"estado"`
	Iniciada     string  `json:"iniciada"`
	Terminada    string  `json:"terminada,omitempty"`
	Atual        string  `json:"atual"` // o que está sendo feito agora
	Feitos       int     `json:"feitos"`
	Total        int     `json:"total"`
	BatchID      string  `json:"batch_id,omitempty"` // lote no histórico → undo
	RequerReboot bool    `json:"requer_reboot"`
	Aplicados    int     `json:"aplicados"`
	Falhas       int     `json:"falhas"`
	Pulados      int     `json:"pulados"`
	LimpezaMB    float64 `json:"limpeza_mb,omitempty"`
	Mensagem     string  `json:"mensagem,omitempty"`
}

var (
	ErrSemPlano         = errors.New("nenhum plano gerado nesta sessão")
	ErrFaseInvalida     = errors.New("fase inválida (use 1 ou 2)")
	ErrFaseVazia        = errors.New("nenhum item marcado nesta fase")
	ErrJaRodando        = errors.New("já existe uma fase em execução")
	ErrPrecisaAdmin     = errors.New("aplicar exige administrador — reabra o app elevado")
	ErrPrecisaConfirmar = errors.New("há itens que pedem confirmação explícita")
)

// ItensDaFase devolve os itens selecionados de uma fase, na ordem do plano.
func (p *Plano) ItensDaFase(fase int) []*PlanoItem {
	var out []*PlanoItem
	for i := range p.Itens {
		it := &p.Itens[i]
		if it.Fase == fase && it.Selecionado && it.Selecionavel {
			out = append(out, it)
		}
	}
	return out
}

// FasesPendentes lista as fases que ainda tem item marcado esperando execucao.
func (p *Plano) FasesPendentes() []int {
	var out []int
	for _, f := range []int{FaseDireta, FaseReboot} {
		for _, it := range p.ItensDaFase(f) {
			if it.Estado == ItemPendente || it.Estado == ItemAplicando || it.Estado == ItemDesconhecido {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// PedemConfirmacao lista os itens da fase que exigem consentimento explicito.
func (p *Plano) PedemConfirmacao(fase int) []*PlanoItem {
	var out []*PlanoItem
	for _, it := range p.ItensDaFase(fase) {
		if it.RequerConsentimento {
			out = append(out, it)
		}
	}
	return out
}

// IniciarExecucao valida e abre a execucao de uma fase. Devolve os ids das
// regras a aplicar, na ordem do plano.
func (s *Sessao) IniciarExecucao(fase int, confirmar bool) ([]string, error) {
	if s.Encerrada {
		return nil, ErrEncerrada
	}
	if s.Limitada {
		return nil, ErrPrecisaAdmin // escrever é a única coisa que a sessão limitada não faz
	}
	if s.Plano == nil {
		return nil, ErrSemPlano
	}
	if fase != FaseDireta && fase != FaseReboot {
		return nil, ErrFaseInvalida // fase 3 é de mão: o app não executa
	}
	if s.Execucao != nil && s.Execucao.Estado == ExecRodando {
		return nil, ErrJaRodando
	}
	itens := s.Plano.ItensDaFase(fase)
	if len(itens) == 0 {
		return nil, ErrFaseVazia
	}
	if !confirmar && len(s.Plano.PedemConfirmacao(fase)) > 0 {
		return nil, ErrPrecisaConfirmar
	}

	ids := make([]string, 0, len(itens))
	for _, it := range itens {
		it.Estado = ItemAplicando
		it.Erro = ""
		ids = append(ids, it.Ref)
	}
	s.Execucao = &Execucao{
		Fase: fase, Estado: ExecRodando, Iniciada: agora(),
		Total: len(itens), Atual: "Criando ponto de restauração…",
	}
	s.Atualizada = agora()
	return ids, nil
}

// Andamento atualiza o item corrente (chamado pelo progresso do motor).
func (s *Sessao) Andamento(titulo string, feitos, total int) {
	if s.Execucao == nil || s.Execucao.Estado != ExecRodando {
		return
	}
	s.Execucao.Atual = titulo
	s.Execucao.Feitos = feitos
	if total > 0 {
		s.Execucao.Total = total
	}
	s.Atualizada = agora()
}

// ResultadoFase e o que o server colhe depois de aplicar: o relatorio do motor
// mais a varredura de CONFERENCIA (estado de cada regra depois da escrita).
type ResultadoFase struct {
	Aplicadas    []string
	Puladas      []string
	Erros        map[string]string
	EstadoDepois map[string]string // ruleID → estado relido
	BatchID      string
	RequerReboot bool
	LimpezaMB    float64
	Confirmou    bool
}

// ConcluirExecucao julga item por item e fecha a fase.
func (s *Sessao) ConcluirExecucao(res ResultadoFase) {
	if s.Execucao == nil || s.Plano == nil {
		return
	}
	aplicadas := paraSet(res.Aplicadas)
	puladas := paraSet(res.Puladas)

	ex := s.Execucao
	ex.Aplicados, ex.Falhas, ex.Pulados = 0, 0, 0
	for _, it := range s.Plano.ItensDaFase(ex.Fase) {
		estado, msg := AvaliarEscrita(it, aplicadas[it.Ref], puladas[it.Ref],
			res.Erros[it.Ref], res.EstadoDepois[it.Ref], res.Confirmou)
		it.Estado, it.Erro = estado, msg
		if estado == ItemAplicado {
			it.BatchID = res.BatchID
			ex.Aplicados++
		} else if estado == ItemFalhou {
			ex.Falhas++
		} else if estado == ItemPulado {
			ex.Pulados++
		}
	}
	ex.Estado = ExecConcluida
	ex.Terminada = agora()
	ex.Feitos = ex.Total
	ex.Atual = ""
	ex.BatchID = res.BatchID
	ex.RequerReboot = res.RequerReboot
	ex.LimpezaMB = res.LimpezaMB
	ex.Mensagem = resumoExecucao(ex)
	s.Atualizada = agora()
}

func resumoExecucao(ex *Execucao) string {
	p := []string{fmt.Sprintf("%d aplicados", ex.Aplicados)}
	if ex.Falhas > 0 {
		p = append(p, fmt.Sprintf("%d falharam", ex.Falhas))
	}
	if ex.Pulados > 0 {
		p = append(p, fmt.Sprintf("%d pulados", ex.Pulados))
	}
	if ex.LimpezaMB > 0 {
		p = append(p, fmt.Sprintf("%.0f MB liberados", ex.LimpezaMB))
	}
	return strings.Join(p, " · ")
}

// AvaliarEscrita decide o estado final de um item.
//
// A regra que importa: "o motor disse que aplicou" NAO basta. So vira aplicado
// se a releitura confirmar. Caso contrario e falha com o motivo — nunca um ✓ em
// cima de uma escrita que o Windows desfez.
func AvaliarEscrita(it *PlanoItem, aplicada, pulada bool, erro, estadoDepois string, confirmou bool) (string, string) {
	if erro != "" {
		return ItemFalhou, erro
	}
	if pulada {
		switch {
		case estadoDepois == "aplicado":
			return ItemPulado, "já estava aplicado"
		case estadoDepois == "nao-aplicavel":
			return ItemPulado, "não se aplica a este hardware"
		case it.RequerConsentimento && !confirmou:
			return ItemPulado, "faltou a confirmação explícita"
		}
		return ItemPulado, "não foi aplicado"
	}
	if !aplicada {
		// nem aplicada, nem pulada, nem com erro: o motor não chegou nela
		return ItemDesconhecido, "a execução não chegou neste item"
	}
	// Limpeza nao tem estado "aplicado" (ela sempre pode rodar de novo), entao
	// nao ha o que reconferir: o motor devolve os MB liberados.
	if it.Tipo == "limpeza" {
		return ItemAplicado, ""
	}
	if estadoDepois != "" && estadoDepois != "aplicado" {
		return ItemFalhou, MsgNaoPersistiu
	}
	return ItemAplicado, ""
}

// MarcarInterrompida reconcilia uma execucao que ficou "rodando" sem ninguem
// rodando: o app foi fechado (ou o PC reiniciou) no meio da fase. O que foi
// escrito ate ali continua no historico e da para desfazer — o que nao da e
// afirmar em que pe cada item ficou, entao nao afirmamos.
func (s *Sessao) MarcarInterrompida() bool {
	if s.Execucao == nil || s.Execucao.Estado != ExecRodando {
		return false
	}
	s.Execucao.Estado = ExecInterrompida
	s.Execucao.Terminada = agora()
	s.Execucao.Atual = ""
	s.Execucao.Mensagem = "a execução foi interrompida (o app fechou ou o PC reiniciou no meio). " +
		"O que já foi escrito está no Histórico e pode ser desfeito. Rodar a fase de novo é seguro: " +
		"o motor pula o que já está aplicado."
	if s.Plano != nil {
		for i := range s.Plano.Itens {
			if s.Plano.Itens[i].Estado == ItemAplicando {
				s.Plano.Itens[i].Estado = ItemDesconhecido
				s.Plano.Itens[i].Erro = "a execução foi interrompida antes de confirmar este item"
			}
		}
	}
	s.Atualizada = agora()
	return true
}

// MarcarFeitoNaMao registra que o tecnico fez (ou desfez) um item de fase 3.
// Item de mao nunca vira "aplicado" pelo app — quem afirma e a pessoa.
func (s *Sessao) MarcarFeitoNaMao(id string, feito bool) error {
	if s.Plano == nil {
		return ErrSemPlano
	}
	for i := range s.Plano.Itens {
		it := &s.Plano.Itens[i]
		if it.ID != id {
			continue
		}
		if it.Selecionavel {
			return errors.New("este item é executado pelo app, não marcado à mão")
		}
		if feito {
			it.Estado = ItemFeitoNaMao
		} else {
			it.Estado = ItemPendente
		}
		s.Atualizada = agora()
		return nil
	}
	return errors.New("item não encontrado no plano")
}

func paraSet(v []string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, x := range v {
		m[x] = true
	}
	return m
}
