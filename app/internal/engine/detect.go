package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"thazzdraco/internal/winutil"
)

// Ctx carrega o que a deteccao/aplicacao precisam saber sobre a maquina.
type Ctx struct {
	Sid     string         // SID do usuario real (para HKCU quando elevado)
	Profile map[string]any // perfil de hardware/contexto
}

// DetectResult: estado de uma regra + detalhe legivel.
type DetectResult struct {
	Estado  string `json:"estado"`
	Detalhe string `json:"detalhe"`
}

var reWinEnv = regexp.MustCompile(`%([^%]+)%`)

func expandWinEnv(s string) string {
	return reWinEnv.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.Trim(m, "%")
		if v := os.Getenv(name); v != "" {
			return v
		}
		return m
	})
}

// DetectRule avalia uma regra. Espelha Detect-Rule do lib.ps1.
func DetectRule(r *Rule, ctx Ctx) DetectResult {
	if !TestGate(r.HardwareGate, ctx.Profile) {
		return DetectResult{"nao-aplicavel", "gate de hardware nao satisfeito"}
	}
	d := r.Detect
	switch d.Tipo {
	case "registry":
		return detectRegistry(d, ctx)
	case "registry-foreach":
		return detectForeach(d, ctx)
	case "powercfg":
		return detectPowercfg(d)
	case "powercfg-setting":
		return detectPowercfgSetting(d)
	case "service":
		return detectService(d)
	case "cleanup-size":
		mb := cleanupSizeMB(cleanupTargets(ctx, d.Alvos))
		return DetectResult{"oportunidade", fmt.Sprintf("%.1f MB em temporarios", mb)}
	case "profile-compara":
		return detectProfileCompara(d, ctx)
	case "dns-check":
		return detectDNS(ctx)
	}
	return DetectResult{"desconhecido", "tipo de detect nao suportado: " + d.Tipo}
}

func readRegAsString(d Detect, ctx Ctx, name string) (string, bool) {
	// Tenta inteiro primeiro (cobre DWORD/-1); depois string.
	if v, ok := winutil.ReadInteger(d.Hive, ctx.Sid, d.HkcuRealUser, d.Path, name); ok {
		return asString(v), true
	}
	if s, ok := winutil.ReadString(d.Hive, ctx.Sid, d.HkcuRealUser, d.Path, name); ok {
		return s, true
	}
	return "", false
}

func detectRegistry(d Detect, ctx Ctx) DetectResult {
	todasOK := true
	var det []string
	for _, v := range d.Valores {
		atual, ok := readRegAsString(d, ctx, v.Name)
		esp := asString(v.Esperado)
		if !ok || atual != esp {
			todasOK = false
		}
		shown := atual
		if !ok {
			shown = "ausente"
		}
		det = append(det, fmt.Sprintf("%s=%s(esp:%s)", v.Name, shown, esp))
	}
	return DetectResult{estadoIf(todasOK), strings.Join(det, "; ")}
}

