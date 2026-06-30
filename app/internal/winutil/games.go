//go:build windows

package winutil

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Game: um jogo instalado + estado dos tweaks por-jogo.
type Game struct {
	Nome     string       `json:"nome"`
	Loja     string       `json:"loja"`               // Steam | Epic | Battle.net | ...
	AppID    string       `json:"app_id,omitempty"`   // Steam AppID
	CoverURL string       `json:"cover_url,omitempty"` // URL da capa portrait
	Pasta    string       `json:"pasta"`
	Exe      string       `json:"exe"`                // executavel principal (melhor palpite)
	FSO      bool         `json:"fso"`                // Fullscreen Optimizations desativado
	GPU      bool         `json:"gpu"`                // preferencia de GPU = alto desempenho
	Prio     bool         `json:"prio"`               // prioridade de CPU alta (IFEO)
	AV       bool         `json:"av"`                 // pasta excluida do antivirus (Defender)
	Perfil   *GameProfile `json:"perfil,omitempty"`   // perfil curado por titulo
}

const ifeoBase = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options\`

const (
	regLayers = `Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
	regGpuPref = `Software\Microsoft\DirectX\UserGpuPreferences`
	fsoFlag    = "DISABLEDXMAXIMIZEDWINDOWEDMODE"
	gpuHighVal = "GpuPreference=2;"
)

var (
	reVdfPath   = regexp.MustCompile(`"path"\s*"([^"]+)"`)
	reAcfName   = regexp.MustCompile(`"name"\s*"([^"]+)"`)
	reAcfDir    = regexp.MustCompile(`"installdir"\s*"([^"]+)"`)
	reNonAlnum  = regexp.MustCompile(`[^a-z0-9]+`)
	exeBlock    = []string{"unins", "redist", "vcredist", "vc_redist", "dxsetup", "crashhandler",
		"crashreport", "crashpad", "easyanticheat", "eaanticheat", "anticheat", "battleye", "epiconline",
		"_setup", "setup", "installer", "directx", "dotnet", "ndp48", "helper", "cleanup", "diag",
		"benchmark", "editor", "report", "console", "dedicated", "subprocess", "handler", "service", "uninstall"}
	// nomes que nao sao jogos para otimizar (redists, servidores, ferramentas)
	nameBlock = []string{"redistributable", "steamworks common", "proton", "linux runtime",
		"dedicated server", "sdk", "benchmark", " server", " tools", "tool kit"}
)

func isPlayable(name string) bool {
	n := strings.ToLower(name)
	for _, b := range nameBlock {
		if strings.Contains(n, b) {
			return false
		}
	}
	return true
}

func alnum(s string) string { return reNonAlnum.ReplaceAllString(strings.ToLower(s), "") }

// F1: LiveGames retorna quais jogos da lista estão atualmente rodando como processo.
func LiveGames(games []Game) []Game {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	running := map[string]bool{}
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		running[strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))] = true
	}
	var live []Game
	for _, g := range games {
		if g.Exe == "" {
			continue
		}
		if running[strings.ToLower(filepath.Base(g.Exe))] {
			live = append(live, g)
		}
	}
	return live
}

// DetectGames varre as lojas/launchers e devolve os jogos instalados com os
// tweaks. Cobre Steam, Epic, e via registro: Battle.net (COD/Blizzard), EA
// (Battlefield), Ubisoft, Rockstar, Riot, GOG e jogos avulsos por publisher.
func DetectGames(sid string) []Game {
	var raw []Game
	raw = append(raw, steamGames(sid)...)
	raw = append(raw, epicGames()...)
	raw = append(raw, launcherGames()...)
	seen := map[string]bool{}
	var games []Game
	for _, g := range raw {
		k := strings.ToLower(g.Pasta)
		if seen[k] {
			continue
		}
		seen[k] = true
		games = append(games, g)
	}
	avSet := defenderExclusions()
	for i := range games {
		games[i].FSO, games[i].GPU, games[i].Prio = gameTweakState(sid, games[i].Exe)
		games[i].AV = avSet[strings.ToLower(games[i].Pasta)]
		games[i].Perfil = MatchProfile(games[i].Nome, games[i].Exe)
	}
	sort.Slice(games, func(i, j int) bool { return strings.ToLower(games[i].Nome) < strings.ToLower(games[j].Nome) })
	// resolve capas de jogos sem cover_url via Steam Search API (com cache)
	ResolveCoverURLs(games)
	if games == nil {
		return []Game{}
	}
	return games
}

