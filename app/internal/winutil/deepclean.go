//go:build windows

package winutil

import (
	"os"
	"path/filepath"
	"strings"
)

// CleanCat é uma categoria da limpeza profunda. Tudo regenerável; nada de dados
// pessoais. As pastas protegidas (Windows.old) usam método próprio (icacls+rd).
type CleanCat struct {
	ID          string `json:"id"`
	Nome        string `json:"nome"`
	Descricao   string `json:"descricao"`
	MB          int    `json:"mb"`
	Recomendado bool   `json:"recomendado"`
	Aviso       string `json:"aviso,omitempty"`
	paths       []string
	prefixos    []string // se setado, so toca arquivos com esses prefixos
	special     string   // "" | "windows-old" | "recycle"
}

// cleanCats monta as categorias com os caminhos resolvidos para o usuário real.
func cleanCats(sid string) []*CleanCat {
	// Resolve o LocalAppData do usuário real; só usa se for caminho ABSOLUTO
	// (ex.: C:\Users\fulano\...). Se o perfil não resolver, laJoin devolve ""
	// e os caminhos relativos ao usuário ficam vazios (ignorados com segurança).
	prof := RealUserProfileDir(sid)
	la := ""
	if len(prof) >= 3 && prof[1] == ':' {
		la = filepath.Join(prof, "AppData", "Local")
	}
	laJoin := func(parts ...string) string {
		if la == "" {
			return ""
		}
		return filepath.Join(append([]string{la}, parts...)...)
	}
	win := os.Getenv("SystemRoot")
	if win == "" || len(win) < 3 || win[1] != ':' {
		win = `C:\Windows`
	}
	pd := os.Getenv("ProgramData")
	if pd == "" || len(pd) < 3 || pd[1] != ':' {
		pd = `C:\ProgramData`
	}
	return []*CleanCat{
		{
			ID: "shader-cache", Nome: "Cache de shaders (GPU)", Recomendado: true,
			Descricao: "Cache de shaders compilados (NVIDIA/AMD/DirectX). Limpar libera espaço e costuma corrigir stutter de shaders — é regenerado no jogo.",
			paths: []string{
				laJoin("D3DSCache"),
				laJoin("NVIDIA", "DXCache"),
				laJoin("NVIDIA", "GLCache"),
				laJoin("AMD", "DxCache"),
				laJoin("AMD", "DxcCache"),
			},
		},
		{
			ID: "crash-dumps", Nome: "Despejos de erro e relatórios", Recomendado: true,
			Descricao: "Minidumps, MEMORY.DMP, despejos de apps travados e relatórios do Windows Error Reporting.",
			paths: []string{
				filepath.Join(win, "Minidump"),
				filepath.Join(win, "MEMORY.DMP"),
				laJoin("CrashDumps"),
				filepath.Join(pd, "Microsoft", "Windows", "WER", "ReportQueue"),
				filepath.Join(pd, "Microsoft", "Windows", "WER", "ReportArchive"),
				filepath.Join(pd, "Microsoft", "Windows", "WER", "Temp"),
			},
		},
		{
			ID: "update-cache", Nome: "Cache do Windows Update", Recomendado: true,
			Descricao: "Arquivos de atualização já baixados. O Windows rebaixa se precisar.",
			paths:     []string{filepath.Join(win, "SoftwareDistribution", "Download")},
		},
		{
			ID: "delivery-opt", Nome: "Delivery Optimization (P2P)", Recomendado: true,
			Descricao: "Cache do compartilhamento P2P de atualizações.",
			paths: []string{
				filepath.Join(win, "SoftwareDistribution", "DeliveryOptimization"),
				filepath.Join(win, "ServiceProfiles", "NetworkService", "AppData", "Local", "Microsoft", "Windows", "DeliveryOptimization"),
			},
		},
		{
			ID: "thumb-cache", Nome: "Cache de miniaturas e ícones", Recomendado: false,
			Descricao: "Miniaturas e ícones em cache (alguns podem estar em uso pelo Explorer). Regenerado automaticamente.",
			Aviso:     "Alguns arquivos podem estar em uso; o que estiver livre é limpo.",
			paths:     []string{laJoin("Microsoft", "Windows", "Explorer")},
			prefixos:  []string{"thumbcache_", "iconcache_"}, // só os DBs de cache, nada mais na pasta
		},
		{
			ID: "recycle", Nome: "Esvaziar a Lixeira", Recomendado: false, special: "recycle",
			Descricao: "Esvazia a Lixeira de todos os discos.",
			Aviso:     "Apaga de vez o que está na Lixeira — confira se não há nada que o cliente queira.",
			paths:     []string{`C:\$Recycle.Bin`},
		},
		{
			ID: "windows-old", Nome: "Instalação anterior do Windows (Windows.old)", Recomendado: false, special: "windows-old",
			Descricao: "Pasta da versão anterior do Windows após uma atualização — costuma ter dezenas de GB.",
			Aviso:     "Remove a possibilidade de VOLTAR para a versão anterior do Windows. Só limpe se a atual está estável.",
			paths:     []string{`C:\Windows.old`},
		},
	}
}