func detectForeach(d Detect, ctx Ctx) DetectResult {
	subs, err := winutil.ListSubkeys(d.Hive, ctx.Sid, false, d.Base)
	if err != nil || len(subs) == 0 {
		return DetectResult{"desconhecido", "sem subchaves em " + d.Base}
	}
	tot, ok := 0, 0
	for _, s := range subs {
		tot++
		all := true
		full := d.Base + `\` + s
		for _, v := range d.Valores {
			atual, found := winutil.ReadInteger(d.Hive, ctx.Sid, false, full, v.Name)
			if !found || asString(atual) != asString(v.Esperado) {
				all = false
			}
		}
		if all {
			ok++
		}
	}
	return DetectResult{estadoIf(ok == tot && tot > 0), fmt.Sprintf("%d/%d subchaves", ok, tot)}
}

func detectPowercfg(d Detect) DetectResult {
	guid := winutil.ActiveScheme()
	name := strings.ToLower(winutil.ActiveSchemeName())
	var esperados []string
	_ = json.Unmarshal(d.Esperado, &esperados)
	for _, e := range esperados {
		// Item GUID: compara o GUID ativo. Item não-GUID (ex.: "desempenho"):
		// casa pelo NOME do plano — necessário porque a cópia do Ultimate ganha
		// um GUID aleatório, mas o nome continua "Ultimate/Desempenho Máximo".
		if isSchemeGUID(e) {
			if strings.EqualFold(e, guid) {
				return DetectResult{"aplicado", "esquema ativo: " + guid}
			}
		} else if name != "" && strings.Contains(name, strings.ToLower(e)) {
			return DetectResult{"aplicado", "plano: " + winutil.ActiveSchemeName()}
		}
	}
	return DetectResult{"pendente", "plano: " + winutil.ActiveSchemeName()}
}

func isSchemeGUID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

func detectPowercfgSetting(d Detect) DetectResult {
	ac, ok := winutil.PowercfgAcValue(d.Sub, d.Setting)
	if !ok {
		return DetectResult{"desconhecido", "nao foi possivel ler a configuracao"}
	}
	var esp int64
	_ = json.Unmarshal(d.Esperado, &esp)
	return DetectResult{estadoIf(ac == esp), fmt.Sprintf("AC=%d (esp:%d)", ac, esp)}
}

func detectService(d Detect) DetectResult {
	todasOK := true
	var det []string
	for _, n := range d.Servicos {
		st, exists := winutil.ServiceStartType(n)
		if !exists {
			det = append(det, n+"=ausente")
			continue // ausente nao impede "aplicado" (nada a desativar)
		}
		name := winutil.StartTypeName(st)
		if name == "Disabled" {
			det = append(det, n+"=disabled")
		} else {
			todasOK = false
			det = append(det, n+"="+name)
		}
	}
	return DetectResult{estadoIf(todasOK), strings.Join(det, "; ")}
}

func detectProfileCompara(d Detect, ctx Ctx) DetectResult {
	a := ResolveField(ctx.Profile, d.Atual)
	n := ResolveField(ctx.Profile, d.Nominal)
	av, ok1 := numOf(a)
	nv, ok2 := numOf(n)
	if !ok1 || !ok2 {
		return DetectResult{"desconhecido", "dados de hardware indisponiveis"}
	}
	estado := "ok"
	if av < nv {
		estado = "recomendado"
	}
	return DetectResult{estado, fmt.Sprintf("atual: %s / nominal: %s", asString(a), asString(n))}
}

func detectDNS(ctx Ctx) DetectResult {
	fast := map[string]bool{"1.1.1.1": true, "1.0.0.1": true, "8.8.8.8": true,
		"8.8.4.4": true, "9.9.9.9": true, "149.112.112.112": true}
	var dns []string
	if rede, ok := ctx.Profile["rede"].(map[string]any); ok {
		if list, ok := rede["dns"].([]string); ok {
			dns = list
		}
	}
	temFast := false
	for _, d := range dns {
		if fast[d] {
			temFast = true
		}
	}
	estado := "recomendado"
	if temFast {
		estado = "ok"
	}
	return DetectResult{estado, "DNS atual: " + strings.Join(dns, ", ")}
}

func estadoIf(applied bool) string {
	if applied {
		return "aplicado"
	}
	return "pendente"
}

// cleanupTargets resolve as pastas reais de limpeza. Quando elevado e o SID do
// usuario interativo e conhecido, usa o TEMP DELE (nao o do admin) + Windows\Temp.
// Caso contrario, expande as variaveis dos alvos da regra (processo atual).
func cleanupTargets(ctx Ctx, alvos []string) []string {
	if ctx.Sid != "" {
		if prof := winutil.RealUserProfileDir(ctx.Sid); prof != "" {
			return []string{
				filepath.Join(prof, "AppData", "Local", "Temp"),
				`C:\Windows\Temp`,
			}
		}
	}
	var out []string
	for _, a := range alvos {
		out = append(out, expandWinEnv(a))
	}
	return out
}

// cleanupSizeMB soma o tamanho dos diretorios ja resolvidos (apenas medicao).
func cleanupSizeMB(dirs []string) float64 {
	var total int64
	for _, p := range dirs {
		filepath.WalkDir(p, func(_ string, dent os.DirEntry, err error) error {
			if err != nil || dent.IsDir() {
				return nil
			}
			if info, e := dent.Info(); e == nil {
				total += info.Size()
			}
			return nil
		})
	}
	return float64(total) / (1024 * 1024)
}
