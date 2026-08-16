//go:build windows

package winutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// isTrustedUpdateURL só aceita HTTPS em hosts do GitHub. O updater substitui o
// binário em execução e o relança COMO ADMIN — um download de host arbitrário
// seria execução remota de código. Validar antes de baixar e em cada redirect.
func isTrustedUpdateURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "github.com" || host == "api.github.com" {
		return true
	}
	return strings.HasSuffix(host, ".githubusercontent.com")
}

// UpdateInfo descreve uma versão nova disponível no GitHub.
type UpdateInfo struct {
	Available   bool   `json:"available"`
	Version     string `json:"version"`
	Notes       string `json:"notes"`
	DownloadURL string `json:"download_url"`
	// SigURL e o .sig do release. Sem ele a atualizacao nao acontece: o app se
	// substitui e relanca COMO ADMIN, entao instalar binario nao verificado
	// seria entregar a maquina do cliente a quem controlar o repositorio.
	SigURL string `json:"sig_url"`
	// Assinado diz se este update tem como ser verificado por este build.
	Assinado bool `json:"assinado"`
}

const (
	UpdateOwner = "unreallabbrasil-web"
	UpdateRepo  = "thazzdraco-releases"
)

var (
	updateMu   sync.RWMutex
	updateInfo *UpdateInfo
)

// GetUpdate retorna o resultado da última checagem (nil = ainda não checou).
func GetUpdate() *UpdateInfo {
	updateMu.RLock()
	defer updateMu.RUnlock()
	return updateInfo
}

// ForceCheck força uma checagem imediata. Em caso de erro de rede, preserva o
// cache existente e o retorna — assim o badge não desaparece por falha temporária.
// ctx cancela a requisição HTTP se o cliente (a janela do app) desistir antes.
func ForceCheck(ctx context.Context, currentVersion string) *UpdateInfo {
	info, ok := fetchUpdate(ctx, currentVersion)
	if ok {
		updateMu.Lock()
		updateInfo = info
		updateMu.Unlock()
		return info
	}
	// Erro de rede: retorna o cache sem sobrescrever
	updateMu.RLock()
	defer updateMu.RUnlock()
	return updateInfo
}

// StartUpdateChecker inicia a goroutine de checagem em background.
// Primeira checagem ocorre 5s após a inicialização; depois, a cada 4 horas.
func StartUpdateChecker(currentVersion string) {
	go func() {
		time.Sleep(5 * time.Second)
		if info, ok := fetchUpdate(context.Background(), currentVersion); ok {
			updateMu.Lock()
			updateInfo = info
			updateMu.Unlock()
		}
		ticker := time.NewTicker(4 * time.Hour)
		for range ticker.C {
			if info, ok := fetchUpdate(context.Background(), currentVersion); ok {
				updateMu.Lock()
				updateInfo = info
				updateMu.Unlock()
			}
		}
	}()
}

// fetchUpdate consulta a GitHub Releases API e compara versões.
// Retorna (nil, false) em caso de erro de rede/parse — o segundo valor indica sucesso.
// Retorna (&UpdateInfo{Available:false}, true) quando não há versão mais nova.
// Retorna (&UpdateInfo{Available:true,...}, true) quando há update disponível.
func fetchUpdate(ctx context.Context, current string) (*UpdateInfo, bool) {
	if UpdateOwner == "SEU_USUARIO_GITHUB" {
		return &UpdateInfo{Available: false}, true
	}
	url := "https://api.github.com/repos/" + UpdateOwner + "/" + UpdateRepo + "/releases/latest"
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, false // erro de rede
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
		return nil, false // erro de parse
	}

	remote := strings.TrimPrefix(release.TagName, "v")
	if !isNewer(remote, current) {
		return &UpdateInfo{Available: false}, true
	}

	downloadURL, sigURL := "", ""
	for _, a := range release.Assets {
		nome := strings.ToLower(a.Name)
		if !isTrustedUpdateURL(a.BrowserDownloadURL) {
			continue
		}
		switch {
		case strings.HasSuffix(nome, ".exe.sig"), strings.HasSuffix(nome, ".sig"):
			sigURL = a.BrowserDownloadURL
		case strings.HasSuffix(nome, ".exe"):
			if downloadURL == "" {
				downloadURL = a.BrowserDownloadURL
			}
		}
	}

	return &UpdateInfo{
		Available:   true,
		Version:     remote,
		Notes:       release.Body,
		DownloadURL: downloadURL,
		SigURL:      sigURL,
		Assinado:    sigURL != "" && VerificacaoLigada(),
	}, true
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

/* ---------- download + self-replace --------------------------------------- */

