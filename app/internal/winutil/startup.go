//go:build windows

package winutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StartupEntry é um programa que inicia com o Windows.
type StartupEntry struct {
	Name     string `json:"name"`     // nome para exibir
	Key      string `json:"key"`      // nome do valor em StartupApproved (toggle)
	Command  string `json:"command"`  // caminho/comando
	Location string `json:"location"` // rótulo amigável
	Kind     string `json:"kind"`     // run-hkcu | run-hklm | run-hklm32 | folder-user | folder-common
	Enabled  bool   `json:"enabled"`
}

const (
	approvedRun    = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	approvedRun32  = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run32`
	approvedFolder = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder`
)

// ListStartup lista as entradas de inicializacao (chaves Run + pastas Startup).
// O estado habilitado/desabilitado vem do StartupApproved (mesma fonte do
// Gerenciador de Tarefas).
func ListStartup(sid string) []StartupEntry {
	var out []StartupEntry

	addRun := func(kind, label, hive string, hkcuReal bool, runPath, approvedHive string, approvedReal bool, approvedPath string) {
		for _, nv := range EnumStringValues(hive, sid, hkcuReal, runPath) {
			out = append(out, StartupEntry{
				Name: nv.Name, Key: nv.Name, Command: nv.Value, Location: label, Kind: kind,
				Enabled: startupEnabled(approvedHive, sid, approvedReal, approvedPath, nv.Name),
			})
		}
	}
	addRun("run-hkcu", "Usuário", "HKCU", true, `Software\Microsoft\Windows\CurrentVersion\Run`, "HKCU", true, approvedRun)
	addRun("run-hklm", "Sistema", "HKLM", false, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKLM", false, approvedRun)
	addRun("run-hklm32", "Sistema (32-bit)", "HKLM", false, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, "HKLM", false, approvedRun32)

	if prof := RealUserProfileDir(sid); prof != "" {
		dir := filepath.Join(prof, `AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup`)
		out = append(out, folderStartup(dir, "folder-user", "Pasta do usuário", "HKCU", sid, true)...)
	}
	common := filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs\Startup`)
	out = append(out, folderStartup(common, "folder-common", "Pasta comum", "HKLM", sid, false)...)

	return out
}

func startupEnabled(hive, sid string, hkcuReal bool, path, name string) bool {
	b, ok := ReadBinary(hive, sid, hkcuReal, path, name)
	if !ok || len(b) == 0 {
		return true // sem marca de aprovacao = habilitado (padrao)
	}
	return b[0]%2 == 0 // par (2) = habilitado; impar (3) = desabilitado
}

func folderStartup(dir, kind, label, approvedHive, sid string, approvedReal bool) []StartupEntry {
	var out []StartupEntry
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		fn := e.Name()
		if strings.EqualFold(fn, "desktop.ini") {
			continue
		}
		out = append(out, StartupEntry{
			Name: strings.TrimSuffix(fn, filepath.Ext(fn)), Key: fn,
			Command: filepath.Join(dir, fn), Location: label, Kind: kind,
			Enabled: startupEnabled(approvedHive, sid, approvedReal, approvedFolder, fn),
		})
	}
	return out
}

// approvedLoc devolve onde gravar a marca de aprovacao para cada tipo.
func approvedLoc(kind string) (hive string, hkcuReal bool, path string) {
	switch kind {
	case "run-hkcu":
		return "HKCU", true, approvedRun
	case "run-hklm":
		return "HKLM", false, approvedRun
	case "run-hklm32":
		return "HKLM", false, approvedRun32
	case "folder-user":
		return "HKCU", true, approvedFolder
	case "folder-common":
		return "HKLM", false, approvedFolder
	}
	return "", false, ""
}

// SetStartupEnabled habilita/desabilita uma entrada via StartupApproved. Nao
// apaga a entrada Run nem o atalho — apenas marca o estado (totalmente reversivel,
// igual ao Gerenciador de Tarefas).
func SetStartupEnabled(kind, key string, enabled bool, sid string) error {
	hive, hkcuReal, path := approvedLoc(kind)
	if path == "" {
		return fmt.Errorf("tipo de inicializacao invalido: %s", kind)
	}
	blob := []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} // habilitado
	if !enabled {
		blob[0] = 3 // desabilitado
	}
	return WriteBinary(hive, sid, hkcuReal, path, key, blob)
}