// DeepCleanScan calcula o tamanho de cada categoria (só as presentes/relevantes).
func DeepCleanScan(sid string) map[string]any {
	cats := cleanCats(sid)
	out := []*CleanCat{}
	totalMB := 0
	for _, c := range cats {
		mb := 0
		for _, p := range c.paths {
			mb += pathSizeMB(p, c.prefixos)
		}
		c.MB = mb
		// some categorias zeradas (sem nada pra limpar), exceto as especiais
		// que valem mostrar mesmo pequenas — windows-old só se existir.
		if c.special == "windows-old" && mb == 0 {
			continue
		}
		if mb == 0 && c.special == "" {
			continue
		}
		totalMB += mb
		out = append(out, c)
	}
	return map[string]any{"categorias": out, "total_mb": totalMB}
}

// DeepClean limpa as categorias selecionadas e devolve o total liberado (MB).
// As categorias com acao IRREVERSIVEL (esvaziar a Lixeira, apagar a Windows.old)
// so rodam com confirmar=true — defesa de servidor alem do aviso da UI, pra que um
// request solto/malformado jamais apague esse tipo de coisa sem confirmacao.
func DeepClean(sid string, ids []string, confirmar bool) map[string]any {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	cats := cleanCats(sid)
	freedMB := 0
	limpas := []string{}
	bloqueadas := []string{}
	for _, c := range cats {
		if !want[c.ID] {
			continue
		}
		if c.special != "" && !confirmar {
			bloqueadas = append(bloqueadas, c.ID)
			continue
		}
		antes := 0
		for _, p := range c.paths {
			antes += pathSizeMB(p, c.prefixos)
		}
		switch c.special {
		case "recycle":
			psHidden(`Clear-RecycleBin -Force -ErrorAction SilentlyContinue`).Run()
		case "windows-old":
			removeWindowsOld(c.paths[0])
		default:
			for _, p := range c.paths {
				deletePath(p, c.prefixos)
			}
		}
		depois := 0
		for _, p := range c.paths {
			depois += pathSizeMB(p, c.prefixos)
		}
		freed := antes - depois
		if freed < 0 {
			freed = 0
		}
		freedMB += freed
		limpas = append(limpas, c.ID)
	}
	return map[string]any{"liberado_mb": freedMB, "limpas": limpas, "bloqueadas": bloqueadas}
}

// ---- Helpers ----------------------------------------------------------------

// matchPrefix: se prefixos vazio, casa qualquer nome; senão exige o prefixo.
func matchPrefix(name string, prefixos []string) bool {
	if len(prefixos) == 0 {
		return true
	}
	low := strings.ToLower(name)
	for _, pre := range prefixos {
		if strings.HasPrefix(low, strings.ToLower(pre)) {
			return true
		}
	}
	return false
}

// pathSizeMB devolve o tamanho de um caminho (arquivo ou pasta) em MB. Se p for
// pasta e houver prefixos, conta só os itens de topo com esse prefixo.
func pathSizeMB(p string, prefixos []string) int {
	if p == "" {
		return 0
	}
	st, err := os.Lstat(p)
	if err != nil {
		return 0
	}
	if st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return 0 // reparse point/junction: não contar (não vamos seguir)
	}
	if !st.IsDir() {
		return int(st.Size() / (1 << 20))
	}
	if len(prefixos) == 0 {
		return dirSizeMB(p)
	}
	var total int64
	entries, _ := os.ReadDir(p)
	for _, e := range entries {
		if !matchPrefix(e.Name(), prefixos) {
			continue
		}
		if info, err := os.Lstat(filepath.Join(p, e.Name())); err == nil {
			total += info.Size()
		}
	}
	return int(total / (1 << 20))
}

// deletePath: se for arquivo, remove; se for pasta, apaga o CONTEÚDO de topo
// (mantém a pasta). NUNCA segue junctions/symlinks (apaga só o link, não o
// destino). Com prefixos, só toca arquivos com esse prefixo. Best-effort.
func deletePath(p string, prefixos []string) {
	if p == "" {
		return
	}
	st, err := os.Lstat(p)
	if err != nil {
		return
	}
	if st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		os.Remove(p) // reparse point: remove só o vínculo, sem recursar no alvo
		return
	}
	if !st.IsDir() {
		os.Remove(p)
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !matchPrefix(e.Name(), prefixos) {
			continue
		}
		child := filepath.Join(p, e.Name())
		// não seguir junction/symlink filho: remove só o vínculo.
		if info, err := os.Lstat(child); err == nil && info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			os.Remove(child)
			continue
		}
		os.RemoveAll(child)
	}
}

// removeWindowsOld toma posse e remove a pasta Windows.old (protegida pelo
// TrustedInstaller). Guarda: SÓ aceita exatamente <SystemDrive>\Windows.old —
// nunca um caminho arbitrário. Usa o SID dos Administradores (independe de idioma)
// e os.RemoveAll (sem passar pelo parser do cmd.exe).
func removeWindowsOld(path string) {
	sysDrive := os.Getenv("SystemDrive")
	if sysDrive == "" {
		sysDrive = "C:"
	}
	want := filepath.Clean(sysDrive + `\Windows.old`)
	if !strings.EqualFold(filepath.Clean(path), want) {
		return // proteção: jamais rodar em outra pasta
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return
	}
	const adminsSID = "*S-1-5-32-544"
	runQuiet("icacls", path, "/setowner", adminsSID, "/t", "/c", "/q")
	runQuiet("icacls", path, "/grant", adminsSID+":F", "/t", "/c", "/q")
	os.RemoveAll(path)
}
