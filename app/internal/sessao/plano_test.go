package sessao

import (
	"testing"

	"thazzdraco/internal/engine"
	"thazzdraco/internal/winutil"
)

// O planejador decide o que vem marcado no plano de um PC de cliente. Errar
// aqui e marcar sozinho um tweak de risco, ou esconder um gargalo critico —
// os dois quebram promessa do produto. Por isso ele e funcao pura e testada.

func regra(id, tier, impacto string, opts ...func(*engine.RuleView)) engine.RuleView {
	rv := engine.RuleView{
		ID: id, Titulo: "Regra " + id, Tier: tier, Impacto: impacto,
		Modo: "acionavel", Estado: "pendente", Aplicavel: true,
	}
	for _, o := range opts {
		o(&rv)
	}
	return rv
}

func comReboot(rv *engine.RuleView)      { rv.RequerReboot = true }
func comConsent(rv *engine.RuleView)     { rv.RequerConsentimento = true }
func jaAplicada(rv *engine.RuleView)     { rv.Estado = "aplicado"; rv.Aplicavel = false }
func foraDoHardware(rv *engine.RuleView) { rv.Estado = "nao-aplicavel"; rv.Aplicavel = false }
func consultiva(rv *engine.RuleView) {
	rv.Modo = "consultivo"
	rv.Estado = "recomendado"
	rv.Aplicavel = false
}
func limpeza(rv *engine.RuleView) {
	rv.Modo = "limpeza"
	rv.Estado = "oportunidade"
	rv.Aplicavel = false
}

func item(p *Plano, id string) *PlanoItem {
	for i := range p.Itens {
		if p.Itens[i].ID == id {
			return &p.Itens[i]
		}
	}
	return nil
}

func base() Entradas {
	return Entradas{
		Intencao: "equilibrado",
		Regras: []engine.RuleView{
			regra("v1", "verde", "alto"),
			regra("v2", "verde", "baixo"),
			regra("a1", "amarelo", "medio"),
			regra("r1", "vermelho", "alto"),
			regra("ja", "verde", "alto", jaAplicada),
			regra("fora", "verde", "alto", foraDoHardware),
		},
		Preset: []string{"v1", "a1", "r1"}, // preset "pedindo" até uma vermelha, de propósito
	}
}

func TestPresetSemeiaASelecao(t *testing.T) {
	p := GerarPlano(base())
	if it := item(p, "regra:v1"); it == nil || !it.Selecionado {
		t.Error("regra verde do preset devia vir marcada")
	}
	if it := item(p, "regra:a1"); it == nil || !it.Selecionado {
		t.Error("amarela do preset devia vir marcada (o perfil assumiu esse risco)")
	}
	if it := item(p, "regra:v2"); it == nil || it.Selecionado {
		t.Error("verde fora do preset devia aparecer, mas desmarcada")
	}
	if it := item(p, "regra:v2"); it != nil && it.OrigemTipo != "extra" {
		t.Errorf("origem de v2 = %q, queria \"extra\"", it.OrigemTipo)
	}
}

func TestRiscoAltoNuncaVemMarcado(t *testing.T) {
	for _, intencao := range []string{"equilibrado", "competitivo", "streaming", "livre"} {
		e := base()
		e.Intencao = intencao
		p := GerarPlano(e)
		it := item(p, "regra:r1")
		if it == nil {
			t.Fatalf("[%s] regra vermelha sumiu do plano — ela deve aparecer, só não marcada", intencao)
		}
		if it.Selecionado {
			t.Errorf("[%s] regra de risco alto veio marcada sozinha", intencao)
		}
	}
}

func TestConsentimentoNaoVemMarcado(t *testing.T) {
	e := base()
	e.Regras = append(e.Regras, regra("c1", "verde", "alto", comConsent))
	e.Preset = append(e.Preset, "c1")
	p := GerarPlano(e)
	it := item(p, "regra:c1")
	if it == nil {
		t.Fatal("regra que pede consentimento sumiu do plano")
	}
	if it.Selecionado {
		t.Error("consentimento marcado por padrão não é consentimento")
	}
}

func TestLivreNaoMarcaNada(t *testing.T) {
	e := base()
	e.Intencao = "livre"
	p := GerarPlano(e)
	if p.Resumo.Selecionados != 0 {
		t.Errorf("intenção livre marcou %d itens; devia marcar 0", p.Resumo.Selecionados)
	}
	if p.Resumo.Total == 0 {
		t.Error("intenção livre devia mostrar o cardápio completo mesmo sem marcar")
	}
}

