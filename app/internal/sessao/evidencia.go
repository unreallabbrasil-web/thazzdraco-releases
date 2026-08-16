package sessao

import (
	"fmt"
	"math"
	"strings"

	"thazzdraco/internal/fps"
	"thazzdraco/internal/winutil"
)

// Evidencia: a medicao de antes e a de depois (docs/SESSAO.md §4.2).
//
// O produto inteiro se apoia numa promessa — "o ganho e medido, nao prometido".
// Quem sustenta essa promessa e este arquivo, e ele so faz isso se for
// implacavel numa coisa: **so compara cenario identico**. FPS medido em outro
// jogo, ou na mesma partida com outra resolucao, nao e ganho; e ruido com cara
// de resultado.

// Cenario e a identidade de uma medicao. Sem ele, dois numeros lado a lado
// mentem por omissao.
type Cenario struct {
	Jogo      string `json:"jogo,omitempty"`
	Exe       string `json:"exe,omitempty"`
	Resolucao string `json:"resolucao,omitempty"`
	RefreshHz int    `json:"refresh_hz,omitempty"`
	DuracaoS  int    `json:"duracao_s,omitempty"`
}

// Chave identifica o cenario para efeito de COMPARACAO.
//
// Entram o jogo e a resolucao: sao eles que definem o trabalho que a maquina
// esta fazendo. NAO entra a taxa de atualizacao — se ela mudou de 120 para 165
// porque a sessao corrigiu isso, essa mudanca e justamente o ganho a mostrar,
// nao um motivo para recusar a comparacao. Tambem nao entra a duracao: ela
// afeta a confianca da medida, e vira observacao.
func (c Cenario) Chave() string {
	return strings.ToLower(strings.TrimSpace(c.Exe)) + "|" + strings.TrimSpace(c.Resolucao)
}

func (c Cenario) Texto() string {
	p := []string{}
	if c.Jogo != "" {
		p = append(p, c.Jogo)
	} else if c.Exe != "" {
		p = append(p, c.Exe)
	}
	if c.Resolucao != "" {
		p = append(p, c.Resolucao)
	}
	if c.RefreshHz > 0 {
		p = append(p, fmt.Sprintf("%d Hz", c.RefreshHz))
	}
	if c.DuracaoS > 0 {
		p = append(p, fmt.Sprintf("%ds", c.DuracaoS))
	}
	return strings.Join(p, " · ")
}

type MedidaBench struct {
	Quando string              `json:"quando"`
	Res    winutil.BenchResult `json:"res"`
}

type MedidaFPS struct {
	Quando  string         `json:"quando"`
	Cenario Cenario        `json:"cenario"`
	Res     fps.FrameStats `json:"res"`
}

// Evidencia guarda as medidas de um lado (antes ou depois). As duas sao
// independentes: da para ter bench sem FPS (PC sem jogo aberto) e vice-versa.
type Evidencia struct {
	Bench *MedidaBench `json:"bench,omitempty"`
	FPS   *MedidaFPS   `json:"fps,omitempty"`
}

func (e *Evidencia) Vazia() bool { return e == nil || (e.Bench == nil && e.FPS == nil) }

// Delta e uma metrica comparada.
type Delta struct {
	Rotulo      string  `json:"rotulo"`
	Unidade     string  `json:"unidade"`
	Antes       float64 `json:"antes"`
	Depois      float64 `json:"depois"`
	Dif         float64 `json:"dif"`
	Pct         float64 `json:"pct"`
	MaiorMelhor bool    `json:"maior_melhor"`
	Melhorou    bool    `json:"melhorou"`
	Igual       bool    `json:"igual"`
}

type BlocoComparacao struct {
	Comparavel bool    `json:"comparavel"`
	Motivo     string  `json:"motivo,omitempty"`     // por que NAO da para comparar
	Observacao string  `json:"observacao,omitempty"` // ressalva que nao invalida
	Cenario    string  `json:"cenario,omitempty"`
	Metricas   []Delta `json:"metricas,omitempty"`
}

type Comparacao struct {
	TemBaseline bool             `json:"tem_baseline"`
	TemDepois   bool             `json:"tem_depois"`
	Bench       *BlocoComparacao `json:"bench,omitempty"`
	FPS         *BlocoComparacao `json:"fps,omitempty"`
}

