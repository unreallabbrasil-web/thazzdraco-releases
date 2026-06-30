//go:build windows

package winutil

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// CustomPreset é um perfil definido pelo usuário.
type CustomPreset struct {
	ID        string   `json:"id"`
	Nome      string   `json:"nome"`
	Descricao string   `json:"descricao"`
	IDs       []string `json:"ids"`
}

type customPresetsFile struct {
	Presets []CustomPreset `json:"presets"`
}

func customPresetsPath(sid string) string {
	prof := RealUserProfileDir(sid)
	return filepath.Join(prof, "AppData", "Roaming", "ThazzDraco", "custom_presets.json")
}

func loadCustomPresetsFile(sid string) customPresetsFile {
	p := customPresetsPath(sid)
	data, err := os.ReadFile(p)
	if err != nil {
		return customPresetsFile{}
	}
	var f customPresetsFile
	json.Unmarshal(data, &f)
	return f
}

func saveCustomPresetsFile(sid string, f customPresetsFile) error {
	p := customPresetsPath(sid)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

func randomID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "custom_" + hex.EncodeToString(b)
}

// ListCustomPresets retorna os presets customizados do usuário.
func ListCustomPresets(sid string) []CustomPreset {
	f := loadCustomPresetsFile(sid)
	if f.Presets == nil {
		return []CustomPreset{}
	}
	return f.Presets
}

// SaveCustomPreset cria ou atualiza um preset customizado.
// Se id == "", gera um novo ID.
func SaveCustomPreset(sid, id, nome, descricao string, ids []string) (CustomPreset, error) {
	f := loadCustomPresetsFile(sid)
	p := CustomPreset{
		ID:        id,
		Nome:      nome,
		Descricao: descricao,
		IDs:       ids,
	}
	if p.ID == "" {
		p.ID = randomID()
	}
	// Atualizar existente ou adicionar
	found := false
	for i, pr := range f.Presets {
		if pr.ID == p.ID {
			f.Presets[i] = p
			found = true
			break
		}
	}
	if !found {
		f.Presets = append(f.Presets, p)
	}
	return p, saveCustomPresetsFile(sid, f)
}

// DeleteCustomPreset remove um preset pelo ID.
func DeleteCustomPreset(sid, id string) error {
	f := loadCustomPresetsFile(sid)
	filtered := f.Presets[:0]
	for _, p := range f.Presets {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	f.Presets = filtered
	return saveCustomPresetsFile(sid, f)
}

// GetCustomPresetIDs retorna os IDs de regras de um preset customizado.
// Retorna nil se o preset não existir.
func GetCustomPresetIDs(sid, id string) []string {
	f := loadCustomPresetsFile(sid)
	for _, p := range f.Presets {
		if p.ID == id {
			return p.IDs
		}
	}
	return nil
}