// DownloadExe baixa o exe de url para um arquivo temporário.
// progress(downloaded, total) é chamado a cada chunk; total pode ser -1.
func DownloadExe(url string, progress func(int64, int64)) (string, error) {
	if !isTrustedUpdateURL(url) {
		return "", fmt.Errorf("URL de update não confiável (esperado https em github.com): %s", url)
	}
	tmp, err := os.CreateTemp("", "ThazzDraco_update_*.exe")
	if err != nil {
		return "", fmt.Errorf("criar temp: %w", err)
	}
	tmpPath := tmp.Name()

	client := &http.Client{
		Timeout: 10 * time.Minute,
		// GitHub redireciona o browser_download_url para *.githubusercontent.com —
		// cada salto tem de continuar em host confiável, senão aborta.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("excesso de redirecionamentos")
			}
			if !isTrustedUpdateURL(req.URL.String()) {
				return fmt.Errorf("redirecionamento para host não confiável: %s", req.URL.Host)
			}
			return nil
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, we := tmp.Write(buf[:n]); we != nil {
				tmp.Close()
				os.Remove(tmpPath)
				return "", fmt.Errorf("gravar: %w", we)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("leitura: %w", rerr)
		}
	}
	tmp.Close()
	return tmpPath, nil
}

// BaixarAtualizacaoVerificada baixa o exe E a assinatura, e só devolve o
// caminho se a assinatura conferir com a chave pública deste build.
//
// É o único caminho que a UI deve usar. Recusar é o comportamento padrão:
//   - build sem chave pública embutida  -> não atualiza (avisa para baixar à mão)
//   - release sem .sig                  -> não atualiza
//   - assinatura que não confere        -> não atualiza e apaga o arquivo baixado
//
// Antes disso, o app trocava o próprio binário e relançava como administrador
// com o que quer que viesse da rede.
func BaixarAtualizacaoVerificada(info *UpdateInfo, progress func(int64, int64)) (string, error) {
	if info == nil || !info.Available || info.DownloadURL == "" {
		return "", errors.New("sem atualização disponível")
	}
	if !VerificacaoLigada() {
		return "", ErrSemChavePublica
	}
	if info.SigURL == "" {
		return "", ErrSemAssinatura
	}
	exe, err := DownloadExe(info.DownloadURL, progress)
	if err != nil {
		return "", err
	}
	sig, err := baixarTexto(info.SigURL)
	if err != nil {
		os.Remove(exe)
		return "", fmt.Errorf("baixar assinatura: %w", err)
	}
	defer os.Remove(sig)
	if err := VerificarAssinatura(exe, sig); err != nil {
		os.Remove(exe) // não deixa binário não verificado no disco
		return "", err
	}
	return exe, nil
}

// baixarTexto baixa um arquivo pequeno (a assinatura) para um temporário.
func baixarTexto(url string) (string, error) {
	if !isTrustedUpdateURL(url) {
		return "", fmt.Errorf("URL não confiável: %s", url)
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("excesso de redirecionamentos")
			}
			if !isTrustedUpdateURL(req.URL.String()) {
				return fmt.Errorf("redirecionamento para host não confiável: %s", req.URL.Host)
			}
			return nil
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// teto pequeno: a assinatura tem 128 caracteres hex
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "ThazzDraco_sig_*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(dados); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// SelfReplaceAndRestart substitui o exe em execução pelo novo e reinicia o app.
// Lança um script PowerShell detachado que faz a troca após o processo sair.
func SelfReplaceAndRestart(newExe string) error {
	selfExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("detectar exe: %w", err)
	}
	selfExe = filepath.Clean(selfExe)
	bak := selfExe + ".old"

	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	// Espera o processo atual sair, troca o exe (com rollback se falhar) e reabre.
	// Usa Move-Item (não Rename-Item) porque Rename-Item -NewName rejeita caminho
	// completo no Windows PowerShell 5.1.
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
Start-Sleep -Milliseconds 900
$self = '%s'
$new  = '%s'
$bak  = '%s'
if (Test-Path $bak) { Remove-Item $bak -Force }
$ok = $false
for ($i = 0; $i -lt 20; $i++) {
  Move-Item -Path $self -Destination $bak -Force -ErrorAction SilentlyContinue
  if (-not (Test-Path $self)) { $ok = $true; break }
  Start-Sleep -Milliseconds 300
}
if ($ok) {
  Copy-Item -Path $new -Destination $self -Force
  if (-not (Test-Path $self)) { Move-Item -Path $bak -Destination $self -Force }  # rollback
  Start-Process -FilePath $self
  Start-Sleep -Milliseconds 600
  Remove-Item $bak -Force -ErrorAction SilentlyContinue
  Remove-Item $new -Force -ErrorAction SilentlyContinue
} else {
  # Nao conseguiu trocar o exe (arquivo preso por 6s, ex.: antivirus) — reabre o
  # app antigo sem perder o update baixado silenciosamente, e loga pra diagnostico.
  $logDir = "$env:ProgramData\ThazzDraco"
  New-Item -ItemType Directory -Force -Path $logDir -ErrorAction SilentlyContinue | Out-Null
  "$(Get-Date -Format o) - falha ao trocar o exe apos 20 tentativas (6s). Update preservado em: $new" |
    Out-File -FilePath "$logDir\update-fail.log" -Append -Encoding utf8 -ErrorAction SilentlyContinue
  Start-Process -FilePath $self
}
`, esc(selfExe), esc(newExe), esc(bak))

	cmd := exec.Command("powershell", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("lançar updater: %w", err)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}