func TestJaAplicadaEForaDoHardwareNaoEntram(t *testing.T) {
	p := GerarPlano(base())
	if item(p, "regra:ja") != nil {
		t.Error("regra já aplicada não é trabalho — não devia entrar no plano")
	}
	if item(p, "regra:fora") != nil {
		t.Error("regra fora do hardware não devia entrar no plano")
	}
}

func TestGargaloVirouMotivoEPuxouParaCima(t *testing.T) {
	e := base()
	e.Gargalos = []winutil.Gargalo{
		{ID: "gpu.hags", Titulo: "HAGS desligado", Severidade: "critico",
			Detectado: "HAGS desativado", Acao: "regra:v2"},
	}
	p := GerarPlano(e)
	it := item(p, "regra:v2")
	if it == nil {
		t.Fatal("regra apontada pelo gargalo sumiu")
	}
	if it.OrigemTipo != "gargalo" {
		t.Errorf("origem = %q, queria \"gargalo\"", it.OrigemTipo)
	}
	if !it.Selecionado {
		t.Error("verde apontada por gargalo crítico devia vir marcada, mesmo fora do preset")
	}
	if it.Impacto != "alto" {
		t.Errorf("impacto = %q — gargalo crítico manda no impacto", it.Impacto)
	}
	if p.Itens[0].ID != "regra:v2" && p.Itens[0].ID != "regra:v1" {
		t.Errorf("primeiro item do plano = %q; o que tem motivo real vem primeiro", p.Itens[0].ID)
	}
}

func TestGargaloSemRegraViraItemDeMao(t *testing.T) {
	e := base()
	e.Gargalos = []winutil.Gargalo{
		{ID: "ram.xmp", Titulo: "XMP/EXPO desligado", Severidade: "critico",
			Detectado: "RAM a 2133 MHz, módulo suporta 3600", Correcao: "Ative o XMP na BIOS", Acao: "manual"},
		{ID: "disco.espaco", Titulo: "Disco cheio", Severidade: "atencao", Acao: "pagina:limpeza"},
		{ID: "ok.qualquer", Titulo: "Tudo certo", Severidade: "bom", Acao: ""},
	}
	p := GerarPlano(e)
	xmp := item(p, "manual:ram.xmp")
	if xmp == nil {
		t.Fatal("gargalo sem regra devia virar item de mão")
	}
	if xmp.Fase != FaseManual {
		t.Errorf("fase do XMP = %d, queria %d (o app não entra na BIOS por você)", xmp.Fase, FaseManual)
	}
	if xmp.Selecionavel || xmp.Selecionado {
		t.Error("item de mão não é executável pelo app — não pode ser selecionável")
	}
	if xmp.Orientacao == "" {
		t.Error("item de mão sem orientação é só um alarme")
	}
	if pg := item(p, "pagina:disco.espaco"); pg == nil || pg.Alvo != "limpeza" {
		t.Error("gargalo com ação de página devia virar item apontando para a página")
	}
	if item(p, "manual:ok.qualquer") != nil {
		t.Error("achado \"bom\" não gera trabalho — não devia entrar no plano")
	}
}

func TestGargaloNaoRepeteAConsultivaEquivalente(t *testing.T) {
	e := base()
	e.Regras = append(e.Regras,
		regra("display.refresh-rate-max", "verde", "alto", consultiva),
		regra("net.fast-dns", "verde", "baixo", consultiva),
	)
	e.Gargalos = []winutil.Gargalo{
		{ID: "display.refresh", Titulo: "Tela abaixo da taxa máxima", Severidade: "critico",
			Detectado: "Monitor a 120 Hz, mas suporta 165 Hz", Correcao: "Suba para 165 Hz", Acao: "manual"},
	}
	p := GerarPlano(e)
	if item(p, "manual:display.refresh-rate-max") != nil {
		t.Error("a regra consultiva e o gargalo dizem a mesma coisa — o plano não pode listar as duas")
	}
	if item(p, "manual:display.refresh") == nil {
		t.Error("quem fica é o gargalo, que carrega o valor medido")
	}
	if item(p, "manual:net.fast-dns") == nil {
		t.Error("consultiva sem gargalo equivalente continua no plano")
	}
}

