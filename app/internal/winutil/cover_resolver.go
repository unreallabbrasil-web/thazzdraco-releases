//go:build windows

package winutil

import (
	"context"
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
	coverMu.Lock()
	defer coverMu.Unlock()
	if coverLoaded {
		return
	}
	coverLoaded = true
	raw, err := os.ReadFile(coverCachePath())
	if err != nil {
		return
	}
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

// steamSearch retorna (app_id, completou). completou=false indica erro de
// transporte (rede/HTTP): o chamador NÃO deve cachear nesse caso. completou=true
// com id=0 significa "busca ok, sem correspondência" — aí pode cachear o vazio.
func steamSearch(ctx context.Context, name string) (int, bool) {
	u := "https://store.steampowered.com/api/storesearch/?term=" + url.QueryEscape(name) + "&l=english&cc=US"
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return 0, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false // erro de rede — não cacheia
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, false // rate-limit/erro do servidor — não cacheia
	}

	var result struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return 0, false
	}

	nameLower := strings.ToLower(name)
	for _, item := range result.Items {
		n := strings.ToLower(item.Name)
		if n == nameLower || strings.Contains(n, nameLower) || strings.Contains(nameLower, n) {
			return item.ID, true
		}
	}
	// Nenhum resultado casou o nome: melhor SEM capa do que a capa de outro jogo
	// (Items[0] poderia ser um título totalmente diferente).
	return 0, true
}

// ResolveCoverURL devolve a URL da capa portrait de um jogo, buscando na Steam
// pelo nome. Usa cache em disco; retorna "" se nao encontrado. ctx cancela a
// busca se o cliente desistir antes (ex.: handler HTTP cancelado).
func ResolveCoverURL(ctx context.Context, name string) string {
	loadCoverCache()

	key := strings.ToLower(strings.TrimSpace(name))

	coverMu.Lock()
	if v, ok := coverMap[key]; ok {
		coverMu.Unlock()
		return v
	}
	coverMu.Unlock()

	appID, done := steamSearch(ctx, name)
	if !done {
		return "" // erro de transporte: não cacheia, tentará de novo numa próxima vez
	}
	var coverURL string
	if appID > 0 {
		coverURL = fmt.Sprintf("https://cdn.steamstatic.com/steam/apps/%d/library_600x900.jpg", appID)
	}

	coverMu.Lock()
	coverMap[key] = coverURL // busca completou (achou ou não) — cacheia o resultado
	coverDirty = true
	coverMu.Unlock()

	go flushCoverCache()
	return coverURL
}

// ResolveCoverURLs resolve capas para uma lista de jogos sem cover_url em paralelo,
// com timeout global de 6 segundos. Modifica os elementos no slice passado.
//
// As goroutines NUNCA escrevem direto em games[i] — cada uma manda seu resultado
// por um channel, e só a goroutine chamadora (esta função) grava em games, só
// para os jobs que respondem antes do timeout. Isso evita que uma goroutine
// atrasada escreva no slice depois que a função já retornou (o slice pode estar
// sendo serializado para JSON nesse momento pelo handler HTTP).
func ResolveCoverURLs(games []Game) {
	type job struct {
		i    int
		name string
	}
	var jobs []job
	for i, g := range games {
		if g.CoverURL == "" {
			jobs = append(jobs, job{i, g.Nome})
		}
	}
	if len(jobs) == 0 {
		return
	}

	type result struct {
		i   int
		url string
	}
	results := make(chan result, len(jobs)) // buffered: goroutines atrasadas nao travam
	sem := make(chan struct{}, 4)           // max 4 requisicoes simultaneas
	for _, j := range jobs {
		go func(idx int, name string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- result{idx, ResolveCoverURL(context.Background(), name)}
		}(j.i, j.name)
	}

	deadline := time.After(6 * time.Second)
	for range jobs {
		select {
		case r := <-results:
			if r.url != "" {
				games[r.i].CoverURL = r.url
			}
		case <-deadline:
			return // jobs restantes terminam depois mas nao tocam mais em games
		}
	}
}
