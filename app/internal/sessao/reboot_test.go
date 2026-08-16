package sessao

import "testing"

// O reinício é onde o atendimento costuma se perder: o PC reinicia, o app fecha
// e o "vamos medir o resultado" fica na memória de quem lembrar. Estes testes
// travam as duas afirmações do mecanismo — "reiniciou" e "ainda não reiniciou".

func TestMarcarEConfirmarReboot(t *testing.T) {
	s := Nova("PC", Cliente{}, "equilibrado", true, nil)
	if s.PrecisaReiniciar() {
		t.Error("sem execução nenhuma, não há reinício a pedir")
	}
	if err := s.MarcarReboot("provar", 7200); err != nil {
		t.Fatalf("marcar: %v", err)
	}
	if !s.Reboot.Pendente || s.Reboot.VoltarPara != "provar" {
		t.Fatalf("estado do reboot = %+v", s.Reboot)
	}

	// app reaberto SEM reiniciar: uptime maior que o do pedido
	if s.ConfirmarReboot(7500) {
		t.Error("uptime maior significa que a máquina NÃO reiniciou — não pode confirmar")
	}
	if !s.Reboot.Pendente {
		t.Error("o pedido continua pendente enquanto não reiniciar")
	}

	// agora reiniciou de verdade: o Windows subiu há 40s
	if !s.ConfirmarReboot(40) {
		t.Fatal("uptime menor que o do pedido é reinício — tinha que confirmar")
	}
	if s.Reboot.Pendente || s.Reboot.Confirmado == "" || !s.Reboot.Avisar {
		t.Errorf("depois de confirmar: %+v", s.Reboot)
	}
	if s.ConfirmarReboot(45) {
		t.Error("confirmar duas vezes não pode mexer em nada")
	}

	s.RebootVisto()
	if s.Reboot.Avisar {
		t.Error("o aviso some depois de mostrado")
	}
}

func TestMarcarRebootRecusaEtapaInventada(t *testing.T) {
	s := Nova("PC", Cliente{}, "equilibrado", true, nil)
	if err := s.MarcarReboot("etapa-que-nao-existe", 100); err != ErrEtapaAlvoInvalida {
		t.Errorf("erro = %v, queria %v", err, ErrEtapaAlvoInvalida)
	}
	if s.Reboot != nil {
		t.Error("pedido inválido não pode deixar estado pela metade")
	}
}

func TestPrecisaReiniciarSoDepoisDeUmaFaseQuePediu(t *testing.T) {
	s := comPlano(true, pi("a", "regra", FaseReboot, true))
	if _, err := s.IniciarExecucao(FaseReboot, false); err != nil {
		t.Fatal(err)
	}
	s.ConcluirExecucao(ResultadoFase{
		Aplicadas: []string{"a"}, Erros: map[string]string{},
		EstadoDepois: map[string]string{"a": "aplicado"}, BatchID: "lote-1", RequerReboot: true,
	})
	if !s.PrecisaReiniciar() {
		t.Fatal("a fase reportou que pede reinício — o aviso tem que aparecer")
	}
	s.MarcarReboot("provar", 5000)
	if !s.PrecisaReiniciar() {
		t.Error("pedir não é reiniciar; enquanto não reiniciar, o aviso fica")
	}
	s.ConfirmarReboot(30)
	if s.PrecisaReiniciar() {
		t.Error("depois do reinício confirmado o aviso tem que sumir")
	}
}
