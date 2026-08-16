package sessao

import (
	"strings"
	"testing"
)

// O executor decide o que o app AFIRMA ter feito na máquina do cliente. As duas
// afirmações perigosas são "aplicado" (quando o Windows reverteu) e "aplicando"
// (quando ninguém está aplicando). As duas são testadas aqui.

func pi(id, tipo string, fase int, sel bool) PlanoItem {
	return PlanoItem{
		ID: tipo + ":" + id, Ref: id, Tipo: tipo, Titulo: "Item " + id, Fase: fase,
		Selecionavel: tipo == "regra" || tipo == "limpeza", Selecionado: sel,
		Estado: ItemPendente,
	}
}

func comPlano(admin bool, itens ...PlanoItem) *Sessao {
	s := Nova("PC", Cliente{}, "equilibrado", admin, nil)
	s.Plano = &Plano{Itens: itens, BaseAplicados: 1, BasePendentes: 3}
	s.Plano.Recalcular()
	return s
}

func TestIniciarExecucaoValida(t *testing.T) {
	semPlano := Nova("PC", Cliente{}, "equilibrado", true, nil)
	if _, err := semPlano.IniciarExecucao(FaseDireta, false); err != ErrSemPlano {
		t.Errorf("sem plano: erro = %v, queria %v", err, ErrSemPlano)
	}

	limitada := comPlano(false, pi("a", "regra", FaseDireta, true))
	if _, err := limitada.IniciarExecucao(FaseDireta, false); err != ErrPrecisaAdmin {
		t.Errorf("sem admin: erro = %v, queria %v", err, ErrPrecisaAdmin)
	}

	s := comPlano(true, pi("a", "regra", FaseDireta, true), pi("m", "manual", FaseManual, false))
	if _, err := s.IniciarExecucao(FaseManual, false); err != ErrFaseInvalida {
		t.Errorf("fase de mão: erro = %v, queria %v — o app não executa a fase 3", err, ErrFaseInvalida)
	}
	if _, err := s.IniciarExecucao(FaseReboot, false); err != ErrFaseVazia {
		t.Errorf("fase sem item marcado: erro = %v, queria %v", err, ErrFaseVazia)
	}

	ids, err := s.IniciarExecucao(FaseDireta, false)
	if err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if len(ids) != 1 || ids[0] != "a" {
		t.Errorf("ids = %v, queria [a] (o id da REGRA, não o do item)", ids)
	}
	if s.Plano.Itens[0].Estado != ItemAplicando {
		t.Errorf("item = %q, queria %q", s.Plano.Itens[0].Estado, ItemAplicando)
	}
	if _, err := s.IniciarExecucao(FaseDireta, false); err != ErrJaRodando {
		t.Errorf("segunda execução: erro = %v, queria %v", err, ErrJaRodando)
	}
}

func TestConsentimentoBarraAExecucao(t *testing.T) {
	it := pi("x", "regra", FaseDireta, true)
	it.RequerConsentimento = true
	s := comPlano(true, it)

	if _, err := s.IniciarExecucao(FaseDireta, false); err != ErrPrecisaConfirmar {
		t.Fatalf("erro = %v, queria %v — consentimento não pode ser presumido", err, ErrPrecisaConfirmar)
	}
	if _, err := s.IniciarExecucao(FaseDireta, true); err != nil {
		t.Errorf("com confirmação explícita devia rodar: %v", err)
	}
}

func TestAplicadoSoDepoisDeReler(t *testing.T) {
	reg := pi("r", "regra", FaseDireta, true)
	casos := []struct {
		nome                   string
		aplicada, pulada       bool
		erro, depois           string
		querEstado, querTrecho string
	}{
		{"escrita confirmada", true, false, "", "aplicado", ItemAplicado, ""},
		{"escrita não persistiu", true, false, "", "pendente", ItemFalhou, MsgNaoPersistiu},
		{"erro do motor", false, false, "acesso negado", "pendente", ItemFalhou, "acesso negado"},
		{"pulada porque já estava", false, true, "", "aplicado", ItemPulado, "já estava aplicado"},
		{"pulada por hardware", false, true, "", "nao-aplicavel", ItemPulado, "não se aplica"},
		{"nem tocada", false, false, "", "pendente", ItemDesconhecido, "não chegou"},
	}
	for _, c := range casos {
		estado, msg := AvaliarEscrita(&reg, c.aplicada, c.pulada, c.erro, c.depois, true)
		if estado != c.querEstado {
			t.Errorf("[%s] estado = %q, queria %q", c.nome, estado, c.querEstado)
		}
		if c.querTrecho != "" && !strings.Contains(msg, c.querTrecho) {
			t.Errorf("[%s] mensagem = %q, queria conter %q", c.nome, msg, c.querTrecho)
		}
	}

	// limpeza não tem estado "aplicado" para reler — o motor devolve os MB
	limp := pi("c", "limpeza", FaseDireta, true)
	if estado, _ := AvaliarEscrita(&limp, true, false, "", "oportunidade", true); estado != ItemAplicado {
		t.Errorf("limpeza = %q, queria %q", estado, ItemAplicado)
	}

	// consentimento faltando aparece como motivo do pulo
	consent := pi("k", "regra", FaseDireta, true)
	consent.RequerConsentimento = true
	if _, msg := AvaliarEscrita(&consent, false, true, "", "pendente", false); !strings.Contains(msg, "confirmação") {
		t.Errorf("motivo = %q, queria falar de confirmação", msg)
	}
}

