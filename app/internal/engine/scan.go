package engine

import (
	"fmt"
	"strings"
	"time"

	"thazzdraco/internal/winutil"
)

// RuleView e a visao de uma regra para a UI: metadados + estado detectado.
type RuleView struct {
	ID                  string `json:"id"`
	Parte               int    `json:"parte"`
	Categoria           string `json:"categoria"`
	Titulo              string `json:"titulo"`
	Descricao           string `json:"descricao"`
	Tier                string `json:"tier"`
	Modo                string `json:"modo"`
	Impacto             string `json:"impacto"`
	RequerReboot        bool   `json:"requer_reboot"`
	RequerConsentimento bool   `json:"requer_consentimento"`
	Orientacao          string `json:"orientacao,omitempty"`
	Estado              string `json:"estado"`
	Detalhe             string `json:"detalhe"`
	Aplicavel           bool   `json:"aplicavel"` // pode aplicar agora (tem action e esta pendente)
	Tecnico             string `json:"tecnico"`   // o que a regra mexe (chave/valor/servico/powercfg)
}

// tecnicoOf descreve, em texto, o que a regra altera no sistema (transparencia).
func tecnicoOf(r *Rule) string {
	a := r.Action
	if a == nil {
		return "Consultivo — não altera o sistema; só recomenda."
	}
	switch a.Tipo {
	case "registry":
		var p []string
		for _, v := range a.Valores {
			p = append(p, fmt.Sprintf("%s = %s (%s)", v.Name, asString(v.Value), v.ValueType))
		}
		return fmt.Sprintf("Registro: %s\\%s → %s", a.Hive, a.Path, strings.Join(p, " · "))
	case "registry-foreach":
		var p []string
		for _, v := range a.Valores {
			p = append(p, fmt.Sprintf("%s = %s", v.Name, asString(v.Value)))
		}
		return fmt.Sprintf("Registro (cada interface): %s\\%s\\* → %s", a.Hive, a.Base, strings.Join(p, " · "))
	case "service":
		return fmt.Sprintf("Serviços → %s: %s", a.Startup, strings.Join(a.Servicos, ", "))
	case "powercfg":
		var p []string
		for _, c := range a.Comandos {
			p = append(p, strings.Join(c, " "))
		}
		return "powercfg " + strings.Join(p, " ; ")
	case "cleanup":
		return "Apaga arquivos temporários em: " + strings.Join(a.Alvos, ", ")
	}
	return ""
}

// ScanResult e o retorno completo de uma varredura.
type ScanResult struct {
	Quando string         `json:"quando"`
	Score  int            `json:"score"`
	Totais map[string]int `json:"totais"`
	Perfil map[string]any `json:"perfil"`
	Regras []RuleView     `json:"regras"`
}

// Scan roda a deteccao em todas as regras e consolida score + totais.
func Scan(rules []Rule, ctx Ctx) ScanResult {
	res := ScanResult{
		Quando: time.Now().Format("2006-01-02 15:04:05"),
		Perfil: ctx.Profile,
		Totais: map[string]int{},
	}
	aplicados, pendentes := 0, 0
	for i := range rules {
		r := &rules[i]
		d := DetectRule(r, ctx)
		view := RuleView{
			ID: r.ID, Parte: r.Parte, Categoria: r.Categoria, Titulo: r.Titulo,
			Descricao: r.Descricao, Tier: r.Tier, Modo: r.Modo, Impacto: r.Impacto,
			RequerReboot: r.RequerReboot, RequerConsentimento: r.RequerConsentimento,
			Orientacao: r.Orientacao, Estado: d.Estado, Detalhe: d.Detalhe,
		}
		view.Aplicavel = r.Action != nil && d.Estado == "pendente"
		view.Tecnico = tecnicoOf(r)

		switch d.Estado {
		case "aplicado":
			aplicados++
			res.Totais["aplicados"]++
		case "pendente":
			pendentes++
			res.Totais["pendentes"]++
			res.Totais[r.Tier]++ // verde/amarelo/vermelho
		case "oportunidade", "recomendado":
			res.Totais["oportunidades"]++
		}
		res.Regras = append(res.Regras, view)
	}
	res.Totais["total"] = len(rules)
	if aplicados+pendentes > 0 {
		res.Score = int(float64(aplicados) / float64(aplicados+pendentes) * 100.0)
	} else {
		res.Score = 100
	}
	return res
}

// BuildCtx monta o contexto (SID real + perfil) uma vez por operacao.
func BuildCtx() Ctx {
	return Ctx{Sid: winutil.RealUserSid(), Profile: winutil.BuildProfile()}
}
