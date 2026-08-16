package sessao

import "errors"

// Regras da maquina de etapas (docs/SESSAO.md §3.1):
//
//   - So avanca para frente. Rever uma etapa passada e assunto da UI — o campo
//     Etapa guarda ate onde a sessao chegou e nunca retrocede, senao o progresso
//     gravado passaria a mentir.
//   - Pular e permitido; esquecer nao. Pular exige motivo, e o motivo fica
//     gravado para entrar no relatorio.
//   - Etapa que exige admin numa sessao Limitada fica bloqueada: da para pular
//     (com motivo), nunca para concluir.
//   - Concluir a ultima etapa encerra a sessao.

var (
	ErrEtapaDesconhecida = errors.New("etapa desconhecida")
	ErrNaoCorrente       = errors.New("só a etapa corrente pode ser concluída ou pulada")
	ErrEncerrada         = errors.New("sessão já encerrada")
	ErrMotivo            = errors.New("escreva o motivo de estar pulando esta etapa")
)

// Nova cria a sessao e roda a etapa 0 (Preparar): hoje ela confere elevacao.
// O ponto de restauracao NAO e criado aqui — quem cria e o motor, imediatamente
// antes da primeira escrita (engine.Apply). Criar um segundo ponto na abertura
// so gastaria a cota diaria do Windows sem proteger nada a mais.
func Nova(pc string, c Cliente, intencao string, admin bool, perfil map[string]any) *Sessao {
	s := &Sessao{
		ID:        novoID(),
		SchemaVer: SchemaVer,
		Criada:    agora(),
		PC:        limpaTexto(pc, 64),
		Cliente:   Cliente{Nome: limpaTexto(c.Nome, 80)},
		Intencao:  normalizaIntencao(intencao),
		Limitada:  !admin,
		Perfil:    perfil,
	}
	for _, d := range Etapas {
		s.Etapas = append(s.Etapas, EtapaState{ID: d.ID, Status: StatusDisponivel})
	}
	s.Etapa = Etapas[0].ID
	s.sincroniza()
	return s
}

// Concluir marca a etapa corrente como feita e avanca o trilho.
func (s *Sessao) Concluir(etapa string) error {
	e, err := s.corrente(etapa)
	if err != nil {
		return err
	}
	if e.Status == StatusBloqueada {
		return errors.New("etapa bloqueada: " + e.Motivo)
	}
	e.Status = StatusConcluida
	e.Quando = agora()
	e.Motivo = ""
	s.avanca()
	return nil
}

// Pular marca a etapa corrente como pulada. Motivo e obrigatorio: e ele que
// impede o relatorio de dar a entender que a etapa aconteceu.
func (s *Sessao) Pular(etapa, motivo string) error {
	e, err := s.corrente(etapa)
	if err != nil {
		return err
	}
	motivo = limpaTexto(motivo, 200)
	if motivo == "" {
		return ErrMotivo
	}
	e.Status = StatusPulada
	e.Quando = agora()
	e.Motivo = motivo
	s.avanca()
	return nil
}

// Encerrar fecha a sessao antes do fim do trilho (o tecnico desistiu, o cliente
// chamou, o PC foi embora). O que ficou por fazer continua registrado como tal.
func (s *Sessao) Encerrar() {
	s.Encerrada = true
	s.sincroniza()
}

// corrente valida que a etapa pedida existe, e a corrente e que a sessao esta
// aberta. Devolve o ponteiro para o estado dela.
func (s *Sessao) corrente(etapa string) (*EtapaState, error) {
	if s.Encerrada {
		return nil, ErrEncerrada
	}
	if _, ok := DefEtapa(etapa); !ok {
		return nil, ErrEtapaDesconhecida
	}
	if etapa != s.Etapa {
		return nil, ErrNaoCorrente
	}
	for i := range s.Etapas {
		if s.Etapas[i].ID == etapa {
			return &s.Etapas[i], nil
		}
	}
	return nil, ErrEtapaDesconhecida
}

// avanca move a etapa corrente para a proxima do catalogo. Concluir/pular a
// ultima encerra a sessao.
func (s *Sessao) avanca() {
	i := indiceEtapa(s.Etapa)
	if i < 0 || i >= len(Etapas)-1 {
		s.Encerrada = true
		s.sincroniza()
		return
	}
	s.Etapa = Etapas[i+1].ID
	s.sincroniza()
}

// sincroniza recalcula o status de tudo que ainda nao aconteceu. Recalcular (em
// vez de mexer em cada transicao) mantem o estado sempre coerente com Etapa,
// Limitada e Encerrada — nao ha como uma etapa ficar "em andamento" orfa.
func (s *Sessao) sincroniza() {
	for i := range s.Etapas {
		e := &s.Etapas[i]
		if e.Status == StatusConcluida || e.Status == StatusPulada {
			continue // o passado nao muda
		}
		def, _ := DefEtapa(e.ID)
		switch {
		case s.Encerrada:
			e.Status = StatusBloqueada
			e.Motivo = "sessão encerrada"
		case def.ExigeAdmin && s.Limitada:
			e.Status = StatusBloqueada
			e.Motivo = MotivoSemAdmin
		case e.ID == s.Etapa:
			e.Status = StatusEmAndamento
			e.Motivo = ""
		default:
			e.Status = StatusDisponivel
			e.Motivo = ""
		}
	}
	s.Atualizada = agora()
}

func indiceEtapa(id string) int {
	for i, d := range Etapas {
		if d.ID == id {
			return i
		}
	}
	return -1
}