func TestConcluirExecucaoContaEAmarraOLote(t *testing.T) {
	s := comPlano(true,
		pi("ok", "regra", FaseDireta, true),
		pi("nao", "regra", FaseDireta, true),
		pi("ja", "regra", FaseDireta, true),
	)
	if _, err := s.IniciarExecucao(FaseDireta, false); err != nil {
		t.Fatal(err)
	}
	s.ConcluirExecucao(ResultadoFase{
		Aplicadas:    []string{"ok", "nao"},
		Puladas:      []string{"ja"},
		Erros:        map[string]string{},
		EstadoDepois: map[string]string{"ok": "aplicado", "nao": "pendente", "ja": "aplicado"},
		BatchID:      "lote-1", Confirmou: true,
	})
	ex := s.Execucao
	if ex.Estado != ExecConcluida {
		t.Errorf("execução = %q", ex.Estado)
	}
	if ex.Aplicados != 1 || ex.Falhas != 1 || ex.Pulados != 1 {
		t.Errorf("contagem = %d aplicados / %d falhas / %d pulados; queria 1/1/1", ex.Aplicados, ex.Falhas, ex.Pulados)
	}
	if it := item(s.Plano, "regra:ok"); it.BatchID != "lote-1" {
		t.Error("item aplicado precisa apontar para o lote — é o que liga ao undo do Histórico")
	}
	if it := item(s.Plano, "regra:nao"); it.BatchID != "" {
		t.Error("item que falhou não pode fingir que tem lote para desfazer")
	}
	if len(s.Plano.FasesPendentes()) != 0 {
		t.Error("fase executada não devia continuar pendente")
	}
}

func TestExecucaoMortaViraInterrompida(t *testing.T) {
	s := comPlano(true, pi("a", "regra", FaseDireta, true), pi("b", "regra", FaseReboot, true))
	if _, err := s.IniciarExecucao(FaseDireta, false); err != nil {
		t.Fatal(err)
	}
	// aqui o app morre no meio: o arquivo fica com estado "rodando"

	if !s.MarcarInterrompida() {
		t.Fatal("execução rodando sem ninguém rodando devia ser reconciliada")
	}
	if s.Execucao.Estado != ExecInterrompida {
		t.Errorf("execução = %q, queria %q", s.Execucao.Estado, ExecInterrompida)
	}
	it := item(s.Plano, "regra:a")
	if it.Estado != ItemDesconhecido {
		t.Errorf("item = %q, queria %q — não dá para afirmar que aplicou", it.Estado, ItemDesconhecido)
	}
	if it.Erro == "" {
		t.Error("item em estado desconhecido precisa dizer por quê")
	}
	if s.MarcarInterrompida() {
		t.Error("reconciliar duas vezes não pode mexer em nada")
	}
	// e a fase volta a ficar pendente, para poder rodar de novo (o motor é idempotente)
	if len(s.Plano.FasesPendentes()) != 2 {
		t.Errorf("fases pendentes = %v, queria as duas", s.Plano.FasesPendentes())
	}
}

func TestItemDeMaoSoAPessoaMarca(t *testing.T) {
	s := comPlano(true, pi("a", "regra", FaseDireta, true), pi("xmp", "manual", FaseManual, false))
	if err := s.MarcarFeitoNaMao("manual:xmp", true); err != nil {
		t.Fatalf("marcar item de mão: %v", err)
	}
	if it := item(s.Plano, "manual:xmp"); it.Estado != ItemFeitoNaMao {
		t.Errorf("item = %q, queria %q", it.Estado, ItemFeitoNaMao)
	}
	if err := s.MarcarFeitoNaMao("regra:a", true); err == nil {
		t.Error("item executado pelo app não pode ser marcado à mão como feito")
	}
	if err := s.MarcarFeitoNaMao("manual:xmp", false); err != nil {
		t.Fatal(err)
	}
	if it := item(s.Plano, "manual:xmp"); it.Estado != ItemPendente {
		t.Errorf("desmarcar devia voltar para %q, veio %q", ItemPendente, it.Estado)
	}
}