// ---- Outros launchers (via registro) ----------------------------------------

// publishers de jogos conhecidos (campo Publisher das entradas de Uninstall).
var gamePublishers = []string{"blizzard", "activision", "electronic arts", "ea ",
	"ubisoft", "rockstar games", "riot games", "bethesda", "cd projekt", "square enix",
	"bandai namco", "2k games", "capcom", "sega", "take-two", "epic games"}

// entradas que sao launchers/ferramentas/anticheat — nunca jogos.
var notGameNames = []string{"launcher", "connect", "battle.net", "ea app", "origin",
	"uplay", "redist", "directx", "vcredist", "runtime", "social club", "overwolf",
	"easyanticheat", "anticheat", "battleye", "riot client", "vanguard", "framework",
	" sdk", "service", "driver", "tool", "epic online", "dotnet"}

func lojaFromPub(pub string) string {
	p := strings.ToLower(pub)
	switch {
	case strings.Contains(p, "blizzard"), strings.Contains(p, "activision"):
		return "Battle.net"
	case strings.Contains(p, "electronic arts"), strings.HasPrefix(p, "ea"):
		return "EA"
	case strings.Contains(p, "ubisoft"):
		return "Ubisoft"
	case strings.Contains(p, "rockstar"):
		return "Rockstar"
	case strings.Contains(p, "riot"):
		return "Riot"
	}
	return "Loja"
}