// margemIgual e a faixa em que uma diferenca nao e ganho nem perda: e ruido de
// medicao. Anunciar +0,4% como melhoria seria o mesmo "snake oil" que o produto
// se recusa a fazer.
const margemIgual = 1.0 // %

func delta(rotulo, unidade string, antes, depois float64, maiorMelhor bool) Delta {
	d := Delta{Rotulo: rotulo, Unidade: unidade, Antes: antes, Depois: depois,
		Dif: round2(depois - antes), MaiorMelhor: maiorMelhor}
	if antes != 0 {
		d.Pct = round1((depois - antes) / math.Abs(antes) * 100)
	}
	d.Igual = math.Abs(d.Pct) < margemIgual
	if !d.Igual {
		d.Melhorou = (d.Dif > 0) == maiorMelhor
	}
	return d
}

// Comparar monta o antes × depois. Tudo que nao da para comparar vira motivo
// escrito, nunca um espaco em branco que o cliente interpreta como quiser.
func Comparar(base, dep *Evidencia) Comparacao {
	c := Comparacao{TemBaseline: !base.Vazia(), TemDepois: !dep.Vazia()}
	if base.Vazia() && dep.Vazia() {
		return c
	}
	c.Bench = compararBench(base, dep)
	c.FPS = compararFPS(base, dep)
	return c
}

func compararBench(base, dep *Evidencia) *BlocoComparacao {
	temA := base != nil && base.Bench != nil
	temB := dep != nil && dep.Bench != nil
	if !temA && !temB {
		return nil
	}
	if !temA || !temB {
		return &BlocoComparacao{Motivo: faltaLado(temA, temB, "benchmark")}
	}
	a, b := base.Bench.Res, dep.Bench.Res
	bl := &BlocoComparacao{Comparavel: true, Metricas: []Delta{
		delta("Índice", "", float64(a.Indice), float64(b.Indice), true),
		delta("CPU 1 núcleo", "Mops/s", a.CPUSingle, b.CPUSingle, true),
		delta("CPU todos", "Mops/s", a.CPUMulti, b.CPUMulti, true),
		delta("Memória", "GB/s", a.MemBW, b.MemBW, true),
		delta("Disco (escrita)", "MB/s", a.DiskWrite, b.DiskWrite, true),
	}}
	// Ressalva fixa, e nao decorativa: rodando o benchmark duas vezes seguidas,
	// SEM mudar nada, o indice varia (o disco chega a dobrar por causa do cache do
	// Windows). Sem este aviso, o proprio produto entregaria "ganho" de ruido —
	// que e o que ele promete nao fazer. A prova de verdade e o FPS no jogo.
	bl.Observacao = "medição de bancada varia entre execuções (o disco é o mais sensível, por causa do cache) — " +
		"use como indicador, não como prova; a prova é o FPS no jogo"
	if av := primeiroNaoVazio(b.Aviso, a.Aviso); av != "" {
		bl.Observacao = av + " · " + bl.Observacao
	}
	return bl
}

func compararFPS(base, dep *Evidencia) *BlocoComparacao {
	temA := base != nil && base.FPS != nil
	temB := dep != nil && dep.FPS != nil
	if !temA && !temB {
		return nil
	}
	if !temA || !temB {
		return &BlocoComparacao{Motivo: faltaLado(temA, temB, "medição de FPS")}
	}
	ca, cb := base.FPS.Cenario, dep.FPS.Cenario
	if ca.Chave() != cb.Chave() {
		return &BlocoComparacao{Motivo: motivoCenario(ca, cb),
			Cenario: ca.Texto() + "  ×  " + cb.Texto()}
	}

	a, b := base.FPS.Res, dep.FPS.Res
	bl := &BlocoComparacao{Comparavel: true, Cenario: cb.Texto(), Metricas: []Delta{
		delta("FPS médio", "fps", a.FPSAvg, b.FPSAvg, true),
		delta("1% low", "fps", a.Low1, b.Low1, true),
		delta("0.1% low", "fps", a.Low01, b.Low01, true),
		delta("Engasgo", "%", a.StutterPct, b.StutterPct, false),
	}}

	var obs []string
	if ca.RefreshHz != cb.RefreshHz && ca.RefreshHz > 0 && cb.RefreshHz > 0 {
		obs = append(obs, fmt.Sprintf("a tela mudou de %d Hz para %d Hz entre as medições", ca.RefreshHz, cb.RefreshHz))
	}
	if durDiferente(ca.DuracaoS, cb.DuracaoS) {
		obs = append(obs, fmt.Sprintf("durações diferentes (%ds × %ds) — a captura mais curta tem menos confiança",
			ca.DuracaoS, cb.DuracaoS))
	}
	if a.Congelamentos > 0 || b.Congelamentos > 0 {
		obs = append(obs, fmt.Sprintf("congelamentos (>1s): %d antes, %d depois", a.Congelamentos, b.Congelamentos))
	}
	bl.Observacao = strings.Join(obs, " · ")
	return bl
}

