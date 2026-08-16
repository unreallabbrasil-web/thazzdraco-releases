package sessao

import (
	"strings"
	"testing"

	"thazzdraco/internal/engine"
)

// Relacoes entre regras (depende_de / conflita_com). O risco aqui e nos dois
// sentidos: bloquear o que era legitimo (o tecnico perde um ganho e nao entende
// por que) e NAO bloquear o que devia (o item aplica fora de ordem e nao pega).

func comDep(deps ...string) func(*engine.RuleView) {
	return func(rv *engine.RuleView) { rv.DependeDe = deps }
}
func comConflito(ids ...string) func(*engine.RuleView) {
	return func(rv *engine.RuleView) { rv.ConflitaCom = ids }
}

func entradasRelacao() Entradas {
	return Entradas{
		Intencao: "equilibrado",
		Regras: []engine.RuleView{
			regra("plano", "verde", "alto"),                   // a dependência
			regra("cpu100", "verde", "alto", comDep("plano")), // depende dela
			regra("usb", "verde", "baixo", comDep("plano")),   // idem
			regra("solta", "verde", "medio"),                  // sem relação
		},
		Preset: []string{"cpu100", "usb", "solta"}, // preset pede os dependentes, NÃO a dependência
	}
}

func TestDependenciaNaoSatisfeitaBloqueiaComMotivo(t *testing.T) {
	p := GerarPlano(entradasRelacao())
	it := item(p, "regra:cpu100")
	if it == nil {
		t.Fatal("item sumiu do plano")
	}
	if it.Bloqueado == "" {
		t.Fatal("o preset pediu o dependente sem a dependência — tinha que bloquear")
	}
	if !strings.Contains(it.Bloqueado, "Regra plano") {
		t.Errorf("o motivo tem que nomear de quem depende, em português; veio %q", it.Bloqueado)
	}
	if it.Selecionado {
		t.Error("item bloqueado não pode ficar marcado — aplicaria fora de ordem")
	}
	if p.Resumo.Bloqueados != 2 {
		t.Errorf("bloqueados = %d, queria 2 (cpu100 e usb)", p.Resumo.Bloqueados)
	}
	if it2 := item(p, "regra:solta"); it2.Bloqueado != "" || !it2.Selecionado {
		t.Error("item sem relação não pode ser afetado")
	}
}

func TestMarcarADependenciaDesbloqueia(t *testing.T) {
	p := GerarPlano(entradasRelacao())
	p.Selecionar(map[string]bool{"regra:plano": true})

	it := item(p, "regra:cpu100")
	if it.Bloqueado != "" {
		t.Fatalf("com a dependência marcada o item tinha que liberar; ainda diz %q", it.Bloqueado)
	}
	// liberado ≠ marcado: quem desmarcou foi a resolução, quem remarca é o técnico
	p.Selecionar(map[string]bool{"regra:cpu100": true})
	if !item(p, "regra:cpu100").Selecionado {
		t.Error("depois de liberado, o item tem que aceitar ser marcado")
	}

	// e desmarcar a dependência bloqueia de novo — o bloqueio não é decoração
	p.Selecionar(map[string]bool{"regra:plano": false})
	if it := item(p, "regra:cpu100"); it.Bloqueado == "" || it.Selecionado {
		t.Errorf("desmarcar a dependência tinha que bloquear de volta; veio %+v", it)
	}
}

func TestDependenciaJaAplicadaNoPCNaoBloqueia(t *testing.T) {
	e := entradasRelacao()
	// a dependência já está aplicada nesta máquina → some do plano
	for i := range e.Regras {
		if e.Regras[i].ID == "plano" {
			e.Regras[i].Estado = "aplicado"
			e.Regras[i].Aplicavel = false
		}
	}
	p := GerarPlano(e)
	if item(p, "regra:plano") != nil {
		t.Fatal("regra já aplicada não entra no plano")
	}
	if len(p.DepsAplicadas) != 1 || p.DepsAplicadas[0] != "plano" {
		t.Fatalf("deps já satisfeitas = %v, queria [plano]", p.DepsAplicadas)
	}
	it := item(p, "regra:cpu100")
	if it.Bloqueado != "" {
		t.Errorf("dependência já feita no PC não pode bloquear ninguém; veio %q", it.Bloqueado)
	}
	if !it.Selecionado {
		t.Error("e o item devia vir marcado normalmente pelo preset")
	}
}

func TestDependenteVemDepoisNaFila(t *testing.T) {
	e := entradasRelacao()
	e.Preset = append(e.Preset, "plano")
	p := GerarPlano(e)
	pos := map[string]int{}
	for i, it := range p.Itens {
		pos[it.Ref] = i
	}
	if pos["plano"] > pos["cpu100"] || pos["plano"] > pos["usb"] {
		t.Errorf("a dependência tem que ser executada antes; ordem = %v", pos)
	}
	// e o dependente de impacto alto não pode ter furado a fila da dependência
	if p.Itens[pos["plano"]].Fase != p.Itens[pos["cpu100"]].Fase {
		t.Skip("fases diferentes já garantem a ordem")
	}
}