// launcherGames detecta jogos fora de Steam/Epic via registro: a varredura das
// entradas de desinstalacao por publisher de jogo (ampla) + chaves especificas
// de Ubisoft e GOG. Tudo best-effort; se nada bater, devolve vazio.
func launcherGames() []Game {
	var out []Game
	seen := map[string]bool{}
	add := func(name, loja, folder string) {
		folder = strings.TrimRight(filepath.Clean(folder), `\`)
		if folder == "" || len(folder) < 4 {
			return
		}
		if st, err := os.Stat(folder); err != nil || !st.IsDir() {
			return
		}
		k := strings.ToLower(folder)
		if seen[k] {
			return
		}
		ln := strings.ToLower(name)
		for _, b := range notGameNames {
			if strings.Contains(ln, b) {
				return
			}
		}
		if !isPlayable(name) {
			return
		}
		seen[k] = true
		out = append(out, Game{Nome: name, Loja: loja, Pasta: folder, Exe: findMainExe(folder, name)})
	}

	// 1) Uninstall (64 + 32 bits) filtrado por publisher de jogo.
	for _, base := range []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
		`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	} {
		subs, _ := ListSubkeys("HKLM", "", false, base)
		for _, s := range subs {
			path := base + `\` + s
			pub, _ := ReadString("HKLM", "", false, path, "Publisher")
			lp := strings.ToLower(pub)
			isGame := false
			for _, gp := range gamePublishers {
				if strings.Contains(lp, gp) {
					isGame = true
					break
				}
			}
			if !isGame {
				continue
			}
			name, _ := ReadString("HKLM", "", false, path, "DisplayName")
			loc, _ := ReadString("HKLM", "", false, path, "InstallLocation")
			if name != "" && loc != "" {
				add(name, lojaFromPub(pub), loc)
			}
		}
	}
	// 2) Ubisoft Connect — Installs\<id>\InstallDir.
	ub := `SOFTWARE\WOW6432Node\Ubisoft\Launcher\Installs`
	for _, s := range mustSubkeys("HKLM", ub) {
		if dir, ok := ReadString("HKLM", "", false, ub+`\`+s, "InstallDir"); ok && dir != "" {
			add(filepath.Base(strings.TrimRight(filepath.Clean(dir), `\`)), "Ubisoft", dir)
		}
	}
	// 3) GOG Galaxy — Games\<id>\{path,gameName}.
	gog := `SOFTWARE\WOW6432Node\GOG.com\Games`
	for _, s := range mustSubkeys("HKLM", gog) {
		p := gog + `\` + s
		dir, _ := ReadString("HKLM", "", false, p, "path")
		nm, _ := ReadString("HKLM", "", false, p, "gameName")
		if dir != "" {
			if nm == "" {
				nm = filepath.Base(dir)
			}
			add(nm, "GOG", dir)
		}
	}
	return out
}

func mustSubkeys(hive, path string) []string {
	s, _ := ListSubkeys(hive, "", false, path)
	return s
}

// ---- Steam ------------------------------------------------------------------

func steamGames(sid string) []Game {
	steam, ok := ReadString("HKCU", sid, true, `Software\Valve\Steam`, "SteamPath")
	if !ok || steam == "" {
		return nil
	}
	steam = filepath.FromSlash(steam)
	libs := []string{steam}
	if data, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, m := range reVdfPath.FindAllStringSubmatch(string(data), -1) {
			libs = append(libs, filepath.FromSlash(strings.ReplaceAll(m[1], `\\`, `\`)))
		}
	}
	seen := map[string]bool{}
	var out []Game
	for _, lib := range libs {
		manifests, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
		for _, mf := range manifests {
			data, err := os.ReadFile(mf)
			if err != nil {
				continue
			}
			nm := reAcfName.FindStringSubmatch(string(data))
			dir := reAcfDir.FindStringSubmatch(string(data))
			if nm == nil || dir == nil || !isPlayable(nm[1]) {
				continue
			}
			folder := filepath.Join(lib, "steamapps", "common", dir[1])
			key := strings.ToLower(folder)
			if seen[key] {
				continue
			}
			seen[key] = true
			if st, err := os.Stat(folder); err != nil || !st.IsDir() {
				continue
			}
			// extrai o AppID do nome do manifest (appmanifest_XXXXX.acf)
			appID := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(mf), "appmanifest_"), ".acf")
			coverURL := "https://cdn.steamstatic.com/steam/apps/" + appID + "/library_600x900.jpg"
			out = append(out, Game{Nome: nm[1], Loja: "Steam", AppID: appID, CoverURL: coverURL, Pasta: folder, Exe: findMainExe(folder, nm[1])})
		}
	}
	return out
}

// ---- Epic -------------------------------------------------------------------

func epicGames() []Game {
	dir := filepath.Join(os.Getenv("ProgramData"), "Epic", "EpicGamesLauncher", "Data", "Manifests")
	items, _ := filepath.Glob(filepath.Join(dir, "*.item"))
	var out []Game
	for _, it := range items {
		data, err := os.ReadFile(it)
		if err != nil {
			continue
		}
		var m struct {
			DisplayName      string `json:"DisplayName"`
			InstallLocation  string `json:"InstallLocation"`
			LaunchExecutable string `json:"LaunchExecutable"`
			VaultThumbnailUrl string `json:"VaultThumbnailUrl"`
			KeyImages        []struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"KeyImages"`
		}
		if json.Unmarshal(data, &m) != nil || m.DisplayName == "" || m.InstallLocation == "" {
			continue
		}
		if !isPlayable(m.DisplayName) {
			continue
		}
		exe := ""
		if m.LaunchExecutable != "" {
			exe = filepath.Join(m.InstallLocation, filepath.FromSlash(m.LaunchExecutable))
			if _, err := os.Stat(exe); err != nil {
				exe = findMainExe(m.InstallLocation, m.DisplayName)
			}
		} else {
			exe = findMainExe(m.InstallLocation, m.DisplayName)
		}
		// tenta extrair cover portrait dos KeyImages do manifest
		coverURL := ""
		for _, img := range m.KeyImages {
			t := strings.ToLower(img.Type)
			if (strings.Contains(t, "tall") || strings.Contains(t, "portrait") || t == "dieselgameboxtall" || t == "dieselstorefronttall") && img.URL != "" {
				coverURL = img.URL
				break
			}
		}
		if coverURL == "" && m.VaultThumbnailUrl != "" {
			coverURL = m.VaultThumbnailUrl
		}
		out = append(out, Game{Nome: m.DisplayName, Loja: "Epic", CoverURL: coverURL, Pasta: m.InstallLocation, Exe: exe})
	}
	return out
}

// findMainExe acha o executavel principal: prefere um .exe com nome parecido com
// o do jogo; senao o maior .exe. Ignora instaladores/redists/anticheat conhecidos.
func findMainExe(folder, name string) string {
	type cand struct {
		path string
		size int64
	}
	var cands []cand
	filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		low := strings.ToLower(d.Name())
		if !strings.HasSuffix(low, ".exe") {
			return nil
		}
		for _, b := range exeBlock {
			if strings.Contains(low, b) {
				return nil
			}
		}
		if info, e := d.Info(); e == nil {
			cands = append(cands, cand{path, info.Size()})
		}
		return nil
	})
	if len(cands) == 0 {
		return ""
	}
	norm := alnum(name)
	best, bestScore := "", int64(-1)
	for _, c := range cands {
		base := alnum(strings.TrimSuffix(filepath.Base(c.path), ".exe"))
		score := c.size / (1 << 20) // MB
		if base != "" && norm != "" && (strings.Contains(norm, base) || strings.Contains(base, norm)) {
			score += 1 << 40 // forte preferencia para o exe com nome do jogo
		}
		if score > bestScore {
			bestScore, best = score, c.path
		}
	}
	return best
}

