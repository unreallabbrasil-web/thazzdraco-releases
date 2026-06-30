//go:build windows

package winutil

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// ---- exported types ---------------------------------------------------------

// GameCfgField descreve um campo editável na aba "Config Gráfica".
type GameCfgField struct {
	Key     string   `json:"key"`             // chave(s) no arquivo, comma-separated para multi-key
	Label   string   `json:"label"`
	Type    string   `json:"type"`            // "res" | "enum" | "fps_ue" | "fps_int"
	Options []CfgOpt `json:"options,omitempty"`
}

// CfgOpt é uma opção de seleção.
type CfgOpt struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// GameCfgResult é retornado para o frontend.
type GameCfgResult struct {
	Supported  bool              `json:"supported"`
	GameLabel  string            `json:"game_label,omitempty"`
	ConfigFile string            `json:"config_file,omitempty"`
	Values     map[string]string `json:"values,omitempty"`
	Fields     []GameCfgField    `json:"fields,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// ---- internal catalog -------------------------------------------------------

type gameDef struct {
	exeSubstr  string
	gameLabel  string
	configPath string // suporta: %LOCALAPPDATA%, %APPDATA%, %USERPROFILE%, {steam_userdata}, {glob:...}
	format     string // "ueini" | "vdf" | "frostbite" | "arma" | "flatini"
	iniSection string // para "ueini" e "flatini"
	fields     []GameCfgField
}

var (
	resOpts = []CfgOpt{
		{Label: "1280 × 720", Value: "1280x720"},
		{Label: "1600 × 900", Value: "1600x900"},
		{Label: "1920 × 1080 (FHD)", Value: "1920x1080"},
		{Label: "2560 × 1440 (QHD)", Value: "2560x1440"},
		{Label: "3840 × 2160 (4K)", Value: "3840x2160"},
	}
	fpsCaps = []CfgOpt{
		{Label: "Sem limite", Value: "0"},
		{Label: "60 FPS", Value: "60"},
		{Label: "120 FPS", Value: "120"},
		{Label: "144 FPS", Value: "144"},
		{Label: "165 FPS", Value: "165"},
		{Label: "240 FPS", Value: "240"},
		{Label: "360 FPS", Value: "360"},
	}
	qualOpts = []CfgOpt{
		{Label: "Baixo", Value: "0"},
		{Label: "Médio", Value: "1"},
		{Label: "Alto", Value: "2"},
		{Label: "Ultra", Value: "3"},
	}
)

// ueFields retorna campos padrão para jogos Unreal Engine 4/5.
func ueFields(fsOpts []CfgOpt) []GameCfgField {
	if fsOpts == nil {
		fsOpts = []CfgOpt{
			{Label: "Tela cheia", Value: "1"},
			{Label: "Sem borda (Borderless)", Value: "2"},
			{Label: "Janela", Value: "0"},
		}
	}
	return []GameCfgField{
		{Key: "ResolutionSizeX,ResolutionSizeY", Label: "Resolução", Type: "res", Options: resOpts},
		{Key: "FullscreenMode", Label: "Modo de tela", Type: "enum", Options: fsOpts},
		{Key: "FrameRateLimit", Label: "Limite de FPS", Type: "fps_ue", Options: fpsCaps},
		{Key: "sg.ShadowQuality", Label: "Sombras", Type: "enum", Options: qualOpts},
		{Key: "sg.TextureQuality", Label: "Texturas", Type: "enum", Options: qualOpts},
		{Key: "sg.PostProcessQuality", Label: "Pós-processamento", Type: "enum", Options: qualOpts},
	}
}

var gameCatalog = []gameDef{
	{
		// Valorant tem UUID no path: %LOCALAPPDATA%\VALORANT\Saved\Config\<UUID>\Windows\GameUserSettings.ini
		exeSubstr:  "VALORANT-Win64-Shipping",
		gameLabel:  "VALORANT",
		configPath: `{glob:%LOCALAPPDATA%\VALORANT\Saved\Config\*\Windows\GameUserSettings.ini}`,
		format:     "ueini",
		iniSection: "/Script/ShooterGame.ShooterGameUserSettings",
		fields: func() []GameCfgField {
			f := ueFields([]CfgOpt{
				{Label: "Sem borda (Borderless) — recomendado", Value: "1"},
				{Label: "Tela cheia exclusiva", Value: "0"},
				{Label: "Janela", Value: "2"},
			})
			return f
		}(),
	},
	{
		exeSubstr:  "FortniteClient-Win64-Shipping",
		gameLabel:  "Fortnite",
		configPath: `%LOCALAPPDATA%\FortniteGame\Saved\Config\WindowsClient\GameUserSettings.ini`,
		format:     "ueini",
		iniSection: "/Script/Engine.GameUserSettings",
		fields:     ueFields(nil),
	},
	{
		exeSubstr:  "TslGame",
		gameLabel:  "PUBG",
		configPath: `%LOCALAPPDATA%\TslGame\Saved\Config\WindowsNoEditor\GameUserSettings.ini`,
		format:     "ueini",
		iniSection: "/Script/TslGame.TslGameUserSettings",
		fields: []GameCfgField{
			{Key: "ResolutionSizeX,ResolutionSizeY", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "FullscreenMode", Label: "Modo de tela", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "0"},
				{Label: "Sem borda (Borderless)", Value: "1"},
				{Label: "Janela", Value: "2"},
			}},
			{Key: "FrameRateLimit", Label: "Limite de FPS", Type: "fps_ue", Options: fpsCaps},
			{Key: "sg.ShadowQuality", Label: "Sombras", Type: "enum", Options: qualOpts},
			{Key: "sg.TextureQuality", Label: "Texturas", Type: "enum", Options: qualOpts},
		},
	},
	{
		exeSubstr:  "cs2",
		gameLabel:  "CS2",
		configPath: `{steam_userdata}\730\local\cfg\cs2_video.txt`,
		format:     "vdf",
		fields: []GameCfgField{
			{Key: "setting.defaultres,setting.defaultresheight", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "setting.fullscreen", Label: "Modo de tela", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "1"},
				{Label: "Sem borda (Borderless)", Value: "0"},
				{Label: "Janela", Value: "0"},
			}},
			{Key: "setting.gpu_level", Label: "Detalhe de shader", Type: "enum", Options: qualOpts},
			{Key: "setting.gpu_mem_level", Label: "Detalhe de modelo/textura", Type: "enum", Options: qualOpts},
		},
	},
	{
		exeSubstr:  "r5apex",
		gameLabel:  "Apex Legends",
		configPath: `%USERPROFILE%\Saved Games\Respawn\Apex\local\videoconfig.txt`,
		format:     "vdf",
		fields: []GameCfgField{
			{Key: "setting.defaultres,setting.defaultresheight", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "setting.fullscreen", Label: "Modo de tela", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "1"},
				{Label: "Sem borda (Borderless) — precisa setting.nowindowborder=1", Value: "0"},
			}},
			{Key: "setting.mat_picmip", Label: "Qualidade de textura", Type: "enum", Options: []CfgOpt{
				{Label: "Alta", Value: "0"},
				{Label: "Média", Value: "1"},
				{Label: "Baixa", Value: "2"},
			}},
		},
	},
	{
		exeSubstr:  "RocketLeague",
		gameLabel:  "Rocket League",
		configPath: `%USERPROFILE%\Documents\My Games\Rocket League\TAGame\Config\TASystemSettings.ini`,
		format:     "flatini",
		iniSection: "SystemSettings",
		fields: []GameCfgField{
			{Key: "ResX,ResY", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "Fullscreen", Label: "Tela cheia", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "True"},
				{Label: "Janela", Value: "False"},
			}},
			{Key: "CustomFPS", Label: "Limite de FPS", Type: "fps_int", Options: append(
				[]CfgOpt{{Label: "Sem limite (9999)", Value: "9999"}},
				fpsCaps[1:]...,
			)},
		},
	},
	{
		exeSubstr:  "r6-win64-shipping",
		gameLabel:  "Rainbow Six Siege",
		configPath: `{glob:%USERPROFILE%\Documents\My Games\Rainbow Six - Siege\*\GameSettings.ini}`,
		format:     "flatini",
		iniSection: "DISPLAY_SETTINGS",
		fields: []GameCfgField{
			{Key: "ResolutionWidth,ResolutionHeight", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "WindowMode", Label: "Modo de tela", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "0"},
				{Label: "Janela", Value: "1"},
				{Label: "Sem borda (Borderless)", Value: "2"},
			}},
		},
	},
	{
		exeSubstr:  "bf2042",
		gameLabel:  "Battlefield 2042",
		configPath: `%USERPROFILE%\Documents\Battlefield 2042\settings\PROFSAVE_profile`,
		format:     "frostbite",
		fields: []GameCfgField{
			{Key: "GstRender.ResolutionWidth,GstRender.ResolutionHeight", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "GstRender.FullscreenMode", Label: "Modo de tela", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "1"},
				{Label: "Sem borda (Borderless)", Value: "2"},
			}},
			{Key: "GstRender.OverallGraphicsQuality", Label: "Qualidade geral", Type: "enum", Options: []CfgOpt{
				{Label: "Baixo", Value: "0"}, {Label: "Médio", Value: "1"},
				{Label: "Alto", Value: "2"}, {Label: "Ultra", Value: "3"},
			}},
		},
	},
	{
		exeSubstr:  "bf1_x64.exe",
		gameLabel:  "Battlefield 1",
		configPath: `%USERPROFILE%\Documents\Battlefield 1\settings\PROFSAVE_profile`,
		format:     "frostbite",
		fields: []GameCfgField{
			{Key: "GstRender.ResolutionWidth,GstRender.ResolutionHeight", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "GstRender.FullscreenEnabled", Label: "Tela cheia", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "1"},
				{Label: "Janela/Borderless", Value: "0"},
			}},
			{Key: "GstRender.OverallGraphicsQuality", Label: "Qualidade geral", Type: "enum", Options: []CfgOpt{
				{Label: "Baixo", Value: "0"}, {Label: "Médio", Value: "1"},
				{Label: "Alto", Value: "2"}, {Label: "Ultra", Value: "3"},
			}},
		},
	},
	{
		exeSubstr:  "DayZ_x64",
		gameLabel:  "DayZ",
		configPath: `%USERPROFILE%\Documents\DayZ\DayZ.cfg`,
		format:     "arma",
		fields: []GameCfgField{
			{Key: "Resolution_W,Resolution_H", Label: "Resolução", Type: "res", Options: resOpts},
			{Key: "fullscreen", Label: "Modo de tela", Type: "enum", Options: []CfgOpt{
				{Label: "Tela cheia", Value: "1"},
				{Label: "Janela", Value: "0"},
			}},
		},
	},
}

// ---- path resolution --------------------------------------------------------

func expandGamePath(p string) string {
	local := os.Getenv("LOCALAPPDATA")
	appdata := os.Getenv("APPDATA")
	profile := os.Getenv("USERPROFILE")
	p = strings.ReplaceAll(p, "%LOCALAPPDATA%", local)
	p = strings.ReplaceAll(p, "%APPDATA%", appdata)
	p = strings.ReplaceAll(p, "%USERPROFILE%", profile)
	if strings.Contains(p, "{steam_userdata}") {
		p = strings.ReplaceAll(p, "{steam_userdata}", steamUserdataPath())
	}
	if strings.HasPrefix(p, "{glob:") && strings.HasSuffix(p, "}") {
		pattern := expandGamePath(p[6 : len(p)-1])
		if matches, err := filepath.Glob(pattern); err == nil && len(matches) > 0 {
			return matches[0]
		}
		return ""
	}
	return p
}

func steamUserdataPath() string {
	steam := steamRootPath()
	if steam == "" {
		return ""
	}
	ud := filepath.Join(steam, "userdata")
	entries, err := os.ReadDir(ud)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "0" && e.Name() != "anonymous" {
			return filepath.Join(ud, e.Name())
		}
	}
	if len(entries) > 0 && entries[0].IsDir() {
		return filepath.Join(ud, entries[0].Name())
	}
	return ""
}

func steamRootPath() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Valve\Steam`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if v, _, e := k.GetStringValue("SteamPath"); e == nil && v != "" {
			return filepath.FromSlash(v)
		}
	}
	for _, p := range []string{`C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ---- public API -------------------------------------------------------------

// ReadGameConfig lê a config gráfica do jogo identificado pelo caminho do .exe.
func ReadGameConfig(exePath string) GameCfgResult {
	if exePath == "" {
		return GameCfgResult{Supported: false}
	}
	exeBase := strings.ToLower(filepath.Base(exePath))
	for _, def := range gameCatalog {
		if strings.Contains(exeBase, strings.ToLower(def.exeSubstr)) {
			return readGameDef(def)
		}
	}
	return GameCfgResult{Supported: false}
}

// WriteGameConfig escreve novos valores no arquivo de config do jogo.
func WriteGameConfig(exePath string, newVals map[string]string) error {
	exeBase := strings.ToLower(filepath.Base(exePath))
	for _, def := range gameCatalog {
		if strings.Contains(exeBase, strings.ToLower(def.exeSubstr)) {
			return writeGameDef(def, newVals)
		}
	}
	return fmt.Errorf("jogo não suportado")
}

// ---- internal read/write ----------------------------------------------------

func readGameDef(def gameDef) GameCfgResult {
	cfgPath := expandGamePath(def.configPath)
	if cfgPath == "" {
		return GameCfgResult{Supported: true, GameLabel: def.gameLabel,
			Error: "pasta de config não encontrada — abra o jogo pelo menos uma vez"}
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return GameCfgResult{Supported: true, GameLabel: def.gameLabel, ConfigFile: cfgPath,
			Error: "arquivo não encontrado — abra o jogo pelo menos uma vez para gerar a config"}
	}
	var vals map[string]string
	switch def.format {
	case "ueini":
		vals = parseUEINI(string(data), def.iniSection)
	case "flatini":
		vals = parseFlatINI(string(data), def.iniSection)
	case "vdf":
		vals = parseVDF(string(data))
	case "frostbite":
		vals = parseFrostbite(string(data))
	case "arma":
		vals = parseArma(string(data))
	}
	return GameCfgResult{Supported: true, GameLabel: def.gameLabel,
		ConfigFile: cfgPath, Values: vals, Fields: def.fields}
}

func writeGameDef(def gameDef, newVals map[string]string) error {
	cfgPath := expandGamePath(def.configPath)
	if cfgPath == "" {
		return fmt.Errorf("caminho de config não resolvido — abra o jogo pelo menos uma vez")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("arquivo não encontrado: %w", err)
	}
	var result string
	switch def.format {
	case "ueini":
		result = writeUEINI(string(data), def.iniSection, newVals)
	case "flatini":
		result = writeFlatINI(string(data), def.iniSection, newVals)
	case "vdf":
		result = writeVDF(string(data), newVals)
	case "frostbite":
		result = writeFrostbite(string(data), newVals)
	case "arma":
		result = writeArma(string(data), newVals)
	default:
		return fmt.Errorf("formato não suportado: %s", def.format)
	}
	return os.WriteFile(cfgPath, []byte(result), 0644)
}

// ---- UE INI (section-aware) -------------------------------------------------

func parseUEINI(content, section string) map[string]string {
	vals := map[string]string{}
	inSec := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			sec := strings.TrimPrefix(strings.TrimSuffix(line, "]"), "[")
			inSec = sec == section
			continue
		}
		if inSec {
			if idx := strings.IndexByte(line, '='); idx > 0 {
				vals[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return vals
}

func writeUEINI(content, section string, newVals map[string]string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inSec := false
	written := map[string]bool{}
	var out []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			if inSec {
				// flush missing keys before leaving section
				for k, v := range newVals {
					if !written[k] {
						out = append(out, k+"="+v)
						written[k] = true
					}
				}
			}
			sec := strings.TrimPrefix(strings.TrimSuffix(trimmed, "]"), "[")
			inSec = sec == section
			out = append(out, line)
			continue
		}
		if inSec {
			if idx := strings.IndexByte(trimmed, '='); idx > 0 {
				k := strings.TrimSpace(trimmed[:idx])
				if nv, ok := newVals[k]; ok {
					out = append(out, k+"="+nv)
					written[k] = true
					continue
				}
			}
		}
		out = append(out, line)
		if inSec && i == len(lines)-1 {
			for k, v := range newVals {
				if !written[k] {
					out = append(out, k+"="+v)
				}
			}
		}
	}
	return strings.Join(out, "\r\n")
}

// ---- FlatINI (seção simples, keys únicas) -----------------------------------

func parseFlatINI(content, section string) map[string]string {
	if section == "" {
		return parseUEINI(content, section)
	}
	return parseUEINI(content, section)
}

func writeFlatINI(content, section string, newVals map[string]string) string {
	// Para jogos como Rocket League e R6 que têm seções mas keys únicas no arquivo.
	// Estratégia: substituir no lugar independente de seção (mais simples e seguro).
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	written := map[string]bool{}
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx := strings.IndexByte(trimmed, '='); idx > 0 {
			k := strings.TrimSpace(trimmed[:idx])
			if nv, ok := newVals[k]; ok {
				out = append(out, k+"="+nv)
				written[k] = true
				continue
			}
		}
		out = append(out, line)
	}
	// append keys not found
	if len(written) < len(newVals) {
		for k, v := range newVals {
			if !written[k] {
				out = append(out, k+"="+v)
			}
		}
	}
	return strings.Join(out, "\r\n")
}

// ---- VDF (Source/Apex) ------------------------------------------------------

var vdfKVRe = regexp.MustCompile(`"([^"]+)"\s+"([^"]*)"`)

func parseVDF(content string) map[string]string {
	vals := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		m := vdfKVRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) == 3 {
			vals[m[1]] = m[2]
		}
	}
	return vals
}

func writeVDF(content string, newVals map[string]string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var out []string
	for _, line := range lines {
		m := vdfKVRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) == 3 {
			if nv, ok := newVals[m[1]]; ok {
				out = append(out, fmt.Sprintf("\t\t\"%s\"\t\t\"%s\"", m[1], nv))
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\r\n")
}

// ---- Frostbite (Battlefield) ------------------------------------------------

func parseFrostbite(content string) map[string]string {
	vals := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 2 {
			vals[parts[0]] = parts[1]
		}
	}
	return vals
}

func writeFrostbite(content string, newVals map[string]string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	written := map[string]bool{}
	var out []string
	for _, line := range lines {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 2 {
			if nv, ok := newVals[parts[0]]; ok {
				out = append(out, parts[0]+" "+nv)
				written[parts[0]] = true
				continue
			}
		}
		out = append(out, line)
	}
	for k, v := range newVals {
		if !written[k] {
			out = append(out, k+" "+v)
		}
	}
	return strings.Join(out, "\r\n")
}

// ---- ArmA / DayZ ------------------------------------------------------------

func parseArma(content string) map[string]string {
	vals := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.IndexByte(line, '='); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSuffix(strings.TrimSpace(line[idx+1:]), ";")
			vals[k] = v
		}
	}
	return vals
}

func writeArma(content string, newVals map[string]string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	written := map[string]bool{}
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx := strings.IndexByte(trimmed, '='); idx > 0 {
			k := strings.TrimSpace(trimmed[:idx])
			if nv, ok := newVals[k]; ok {
				out = append(out, k+"="+nv+";")
				written[k] = true
				continue
			}
		}
		out = append(out, line)
	}
	for k, v := range newVals {
		if !written[k] {
			out = append(out, k+"="+v+";")
		}
	}
	return strings.Join(out, "\r\n")
}
