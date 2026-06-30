//go:build windows

package winutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	coverMu    sync.Mutex
	coverMap   = map[string]string{} // nome_lower → URL (vazio = nao encontrado)
	coverDirty = false
	coverLoaded = false
)

func coverCachePath() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "ThazzDraco", "covers.json")
}

func loadCoverCache() {
	if coverLoaded {
		return
	}
	coverLoaded = true
	raw, err := os.ReadFile(coverCachePath())
	if err != nil {
		return
	}
	coverMu.Lock()
	defer coverMu.Unlock()
	json.Unmarshal(raw, &coverMap) //nolint:errcheck
}

func flushCoverCache() {
	coverMu.Lock()
	if !coverDirty {
		coverMu.Unlock()
		return
	}
	raw, _ := json.Marshal(coverMap)
	coverDirty = false
	coverMu.Unlock()
	p := coverCachePath()
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, raw, 0o644) //nolint:errcheck
}

// steamSearch retorna o app_id Steam do primeiro resultado que case com o nome.
func steamSearch(name string) int {
	u := "https://store.steampowered.com/api/storesearch/?term=" + url.QueryEscape(name) + "&l=english&cc=US"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var result struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil || len(result.Items) == 0 {
		return 0
	}

	nameLower := strings.ToLower(name)
	for _, item := range result.Items {
		n := strings.ToLower(item.Name)
		if n == nameLower || strings.Contains(n, nameLower) || strings.Contains(nameLower, n) {
			return item.ID
		}
	}
	return result.Items[0].ID
}

// ResolveCoverURL devolve a URL da capa portrait de um jogo, buscando na Steam
// pelo nome. Usa cache em disco; retorna "" se nao encontrado.
func ResolveCoverURL(name string) string {
	loadCoverCache()

	key := strings.ToLower(strings.TrimSpace(name))

	coverMu.Lock()
	if v, ok := coverMap[key]; ok {
		coverMu.Unlock()
		return v
	}
	coverMu.Unlock()

	appID := steamSearch(name)
	var coverURL string
	if appID > 0 {
		coverURL = fmt.Sprintf("https://cdn.steamstatic.com/steam/apps/%d/library_600x900.jpg", appID)
	}

	coverMu.Lock()
	coverMap[key] = coverURL
	coverDirty = true
	coverMu.Unlock()

	go flushCoverCache()
	return coverURL
}

// ResolveCoverURLs resolve capas para uma lista de jogos sem cover_url em paralelo,
// com timeout global de 6 segundos. Modifica os elementos no slice passado.
func ResolveCoverURLs(games []Game) {
	type job struct{ i int; name string }
	var jobs []job
	for i, g := range games {
		if g.CoverURL == "" {
			jobs = append(jobs, job{i, g.Nome})
		}
	}
	if len(jobs) == 0 {
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // max 4 requisicoes simultaneas
	for _, j := range jobs {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if u := ResolveCoverURL(name); u != "" {
				games[idx].CoverURL = u
			}
		}(j.i, j.name)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
	}
}
