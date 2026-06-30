package engine

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var reIndex = regexp.MustCompile(`^(.*)\[(\d+)\]$`)

// asString normaliza qualquer valor para comparacao textual (como o motor PS,
// que comparava "$a" -ne "$esperado"). Numeros inteiros saem sem ".0".
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == math.Trunc(t) && !math.IsInf(t, 0) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	}
	return ""
}

// numOf tenta extrair um float de v (numero nativo ou string numerica).
func numOf(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

// ResolveField navega um caminho pontilhado no perfil (com suporte a indice
// "campo[0]"). Espelha Resolve-Campo do lib.ps1.
func ResolveField(profile map[string]any, campo string) any {
	var cur any = profile
	for _, part := range strings.Split(campo, ".") {
		idx := -1
		if m := reIndex.FindStringSubmatch(part); m != nil {
			part = m[1]
			idx, _ = strconv.Atoi(m[2])
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
		if idx >= 0 {
			switch arr := cur.(type) {
			case []map[string]any:
				if idx >= len(arr) {
					return nil
				}
				cur = arr[idx]
			case []any:
				if idx >= len(arr) {
					return nil
				}
				cur = arr[idx]
			default:
				return nil
			}
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}

// TestGate avalia o gate de hardware contra o perfil. nil => sempre satisfeito.
func TestGate(g *Gate, profile map[string]any) bool {
	if g == nil {
		return true
	}
	if len(g.Todos) > 0 {
		for i := range g.Todos {
			if !TestGate(&g.Todos[i], profile) {
				return false
			}
		}
		return true
	}
	if len(g.Qualquer) > 0 {
		for i := range g.Qualquer {
			if TestGate(&g.Qualquer[i], profile) {
				return true
			}
		}
		return false
	}
	atual := ResolveField(profile, g.Campo)
	switch g.Op {
	case "==":
		return asString(atual) == asString(g.Valor)
	case "!=":
		return asString(atual) != asString(g.Valor)
	case "<":
		a, ok1 := numOf(atual)
		b, ok2 := numOf(g.Valor)
		return ok1 && ok2 && a < b
	case ">":
		a, ok1 := numOf(atual)
		b, ok2 := numOf(g.Valor)
		return ok1 && ok2 && a > b
	case ">=":
		a, ok1 := numOf(atual)
		b, ok2 := numOf(g.Valor)
		return ok1 && ok2 && a >= b
	case "<=":
		a, ok1 := numOf(atual)
		b, ok2 := numOf(g.Valor)
		return ok1 && ok2 && a <= b
	case "contem":
		return strings.Contains(asString(atual), asString(g.Valor))
	}
	return true
}
