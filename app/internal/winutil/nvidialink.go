//go:build windows

package winutil

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// A API antiga da NVIDIA de busca automática de driver (AjaxDriverService)
// foi descontinuada no backend deles (confirmado por teste: retorna "not
// found" pra qualquer placa, até modelos antigos e estáveis). O catálogo de
// produtos (psid/pfid) ainda funciona, então usamos ele só pra montar um link
// direto e correto pra página oficial de driver da placa exata do usuário —
// sem inventar número de versão, sem prometer o que não dá pra verificar.

type nvLookupValue struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}
type nvLookupSearch struct {
	Values []nvLookupValue `xml:"LookupValues>LookupValue"`
}

var (
	nvCacheMu    sync.Mutex
	nvPsidCache  []nvLookupValue
	nvPsidExpiry time.Time
)

// nvLookupFetch busca uma lista TypeID da API de catálogo da NVIDIA (ainda
// ativa) e cacheia por 24h — é só metadado de catálogo, muda raramente.
func nvLookupFetch(typeID int) ([]nvLookupValue, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	u := fmt.Sprintf("https://www.nvidia.com/Download/API/lookupValueSearch.aspx?TypeID=%d", typeID)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed nvLookupSearch
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Values, nil
}

func nvPsidList() ([]nvLookupValue, error) {
	nvCacheMu.Lock()
	if nvPsidCache != nil && time.Now().Before(nvPsidExpiry) {
		defer nvCacheMu.Unlock()
		return nvPsidCache, nil
	}
	nvCacheMu.Unlock()

	vals, err := nvLookupFetch(3)
	if err != nil {
		return nil, err
	}
	nvCacheMu.Lock()
	nvPsidCache = vals
	nvPsidExpiry = time.Now().Add(24 * time.Hour)
	nvCacheMu.Unlock()
	return vals, nil
}

// NvidiaDriverPageURL resolve o psid da placa pelo nome (ex.: "NVIDIA GeForce
// RTX 5070") e monta a URL da página oficial de driver já filtrada pra ela.
// Retorna erro se não achar correspondência exata o bastante pra confiar.
func NvidiaDriverPageURL(gpuName string) (string, error) {
	psids, err := nvPsidList()
	if err != nil {
		return "", fmt.Errorf("catálogo da NVIDIA indisponível agora: %w", err)
	}

	clean := strings.TrimSpace(gpuName)
	cleanLower := strings.ToLower(clean)
	// nomes de registro costumam vir sem o prefixo "NVIDIA "
	if !strings.HasPrefix(cleanLower, "nvidia") {
		cleanLower = "nvidia " + cleanLower
	}

	var bestPsid string
	bestLen := -1
	for _, v := range psids {
		nameLower := strings.ToLower(v.Name)
		if nameLower == cleanLower || strings.TrimPrefix(nameLower, "nvidia ") == strings.TrimPrefix(cleanLower, "nvidia ") {
			bestPsid = v.Value
			bestLen = len(nameLower)
			break // match exato — para na hora
		}
		if strings.Contains(nameLower, strings.TrimPrefix(cleanLower, "nvidia ")) && len(nameLower) > bestLen {
			bestPsid = v.Value
			bestLen = len(nameLower)
		}
	}
	if bestPsid == "" {
		return "", fmt.Errorf("não achei %q no catálogo da NVIDIA", gpuName)
	}

	pfid := "1" // GeForce (padrão)
	if strings.Contains(cleanLower, "quadro") || strings.Contains(cleanLower, "rtx pro") {
		pfid = "3"
	} else if strings.Contains(cleanLower, "titan") {
		pfid = "11"
	}

	osid := windowsOsID()

	q := url.Values{}
	q.Set("psid", bestPsid)
	q.Set("pfid", pfid)
	q.Set("osid", osid)
	q.Set("lid", "1")  // inglês — evita 404 em locales sem tradução
	q.Set("whql", "1")
	return "https://www.nvidia.com/en-us/drivers/results/?" + q.Encode(), nil
}

// windowsOsID mapeia a versão do Windows rodando pro osID que o catálogo da
// NVIDIA usa (Windows 11 = 135, Windows 10 64-bit = 57 — valores extraídos
// da própria API de lookup deles).
func windowsOsID() string {
	v := windows.RtlGetVersion()
	if v.BuildNumber >= 22000 {
		return "135" // Windows 11
	}
	return "57" // Windows 10 64-bit (o app só builda amd64)
}
