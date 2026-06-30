// Package engine implementa o motor declarativo de otimizacao: carrega as regras
// embutidas, detecta o estado de cada uma, aplica com snapshot (reversivel) e
// desfaz. Invariantes de seguranca (espelham docs/GUIA-OTIMIZACAO-GAMING.md):
//   - ponto de restauracao + snapshot por-valor antes de QUALQUER escrita;
//   - undo regrava o valor antigo (ou apaga se nao existia);
//   - limpeza so em pastas temporarias/cache (allowlist) e com consentimento;
//   - nunca toca em documentos pessoais (toca_dados_pessoais sempre false).
package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed assets/rules.json
var rulesJSON []byte

//go:embed assets/presets.json
var presetsJSON []byte

// Gate: arvore de condicoes sobre o perfil do PC.
type Gate struct {
	Campo    string `json:"campo"`
	Op       string `json:"op"`
	Valor    any    `json:"valor"`
	Todos    []Gate `json:"todos"`
	Qualquer []Gate `json:"qualquer"`
}

// RegVal cobre tanto detect ({name, esperado}) quanto action ({name, value, value_type}).
type RegVal struct {
	Name      string `json:"name"`
	Esperado  any    `json:"esperado"`
	Value     any    `json:"value"`
	ValueType string `json:"value_type"`
}

type Detect struct {
	Tipo         string          `json:"tipo"`
	Hive         string          `json:"hive"`
	HkcuRealUser bool            `json:"hkcu_real_user"`
	Path         string          `json:"path"`
	Base         string          `json:"base"`
	Valores      []RegVal        `json:"valores"`
	Sub          string          `json:"sub"`
	Setting      string          `json:"setting"`
	Esperado     json.RawMessage `json:"esperado"`
	Servicos     []string        `json:"servicos"`
	Alvos        []string        `json:"alvos"`
	Atual        string          `json:"atual"`
	Nominal      string          `json:"nominal"`
	Quando       string          `json:"quando"`
}

type Action struct {
	Tipo         string     `json:"tipo"`
	Hive         string     `json:"hive"`
	HkcuRealUser bool       `json:"hkcu_real_user"`
	Path         string     `json:"path"`
	Base         string     `json:"base"`
	Valores      []RegVal   `json:"valores"`
	Servicos     []string   `json:"servicos"`
	Startup      string     `json:"startup"`
	Alvos        []string   `json:"alvos"`
	Comandos     [][]string `json:"comandos"`
}

type Undo struct {
	Tipo string `json:"tipo"`
}

type Rule struct {
	ID                  string  `json:"id"`
	Parte               int     `json:"parte"`
	Categoria           string  `json:"categoria"`
	Titulo              string  `json:"titulo"`
	Descricao           string  `json:"descricao"`
	Tier                string  `json:"tier"`
	Modo                string  `json:"modo"`
	Impacto             string  `json:"impacto"`
	RequerReboot        bool    `json:"requer_reboot"`
	RequerConsentimento bool    `json:"requer_consentimento"`
	TocaDadosPessoais   bool    `json:"toca_dados_pessoais"`
	HardwareGate        *Gate   `json:"hardware_gate"`
	Detect              Detect  `json:"detect"`
	Action              *Action `json:"action"`
	Undo                *Undo   `json:"undo"`
	Orientacao          string  `json:"orientacao"`
}

type ruleFile struct {
	SchemaVersion string `json:"schema_version"`
	Rules         []Rule `json:"rules"`
}

type Preset struct {
	ID        string   `json:"id"`
	Nome      string   `json:"nome"`
	Descricao string   `json:"descricao"`
	IDs       []string `json:"ids"`
}

type presetFile struct {
	Presets []Preset `json:"presets"`
}

// LoadRules devolve as regras embutidas no binario.
func LoadRules() ([]Rule, error) {
	var rf ruleFile
	if err := json.Unmarshal(rulesJSON, &rf); err != nil {
		return nil, fmt.Errorf("rules.json invalido: %w", err)
	}
	return rf.Rules, nil
}

// LoadPresets devolve os perfis curados embutidos.
func LoadPresets() ([]Preset, error) {
	var pf presetFile
	if err := json.Unmarshal(presetsJSON, &pf); err != nil {
		return nil, fmt.Errorf("presets.json invalido: %w", err)
	}
	return pf.Presets, nil
}
