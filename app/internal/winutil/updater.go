//go:build windows

package winutil

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UpdateInfo descreve uma versão nova disponível no GitHub.
type UpdateInfo struct {
	Available   bool   `json:"available"`
	Version     string `json:"version"`
	Notes       string `json:"notes"`
	DownloadURL string `json:"download_url"`
}

// UpdateOwner e UpdateRepo: preencha com seu usuário e repositório do GitHub.
// Altere aqui uma única vez após criar o repo e o sistema funciona automaticamente.
const (
	UpdateOwner = "unreallabbrasil-web"
	UpdateRepo  = "thazzdraco-releases"
)

var (
	updateMu   sync.RWMutex
	updateInfo *UpdateInfo
)

// GetUpdate retorna o resultado da última checagem (nil = sem novidade ainda).
func GetUpdate() *UpdateInfo {
	updateMu.RLock()
	defer updateMu.RUnlock()
	return updateInfo
}

// StartUpdateChecker inicia a goroutine de checagem em background.
// Primeira checagem ocorre 45s após a inicialização; depois, a cada 4 horas.
func StartUpdateChecker(currentVersion string) {
	go func() {
		time.Sleep(45 * time.Second)
		if info := fetchUpdate(currentVersion); info != nil {
			updateMu.Lock()
			updateInfo = info
			updateMu.Unlock()
		}
		ticker := time.NewTicker(4 * time.Hour)
		for range ticker.C {
			if info := fetchUpdate(currentVersion); info != nil {
				updateMu.Lock()
				updateInfo = info
				updateMu.Unlock()
			}
		}
	}()
}

// fetchUpdate consulta a GitHub Releases API e compara versões.
func fetchUpdate(current string) *UpdateInfo {
	if UpdateOwner == "SEU_USUARIO_GITHUB" {
		return nil // repo nao configurado
	}
	url := "https://api.github.com/repos/" + UpdateOwner + "/" + UpdateRepo + "/releases/latest"
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if json.NewDecoder(resp.Body).Decode(&release) != nil {
		return nil
	}

	remote := strings.TrimPrefix(release.TagName, "v")
	if !isNewer(remote, current) {
		return nil
	}

	downloadURL := ""
	for _, a := range release.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}

	return &UpdateInfo{
		Available:   true,
		Version:     remote,
		Notes:       release.Body,
		DownloadURL: downloadURL,
	}
}

// isNewer retorna true se remote > current (comparação semver simplificada).
func isNewer(remote, current string) bool {
	r := semverParts(remote)
	c := semverParts(current)
	for i := 0; i < 3; i++ {
		if r[i] > c[i] {
			return true
		}
		if r[i] < c[i] {
			return false
		}
	}
	return false
}

func semverParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		out[i] = n
	}
	return out
}
