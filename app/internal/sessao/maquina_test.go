package sessao

import "testing"

// A maquina de etapas e o coracao da S1: se ela deixar uma etapa "em andamento"
// orfa, aceitar pular sem motivo ou permitir concluir uma etapa bloqueada, o
// relatorio no fim passa a mentir. Por isso ela e testada direto.

func nova(admin bool) *Sessao {
	return Nova("PC-TESTE", Cliente{Nome: "Fulano"}, "competitivo", admin, nil)
}

func status(t *testing.T, s *Sessao, id string) string {
	t.Helper()
	for _, e := range s.Etapas {
		if e.ID == id {
			return e.Status
		}
	}
	t.Fatalf("etapa %q não existe na sessão", id)
	return ""
}

func TestNovaComecaNaPrimeiraEtapa(t *testing.T) {
	s := nova(true)
	if s.Etapa != Etapas[0].ID {
		t.Fatalf("etapa inicial = %q, queria %q", s.Etapa, Etapas[0].ID)
	}
	if got := status(t, s, "medir"); got != StatusEmAndamento {
		t.Errorf("medir = %q, queria %q", got, StatusEmAndamento)
	}
	if got := status(t, s, "planejar"); got != StatusDisponivel {
		t.Errorf("com admin, planejar = %q, queria %q", got, StatusDisponivel)
	}
	if s.Limitada {
		t.Error("com admin a sessão não deveria nascer limitada")
	}
	if s.Intencao != "competitivo" {
		t.Errorf("intenção = %q", s.Intencao)
	}
}

func TestSemAdminBloqueiaEtapasDeEscrita(t *testing.T) {
	s := nova(false)
	if !s.Limitada {
		t.Fatal("sem admin a sessão deveria nascer limitada")
	}
	if got := status(t, s, "aplicar"); got != StatusBloqueada {
		t.Errorf("aplicar = %q, queria %q", got, StatusBloqueada)
	}
	// Planejar continua liberada: montar o plano é leitura + decisão, não escrita.
	// O que exige elevação é executar.
	for _, id := range []string{"medir", "ler", "entender", "planejar", "provar", "entregar"} {
		if got := status(t, s, id); got == StatusBloqueada {
			t.Errorf("%s não deveria estar bloqueada sem admin", id)
		}
	}
}

func TestConcluirAvancaOTrilho(t *testing.T) {
	s := nova(true)
	if err := s.Concluir("medir"); err != nil {
		t.Fatalf("concluir medir: %v", err)
	}
	if got := status(t, s, "medir"); got != StatusConcluida {
		t.Errorf("medir = %q", got)
	}
	if s.Etapa != "ler" {
		t.Errorf("etapa corrente = %q, queria \"ler\"", s.Etapa)
	}
	if got := status(t, s, "ler"); got != StatusEmAndamento {
		t.Errorf("ler = %q, queria %q", got, StatusEmAndamento)
	}
}

func TestSoAEtapaCorrenteAnda(t *testing.T) {
	s := nova(true)
	if err := s.Concluir("aplicar"); err != ErrNaoCorrente {
		t.Errorf("concluir etapa fora de vez: erro = %v, queria %v", err, ErrNaoCorrente)
	}
	if err := s.Concluir("nao-existe"); err != ErrEtapaDesconhecida {
		t.Errorf("etapa inventada: erro = %v, queria %v", err, ErrEtapaDesconhecida)
	}
	if s.Etapa != "medir" {
		t.Errorf("nada disso podia mover o trilho; etapa = %q", s.Etapa)
	}
}

func TestPularExigeMotivo(t *testing.T) {
	s := nova(true)
	if err := s.Pular("medir", "   "); err != ErrMotivo {
		t.Fatalf("pular sem motivo: erro = %v, queria %v", err, ErrMotivo)
	}
	if s.Etapa != "medir" {
		t.Fatal("pular sem motivo não pode avançar o trilho")
	}
	if err := s.Pular("medir", "  cliente sem jogo instalado\n"); err != nil {
		t.Fatalf("pular com motivo: %v", err)
	}
	if got := status(t, s, "medir"); got != StatusPulada {
		t.Errorf("medir = %q, queria %q", got, StatusPulada)
	}
	for _, e := range s.Etapas {
		if e.ID == "medir" && e.Motivo != "cliente sem jogo instalado" {
			t.Errorf("motivo gravado = %q (devia vir limpo, sem espaço nem quebra de linha)", e.Motivo)
		}
	}
}

func TestEtapaBloqueadaPulaMasNaoConclui(t *testing.T) {
	s := nova(false)
	for _, id := range []string{"medir", "ler", "entender", "planejar"} {
		if err := s.Concluir(id); err != nil {
			t.Fatalf("concluir %s: %v", id, err)
		}
	}
	if s.Etapa != "aplicar" {
		t.Fatalf("etapa corrente = %q, queria \"aplicar\"", s.Etapa)
	}
	if err := s.Concluir("aplicar"); err == nil {
		t.Error("concluir etapa bloqueada devia falhar")
	}
	if err := s.Pular("aplicar", "sem admin nesta máquina"); err != nil {
		t.Errorf("pular etapa bloqueada: %v", err)
	}
	if s.Etapa != "provar" {
		t.Errorf("etapa corrente = %q, queria \"provar\"", s.Etapa)
	}
}

func TestConcluirUltimaEncerra(t *testing.T) {
	s := nova(true)
	for _, d := range Etapas {
		if err := s.Concluir(d.ID); err != nil {
			t.Fatalf("concluir %s: %v", d.ID, err)
		}
	}
	if !s.Encerrada {
		t.Fatal("concluir a última etapa devia encerrar a sessão")
	}
	if err := s.Concluir("medir"); err != ErrEncerrada {
		t.Errorf("sessão encerrada: erro = %v, queria %v", err, ErrEncerrada)
	}
}

func TestEncerrarNoMeioNaoApagaOPassado(t *testing.T) {
	s := nova(true)
	s.Concluir("medir")
	s.Pular("ler", "PC já tinha sido escaneado hoje")
	s.Encerrar()
	if !s.Encerrada {
		t.Fatal("sessão devia estar encerrada")
	}
	if got := status(t, s, "medir"); got != StatusConcluida {
		t.Errorf("medir = %q — o que foi feito não pode virar outra coisa", got)
	}
	if got := status(t, s, "ler"); got != StatusPulada {
		t.Errorf("ler = %q — o que foi pulado tem que continuar pulado", got)
	}
	if got := status(t, s, "aplicar"); got != StatusBloqueada {
		t.Errorf("aplicar = %q, queria %q", got, StatusBloqueada)
	}
}
