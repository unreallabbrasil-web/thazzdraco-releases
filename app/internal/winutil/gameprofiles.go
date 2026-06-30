//go:build windows

package winutil

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed gameprofiles.json
var gameProfilesRaw []byte

// GameProfile é o perfil curado de um título: dicas de configuração in-game
// (aplicadas no menu do próprio jogo — nunca editamos arquivos de jogo com
// anti-cheat) + quais tweaks de sistema mais ajudam.
type GameProfile struct {
	ID    string   `json:"id"`
	Nome  string   `json:"nome"`
	Tipo  string   `json:"tipo"`
	Match []string `json:"match"`
	Dicas []string `json:"dicas"`
	Notas string   `json:"notas,omitempty"`
	Tweaks string  `json:"tweaks,omitempty"`
}

var gameProfiles []GameProfile

func loadGameProfiles() {
	if gameProfiles != nil {
		return
	}
	var doc struct {
		Perfis []GameProfile `json:"perfis"`
	}
	if json.Unmarshal(gameProfilesRaw, &doc) == nil {
		gameProfiles = doc.Perfis
	}
	if gameProfiles == nil {
		gameProfiles = []GameProfile{}
	}
}

// MatchProfile acha o perfil curado de um jogo pelo nome/exe (best-effort).
func MatchProfile(nome, exe string) *GameProfile {
	loadGameProfiles()
	norm := alnum(nome) + " " + alnum(exe)
	for i := range gameProfiles {
		for _, tok := range gameProfiles[i].Match {
			t := alnum(tok)
			if t != "" && strings.Contains(norm, t) {
				return &gameProfiles[i]
			}
		}
	}
	return nil
}
