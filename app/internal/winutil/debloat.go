//go:build windows

package winutil

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

//go:embed debloat_catalog.json
var debloatRaw []byte

// BloatApp é um app UWP removível com segurança (casado no catálogo curado).
type BloatApp struct {
	Pacote      string `json:"pacote"`      // PackageFullName (usado para remover)
	Name        string `json:"name"`        // nome de família (Microsoft.BingNews)
	Nome        string `json:"nome"`        // rótulo amigável PT
	Categoria   string `json:"categoria"`
	Recomendado bool   `json:"recomendado"` // pré-marcado na UI
	Nota        string `json:"nota,omitempty"`
}

type bloatCatalogEntry struct {
	Name        string `json:"name"`
	Nome        string `json:"nome"`
	Categoria   string `json:"categoria"`
	Recomendado bool   `json:"recomendado"`
	Nota        string `json:"nota"`
}

var bloatCatalog []bloatCatalogEntry
var validPkg = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func loadBloatCatalog() {
	if bloatCatalog != nil {
		return
	}
	var doc struct {
		Apps []bloatCatalogEntry `json:"apps"`
	}
	if json.Unmarshal(debloatRaw, &doc) == nil {
		bloatCatalog = doc.Apps
	}
	if bloatCatalog == nil {
		bloatCatalog = []bloatCatalogEntry{}
	}
}

// installedAppx devolve um mapa nome-de-família -> PackageFullName do usuário.
func installedAppx() map[string]string {
	m := map[string]string{}
	out, err := psHidden(`Get-AppxPackage | ForEach-Object { "$($_.Name)|$($_.PackageFullName)" }`).Output()
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		p := strings.SplitN(line, "|", 2)
		if len(p) == 2 && p[0] != "" && p[1] != "" {
			m[strings.ToLower(p[0])] = p[1]
		}
	}
	return m
}

// ListBloat devolve os apps de bloatware instalados (só os do catálogo seguro),
// já com o PackageFullName real, ordenados por categoria. NUNCA inclui apps de
// sistema — o casamento é por nome EXATO de família.
func ListBloat() map[string]any {
	loadBloatCatalog()
	inst := installedAppx()
	var apps []BloatApp
	for _, c := range bloatCatalog {
		if full, ok := inst[strings.ToLower(c.Name)]; ok {
			apps = append(apps, BloatApp{
				Pacote: full, Name: c.Name, Nome: c.Nome, Categoria: c.Categoria,
				Recomendado: c.Recomendado, Nota: c.Nota,
			})
		}
	}
	sort.SliceStable(apps, func(i, j int) bool {
		if apps[i].Categoria != apps[j].Categoria {
			return apps[i].Categoria < apps[j].Categoria
		}
		return apps[i].Nome < apps[j].Nome
	})
	if apps == nil {
		apps = []BloatApp{}
	}
	return map[string]any{"apps": apps, "total_instalados": len(inst)}
}

// RemoveBloat remove (por usuário) os pacotes informados. Reversível: o cliente
// pode reinstalar pela Microsoft Store. Devolve removidos e falhas.
func RemoveBloat(pacotes []string) map[string]any {
	loadBloatCatalog()
	allow := map[string]bool{}
	for _, c := range bloatCatalog {
		allow[strings.ToLower(c.Name)] = true
	}
	var quoted []string
	for _, p := range pacotes {
		// validacao 1: caracteres seguros (anti-injecao no PowerShell)
		if !validPkg.MatchString(p) {
			continue
		}
		// validacao 2: a familia (antes do 1o "_") tem que estar no catalogo —
		// impede remover qualquer pacote arbitrario via API, so os curados.
		fam := strings.ToLower(p)
		if i := strings.IndexByte(fam, '_'); i > 0 {
			fam = fam[:i]
		}
		if !allow[fam] {
			continue
		}
		quoted = append(quoted, "'"+p+"'")
	}
	removidos := []string{}
	falhas := map[string]string{}
	if len(quoted) == 0 {
		return map[string]any{"removidos": removidos, "falhas": falhas}
	}
	script := "$ErrorActionPreference='Stop'; foreach($p in @(" + strings.Join(quoted, ",") +
		")){ try{ Remove-AppxPackage -Package $p; Write-Output \"OK|$p\" }catch{ Write-Output \"ERR|$p\" } }"
	out, _ := psHidden(script).Output()
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "OK|") {
			removidos = append(removidos, strings.TrimPrefix(line, "OK|"))
		} else if strings.HasPrefix(line, "ERR|") {
			falhas[strings.TrimPrefix(line, "ERR|")] = "falha ao remover"
		}
	}
	return map[string]any{"removidos": removidos, "falhas": falhas}
}
