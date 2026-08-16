package sessao

import "errors"

// Continuidade atraves do reinicio (docs/SESSAO.md §4.4).
//
// Metade das regras so vale depois de reiniciar. Hoje isso quebra o
// atendimento: o tecnico reinicia, o app fecha, e o que ia acontecer depois
// (medir o resultado) fica na memoria de quem lembrar. A sessao resolve isso
// guardando a intencao ANTES de mandar reiniciar e reconhecendo, na volta, que
// a maquina de fato reiniciou.
//
// Decisao deliberada: **nada de auto-start**. O binario exige admin pelo
// manifesto, entao um RunOnce/Tarefa Agendada dispararia UAC no logon — e o
// projeto ja briga com SmartScreen/Defender (docs/DISTRIBUICAO-CONFIANCA.md).
// Quando o tecnico reabrir o app, ele retoma. Resolve o problema real sem
// gastar confianca.

type RebootState struct {
	Pendente    bool    `json:"pendente"`
	VoltarPara  string  `json:"voltar_para"`            // etapa a retomar
	Pedido      string  `json:"pedido,omitempty"`       // quando o reinício foi pedido
	UptimeAntes float64 `json:"uptime_antes,omitempty"` // uptime no momento do pedido
	Confirmado  string  `json:"confirmado,omitempty"`   // quando o app viu que reiniciou
	Avisar      bool    `json:"avisar,omitempty"`       // ainda não mostrado ao técnico
}

var ErrEtapaAlvoInvalida = errors.New("etapa de retorno desconhecida")

// MarcarReboot registra a intencao ANTES de reiniciar. Se o app morrer entre o
// registro e o desligamento, o pior caso e a marca ficar pendente sem reinicio —
// e ai o uptime da volta sera MAIOR, entao nada e confirmado por engano.
func (s *Sessao) MarcarReboot(voltarPara string, uptimeAgora float64) error {
	if _, ok := DefEtapa(voltarPara); !ok {
		return ErrEtapaAlvoInvalida
	}
	s.Reboot = &RebootState{
		Pendente: true, VoltarPara: voltarPara,
		Pedido: agora(), UptimeAntes: uptimeAgora,
	}
	s.Atualizada = agora()
	return nil
}

// ConfirmarReboot diz se a maquina reiniciou desde o pedido. O teste e o uptime:
// se o Windows subiu ha menos tempo do que quando pedimos, houve reinicio. Isso
// e mais confiavel que carimbar hora — nao depende de relogio, fuso nem de o
// tecnico ter reiniciado na hora que disse que ia.
//
// Devolve true apenas na transicao (a primeira vez que reconhece), para o
// chamador saber que precisa gravar.
func (s *Sessao) ConfirmarReboot(uptimeAgora float64) bool {
	r := s.Reboot
	if r == nil || !r.Pendente {
		return false
	}
	if uptimeAgora >= r.UptimeAntes {
		return false // a máquina não reiniciou (ainda)
	}
	r.Pendente = false
	r.Confirmado = agora()
	r.Avisar = true
	s.Atualizada = agora()
	return true
}

// RebootVisto apaga o aviso depois que o tecnico o viu.
func (s *Sessao) RebootVisto() {
	if s.Reboot != nil && s.Reboot.Avisar {
		s.Reboot.Avisar = false
		s.Atualizada = agora()
	}
}

// PrecisaReiniciar diz se alguma fase ja executada pediu reinicio e ele ainda
// nao aconteceu. E o que sustenta o aviso "os ajustes so valem depois de
// reiniciar" sem inventar: so aparece se uma execucao real reportou isso.
func (s *Sessao) PrecisaReiniciar() bool {
	if s.Execucao == nil || !s.Execucao.RequerReboot {
		return false
	}
	return s.Reboot == nil || s.Reboot.Confirmado == ""
}