// ---- Tweaks por-jogo (registro HKCU, reversiveis) ---------------------------

func gameTweakState(sid, exe string) (fso, gpu, prio bool) {
	if exe == "" {
		return
	}
	if v, ok := ReadString("HKCU", sid, true, regLayers, exe); ok && strings.Contains(v, fsoFlag) {
		fso = true
	}
	if v, ok := ReadString("HKCU", sid, true, regGpuPref, exe); ok && strings.Contains(v, "GpuPreference=2") {
		gpu = true
	}
	perf := ifeoBase + filepath.Base(exe) + `\PerfOptions`
	if v, ok := ReadInteger("HKLM", "", false, perf, "CpuPriorityClass"); ok && v == 3 {
		prio = true
	}
	return
}

// SetGameTweaks aplica/remove TODOS os tweaks por-jogo (todos reversiveis):
// FSO off, GPU alto desempenho, prioridade de CPU alta (IFEO) e exclusao da
// pasta no antivirus (Defender).
func SetGameTweaks(sid, exe, folder string, fso, gpu, prio, av bool) error {
	if exe != "" {
		if fso {
			WriteString("HKCU", sid, true, regLayers, exe, "~ "+fsoFlag)
		} else {
			RemoveValue("HKCU", sid, true, regLayers, exe)
		}
		if gpu {
			WriteString("HKCU", sid, true, regGpuPref, exe, gpuHighVal)
		} else {
			RemoveValue("HKCU", sid, true, regGpuPref, exe)
		}
		perf := ifeoBase + filepath.Base(exe) + `\PerfOptions`
		if prio {
			WriteDWord("HKLM", "", false, perf, "CpuPriorityClass", 3) // 3 = Alta
		} else {
			RemoveValue("HKLM", "", false, perf, "CpuPriorityClass")
		}
	}
	if folder != "" {
		defenderExclude(folder, av)
	}
	return nil
}

// ---- Defender (unica parte que usa PowerShell — nao ha API nativa limpa) ----

func psHidden(args ...string) *exec.Cmd {
	c := exec.Command("powershell", append([]string{"-NoProfile", "-NonInteractive", "-Command"}, args...)...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c
}

// defenderExclusions devolve o conjunto (minusculo) de pastas excluidas do Defender.
func defenderExclusions() map[string]bool {
	set := map[string]bool{}
	out, err := psHidden("(Get-MpPreference).ExclusionPath").Output()
	if err != nil {
		return set
	}
	for _, line := range strings.Split(string(out), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			set[strings.ToLower(p)] = true
		}
	}
	return set
}

// defenderExclude adiciona/remove a pasta das exclusoes do Defender.
func defenderExclude(folder string, add bool) {
	folder = strings.ReplaceAll(folder, "'", "''")
	cmd := "Add-MpPreference -ExclusionPath '" + folder + "'"
	if !add {
		cmd = "Remove-MpPreference -ExclusionPath '" + folder + "'"
	}
	psHidden(cmd).Run()
}