// motivoCenario diz EXATAMENTE o que mudou. "Não comparável" sem motivo e so
// uma recusa; com motivo, e uma instrucao do que refazer.
func motivoCenario(a, b Cenario) string {
	an, bn := nomeDoJogo(a), nomeDoJogo(b)
	if !strings.EqualFold(strings.TrimSpace(a.Exe), strings.TrimSpace(b.Exe)) {
		return fmt.Sprintf("não dá para comparar: o antes foi medido em %s e o depois em %s. "+
			"Meça o mesmo jogo nos dois lados.", an, bn)
	}
	return fmt.Sprintf("não dá para comparar: a resolução mudou (%s → %s). "+
		"Resolução diferente é outro trabalho para a placa de vídeo.",
		naoVazio(a.Resolucao, "?"), naoVazio(b.Resolucao, "?"))
}

func nomeDoJogo(c Cenario) string {
	if c.Jogo != "" {
		return c.Jogo
	}
	return naoVazio(c.Exe, "?")
}

func faltaLado(temA, temB bool, oQue string) string {
	if temA && !temB {
		return "ainda não há " + oQue + " do depois — meça de novo para provar o ganho"
	}
	return "não houve " + oQue + " no começo, então não há com o que comparar"
}

func durDiferente(a, b int) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	maior, menor := a, b
	if menor > maior {
		maior, menor = menor, maior
	}
	return float64(maior-menor)/float64(maior) > 0.2
}

// ---- Gravacao ---------------------------------------------------------------

// LadoValido diz se o rotulo de fase da medicao e conhecido.
func LadoValido(fase string) bool { return fase == "baseline" || fase == "depois" }

// Lado devolve (e cria, se preciso) a evidencia do lado pedido.
func (s *Sessao) Lado(fase string) *Evidencia {
	if fase == "baseline" {
		if s.Baseline == nil {
			s.Baseline = &Evidencia{}
		}
		return s.Baseline
	}
	if s.Depois == nil {
		s.Depois = &Evidencia{}
	}
	return s.Depois
}

// GuardarBench grava o resultado de um benchmark no lado pedido.
func (s *Sessao) GuardarBench(fase string, res winutil.BenchResult) {
	s.Lado(fase).Bench = &MedidaBench{Quando: agora(), Res: res}
	s.Atualizada = agora()
}

// GuardarFPS grava uma captura de FPS com o cenario em que ela foi feita.
func (s *Sessao) GuardarFPS(fase string, res fps.FrameStats, cen Cenario) {
	if cen.DuracaoS == 0 && res.DurationS > 0 {
		cen.DuracaoS = int(math.Round(res.DurationS))
	}
	if cen.Exe == "" {
		cen.Exe = res.Processo
	}
	s.Lado(fase).FPS = &MedidaFPS{Quando: agora(), Cenario: cen, Res: res}
	s.Atualizada = agora()
}

// Comparacao monta o antes × depois desta sessao.
func (s *Sessao) Comparacao() Comparacao { return Comparar(s.Baseline, s.Depois) }

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }

func naoVazio(v, alt string) string {
	if strings.TrimSpace(v) == "" {
		return alt
	}
	return v
}

func primeiroNaoVazio(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