func TestConflitoMantemUmSoEExplica(t *testing.T) {
	e := Entradas{
		Intencao: "equilibrado",
		Regras: []engine.RuleView{
			regra("servico", "amarelo", "baixo"),
			regra("registro", "amarelo", "baixo", comReboot, comConflito("servico")),
		},
		Preset: []string{"servico", "registro"},
	}
	p := GerarPlano(e)
	serv, reg := item(p, "regra:servico"), item(p, "regra:registro")
	if serv.Selecionado == reg.Selecionado {
		t.Fatalf("os dois fazem o mesmo: só um pode ficar marcado (servico=%v registro=%v)",
			serv.Selecionado, reg.Selecionado)
	}
	// empate em impacto e risco: vence o que NÃO pede reinício
	if !serv.Selecionado {
		t.Error("entre equivalentes, o que não pede reinício é o que fica")
	}
	if !strings.Contains(reg.Bloqueado, "faz o mesmo que") {
		t.Errorf("o perdedor tem que explicar por que saiu; veio %q", reg.Bloqueado)
	}
	if !strings.Contains(reg.Bloqueado, serv.Titulo) {
		t.Errorf("o motivo tem que nomear quem ficou; veio %q", reg.Bloqueado)
	}

	// desmarcar o vencedor libera o perdedor — a escolha continua sendo do técnico
	p.Selecionar(map[string]bool{"regra:servico": false})
	if item(p, "regra:registro").Bloqueado != "" {
		t.Error("sem o vencedor marcado, o outro tem que liberar")
	}
}

func TestConflitoEhSimetricoMesmoDeclaradoDeUmLadoSo(t *testing.T) {
	// "servico" não declara nada; quem declara é "registro"
	e := Entradas{
		Intencao: "equilibrado",
		Regras: []engine.RuleView{
			regra("servico", "amarelo", "baixo", comConflito()),
			regra("registro", "amarelo", "alto", comConflito("servico")),
		},
		Preset: []string{"servico", "registro"},
	}
	p := GerarPlano(e)
	// aqui o registro tem impacto ALTO, então ele vence apesar de não ser o declarante
	if !item(p, "regra:registro").Selecionado {
		t.Error("maior impacto vence o conflito")
	}
	if item(p, "regra:servico").Bloqueado == "" {
		t.Error("o outro lado do conflito tem que ser bloqueado mesmo sem ter declarado a relação")
	}
}

// Ciclo é bug de catálogo, não situação de runtime (o guarda está em
// TestCatalogoSemCicloDeDependencia). Aqui só garantimos que, se um escapar, o
// plano não trava nem perde item — o atendimento continua de pé.
func TestCicloDeDependenciaNaoTravaOPlano(t *testing.T) {
	e := Entradas{
		Intencao: "equilibrado",
		Regras: []engine.RuleView{
			regra("a", "verde", "alto", comDep("b")),
			regra("b", "verde", "alto", comDep("a")),
		},
		Preset: []string{"a", "b"},
	}
	p := GerarPlano(e) // não pode entrar em recursão infinita
	if len(p.Itens) != 2 {
		t.Fatalf("plano com ciclo perdeu itens: %d", len(p.Itens))
	}
	p.Selecionar(map[string]bool{"regra:a": false}) // e continua respondendo
	if item(p, "regra:b").Bloqueado == "" {
		t.Error("com a dependência desmarcada, o outro lado tem que bloquear normalmente")
	}
}

// Um ciclo em rules.json travaria itens sem o técnico ter como resolver: para
// marcar A é preciso marcar B, e vice-versa. Não pode chegar em produção.
func TestCatalogoSemCicloDeDependencia(t *testing.T) {
	regras, err := engine.LoadRules()
	if err != nil {
		t.Fatalf("carregar rules.json: %v", err)
	}
	deps := map[string][]string{}
	for _, r := range regras {
		if len(r.DependeDe) > 0 {
			deps[r.ID] = r.DependeDe
		}
	}
	estado := map[string]int{} // 0 = novo, 1 = na pilha, 2 = fechado
	var visita func(id string, caminho []string)
	visita = func(id string, caminho []string) {
		switch estado[id] {
		case 1:
			t.Errorf("ciclo de dependência no catálogo: %s → %s", strings.Join(caminho, " → "), id)
			return
		case 2:
			return
		}
		estado[id] = 1
		for _, d := range deps[id] {
			visita(d, append(caminho, id))
		}
		estado[id] = 2
	}
	for id := range deps {
		visita(id, nil)
	}
}

// O catálogo real precisa estar íntegro: relação apontando para regra que não
// existe bloquearia o item para sempre, sem o técnico ter como resolver.
func TestRelacoesDoCatalogoApontamParaRegrasQueExistem(t *testing.T) {
	regras, err := engine.LoadRules()
	if err != nil {
		t.Fatalf("carregar rules.json: %v", err)
	}
	existe := map[string]bool{}
	for _, r := range regras {
		existe[r.ID] = true
	}
	n := 0
	for _, r := range regras {
		for _, d := range r.DependeDe {
			n++
			if !existe[d] {
				t.Errorf("%s depende de %q, que não existe no catálogo", r.ID, d)
			}
			if d == r.ID {
				t.Errorf("%s depende de si mesma", r.ID)
			}
		}
		for _, c := range r.ConflitaCom {
			n++
			if !existe[c] {
				t.Errorf("%s conflita com %q, que não existe no catálogo", r.ID, c)
			}
		}
	}
	if n == 0 {
		t.Error("nenhuma relação declarada — o catálogo tem pelo menos as de energia")
	}
}