func TestFasesSeparamRebootEMao(t *testing.T) {
	e := base()
	e.Regras = append(e.Regras,
		regra("rb", "verde", "alto", comReboot),
		regra("cons", "verde", "medio", consultiva),
		regra("limp", "verde", "medio", limpeza),
	)
	e.Preset = append(e.Preset, "rb")
	p := GerarPlano(e)
	if it := item(p, "regra:rb"); it == nil || it.Fase != FaseReboot {
		t.Error("regra que pede reinício devia cair na fase 2")
	}
	if it := item(p, "manual:cons"); it == nil || it.Fase != FaseManual || it.Selecionavel {
		t.Error("regra consultiva vira item de mão na fase 3, não selecionável")
	}
	lp := item(p, "limpeza:limp")
	if lp == nil || lp.Fase != FaseDireta {
		t.Fatal("limpeza devia entrar na fase 1")
	}
	if lp.Selecionado {
		t.Error("limpeza não tem undo: nunca vem marcada sozinha")
	}
	if !lp.Selecionavel {
		t.Error("limpeza é executável pelo app — precisa ser selecionável (só não marcada)")
	}
	// as fases têm que sair em ordem no plano
	ultima := 0
	for _, it := range p.Itens {
		if it.Fase < ultima {
			t.Fatalf("plano fora de ordem: fase %d depois de %d", it.Fase, ultima)
		}
		ultima = it.Fase
	}
}

func TestScoreProjetadoUsaAFormulaDoBoostScore(t *testing.T) {
	e := base() // 4 pendentes (v1,v2,a1,r1) + 1 aplicada + 1 fora
	p := GerarPlano(e)
	if p.BaseAplicados != 1 || p.BasePendentes != 4 {
		t.Fatalf("base do score = %d aplicados / %d pendentes", p.BaseAplicados, p.BasePendentes)
	}
	if p.Resumo.ScoreAtual != 20 { // 1/5
		t.Errorf("score atual = %d, queria 20", p.Resumo.ScoreAtual)
	}
	if p.Resumo.ScoreProjetado != 60 { // 1 + v1 + a1 marcadas = 3/5
		t.Errorf("score projetado = %d, queria 60", p.Resumo.ScoreProjetado)
	}
	// marcar tudo que dá não pode passar de 100
	marcas := map[string]bool{}
	for _, it := range p.Itens {
		marcas[it.ID] = true
	}
	p.Selecionar(marcas)
	if p.Resumo.ScoreProjetado != 100 {
		t.Errorf("com tudo marcado o projetado = %d, queria 100", p.Resumo.ScoreProjetado)
	}
}

func TestSelecionarIgnoraItemDeMaoERecalcula(t *testing.T) {
	e := base()
	e.Gargalos = []winutil.Gargalo{{ID: "ram.xmp", Titulo: "XMP", Severidade: "critico", Acao: "manual"}}
	p := GerarPlano(e)
	antes := p.Resumo.Selecionados

	if n := p.Selecionar(map[string]bool{"manual:ram.xmp": true}); n != 0 {
		t.Errorf("selecionar item de mão mudou %d itens; devia ignorar", n)
	}
	if p.Resumo.Selecionados != antes {
		t.Error("resumo mudou depois de uma seleção ignorada")
	}
	if n := p.Selecionar(map[string]bool{"regra:v1": false}); n != 1 {
		t.Errorf("desmarcar item marcado mudou %d itens, queria 1", n)
	}
	if p.Resumo.Selecionados != antes-1 {
		t.Errorf("selecionados = %d, queria %d", p.Resumo.Selecionados, antes-1)
	}
	if n := p.Selecionar(map[string]bool{"regra:v1": false}); n != 0 {
		t.Error("desmarcar o que já estava desmarcado não é mudança")
	}
}

func TestResumoContaRebootEConsentimento(t *testing.T) {
	e := base()
	e.Regras = append(e.Regras, regra("rb", "verde", "alto", comReboot))
	e.Preset = append(e.Preset, "rb")
	p := GerarPlano(e)
	if p.Resumo.PedemReboot != 1 {
		t.Errorf("pedem reboot = %d, queria 1", p.Resumo.PedemReboot)
	}
	if p.Resumo.PorFase["2"] != 1 {
		t.Errorf("selecionados na fase 2 = %d, queria 1", p.Resumo.PorFase["2"])
	}
	if p.Resumo.TotalPorFase["1"] == 0 {
		t.Error("total por fase não foi contado")
	}
	// marcar uma que pede consentimento tem que aparecer no resumo
	p.Selecionar(map[string]bool{"regra:r1": true})
	if p.Resumo.Selecionados == 0 {
		t.Fatal("seleção manual da vermelha não pegou — ela é selecionável, só não vem marcada")
	}
}
