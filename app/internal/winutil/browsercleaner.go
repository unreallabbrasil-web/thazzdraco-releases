//go:build windows

package winutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BrowserInfo descreve um browser e seus dados mensuráveis.
type BrowserInfo struct {
	ID        string           `json:"id"`
	Nome      string           `json:"nome"`
	Detectado bool             `json:"detectado"`
	Cats      []BrowserCatInfo `json:"cats"`
	TotalMB   int              `json:"total_mb"`
}

// BrowserCatInfo descreve uma categoria de dados de um browser.
type BrowserCatInfo struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`
	MB   int    `json:"mb"`
	Nota string `json:"nota,omitempty"`
}

// BrowserCleanReq é o corpo de uma requisição de limpeza.
type BrowserCleanReq struct {
	Browser string   `json:"browser"`
	Cats    []string `json:"cats"`
}

type browserCatDef struct {
	id    string
	nome  string
	nota  string
	paths []string // relativos ao dir de perfil
}

type browserDef struct {
	id        string
	nome      string
	localPath []string // relativo ao LocalAppData (Chromium)
	isFirefox bool
	cats      []browserCatDef
}

var chromiumCats = []browserCatDef{
	{id: "cache", nome: "Cache", paths: []string{"Cache", "Code Cache", "GPUCache", "ShaderCache"}},
	{id: "cookies", nome: "Cookies", nota: "Remove sessões/login de sites", paths: []string{"Cookies", "Cookies-journal"}},
	{id: "history", nome: "Histórico", nota: "Histórico de navegação", paths: []string{"History", "History-journal", "Visited Links"}},
	{id: "sessions", nome: "Sessões", nota: "Abas salvas pelo browser", paths: []string{"Sessions", "Current Session", "Current Tabs", "Last Session", "Last Tabs"}},
}

var browserDefs = []browserDef{
	{
		id: "chrome", nome: "Google Chrome",
		localPath: []string{"Google", "Chrome", "User Data", "Default"},
		cats:      chromiumCats,
	},
	{
		id: "edge", nome: "Microsoft Edge",
		localPath: []string{"Microsoft", "Edge", "User Data", "Default"},
		cats:      chromiumCats,
	},
	{
		id: "brave", nome: "Brave",
		localPath: []string{"BraveSoftware", "Brave-Browser", "User Data", "Default"},
		cats:      chromiumCats,
	},
	{
		id: "firefox", nome: "Firefox",
		isFirefox: true,
		cats: []browserCatDef{
			{id: "cache", nome: "Cache", paths: []string{"cache2"}},
			{id: "cookies", nome: "Cookies", nota: "Remove sessões/login de sites", paths: []string{"cookies.sqlite", "cookies.sqlite-journal", "cookies.sqlite-wal"}},
		},
	},
}

func browserProfileDir(b browserDef, la, roaming string) string {
	if b.isFirefox {
		base := filepath.Join(roaming, "Mozilla", "Firefox", "Profiles")
		entries, err := os.ReadDir(base)
		if err != nil {
			return ""
		}
		// preferência: *.default-release, depois *.default
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), ".default-release") {
				return filepath.Join(base, e.Name())
			}
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), ".default") {
				return filepath.Join(base, e.Name())
			}
		}
		return ""
	}
	if la == "" || len(b.localPath) == 0 {
		return ""
	}
	return filepath.Join(append([]string{la}, b.localPath...)...)
}

// BrowserScan detecta browsers instalados e mede o tamanho de cada categoria de dados.
func BrowserScan(sid string) []BrowserInfo {
	prof := RealUserProfileDir(sid)
	la, roaming := "", ""
	if len(prof) >= 3 && prof[1] == ':' {
		la = filepath.Join(prof, "AppData", "Local")
		roaming = filepath.Join(prof, "AppData", "Roaming")
	}

	var result []BrowserInfo
	for _, b := range browserDefs {
		profDir := browserProfileDir(b, la, roaming)
		detected := profDir != "" && isDirExist(profDir)
		info := BrowserInfo{ID: b.id, Nome: b.nome, Detectado: detected}
		if detected {
			for _, c := range b.cats {
				mb := 0
				for _, rel := range c.paths {
					mb += pathSizeMB(filepath.Join(profDir, rel), nil)
				}
				info.Cats = append(info.Cats, BrowserCatInfo{ID: c.id, Nome: c.nome, MB: mb, Nota: c.nota})
				info.TotalMB += mb
			}
		}
		result = append(result, info)
	}
	return result
}

// BrowserClean limpa as categorias indicadas de cada browser solicitado.
// Retorna MB liberados e detalhes por categoria.
func BrowserClean(sid string, reqs []BrowserCleanReq) map[string]any {
	prof := RealUserProfileDir(sid)
	la, roaming := "", ""
	if len(prof) >= 3 && prof[1] == ':' {
		la = filepath.Join(prof, "AppData", "Local")
		roaming = filepath.Join(prof, "AppData", "Roaming")
	}

	totalFreed := 0
	var details []string

	for _, req := range reqs {
		var bdef *browserDef
		for i := range browserDefs {
			if browserDefs[i].id == req.Browser {
				bdef = &browserDefs[i]
				break
			}
		}
		if bdef == nil {
			continue
		}
		profDir := browserProfileDir(*bdef, la, roaming)
		if profDir == "" || !isDirExist(profDir) {
			continue
		}
		wantCats := map[string]bool{}
		for _, c := range req.Cats {
			wantCats[c] = true
		}
		for _, c := range bdef.cats {
			if !wantCats[c.id] {
				continue
			}
			freed := 0
			for _, rel := range c.paths {
				full := filepath.Join(profDir, rel)
				before := pathSizeMB(full, nil)
				deletePath(full, nil)
				after := pathSizeMB(full, nil)
				if d := before - after; d > 0 {
					freed += d
				}
			}
			totalFreed += freed
			details = append(details, fmt.Sprintf("%s · %s: %s liberados", bdef.nome, c.nome, fmtMBStr(freed)))
		}
	}

	return map[string]any{"ok": true, "liberado_mb": totalFreed, "detalhes": details}
}

func isDirExist(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func fmtMBStr(mb int) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1f GB", float64(mb)/1024)
	}
	return fmt.Sprintf("%d MB", mb)
}
