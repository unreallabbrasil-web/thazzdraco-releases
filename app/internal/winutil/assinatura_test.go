//go:build windows

package winutil

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// O updater troca o binário em execução e relança COMO ADMINISTRADOR. Se a
// verificação falhar em qualquer um destes casos, quem controlar o repositório
// de releases controla a máquina de todo cliente.

func criaExeFalso(t *testing.T, dir, nome, conteudo string) string {
	t.Helper()
	p := filepath.Join(dir, nome)
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAssinarEVerificarRoundtrip(t *testing.T) {
	dir := t.TempDir()
	chave := filepath.Join(dir, "priv.key")
	pubHex, err := GerarParDeChaves(chave)
	if err != nil {
		t.Fatal(err)
	}
	exe := criaExeFalso(t, dir, "app.exe", "binario de verdade")
	sig, err := AssinarArquivo(exe, chave)
	if err != nil {
		t.Fatal(err)
	}

	pub, _ := hex.DecodeString(pubHex)
	digest, _ := DigestArquivo(exe)
	raw, _ := os.ReadFile(sig)
	assinatura, _ := hex.DecodeString(string(raw[:128]))
	if !ed25519.Verify(pub, digest, assinatura) {
		t.Fatal("a assinatura gerada não valida com a chave pública do mesmo par")
	}
}

// O cenário que o mecanismo existe para impedir: o repositório é comprometido e
// o atacante troca o .exe. Ele não tem a chave privada, então a assinatura
// publicada não bate com o binário dele.
func TestBinarioTrocadoNaoPassa(t *testing.T) {
	dir := t.TempDir()
	chave := filepath.Join(dir, "priv.key")
	if _, err := GerarParDeChaves(chave); err != nil {
		t.Fatal(err)
	}
	exe := criaExeFalso(t, dir, "app.exe", "binario legitimo")
	sig, _ := AssinarArquivo(exe, chave)

	// atacante substitui o exe mantendo a assinatura antiga
	if err := os.WriteFile(exe, []byte("binario malicioso"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verificarCom(t, chave, exe, sig); err == nil {
		t.Fatal("BINÁRIO TROCADO PASSOU NA VERIFICAÇÃO")
	}
}

// O atacante também pode assinar com a chave DELE. Não adianta: a pública
// embutida no app é outra.
func TestAssinaturaDeOutraChaveNaoPassa(t *testing.T) {
	dir := t.TempDir()
	nossa := filepath.Join(dir, "nossa.key")
	deles := filepath.Join(dir, "deles.key")
	if _, err := GerarParDeChaves(nossa); err != nil {
		t.Fatal(err)
	}
	if _, err := GerarParDeChaves(deles); err != nil {
		t.Fatal(err)
	}
	exe := criaExeFalso(t, dir, "app.exe", "binario malicioso")
	sig, _ := AssinarArquivo(exe, deles) // assinado pela chave errada

	if err := verificarCom(t, nossa, exe, sig); err == nil {
		t.Fatal("ASSINATURA DE OUTRA CHAVE PASSOU")
	}
}

func TestSemAssinaturaNaoPassa(t *testing.T) {
	dir := t.TempDir()
	exe := criaExeFalso(t, dir, "app.exe", "binario")
	if err := VerificarAssinatura(exe, filepath.Join(dir, "nao-existe.sig")); err != ErrSemAssinatura {
		t.Errorf("sem .sig: erro = %v, queria %v", err, ErrSemAssinatura)
	}
	// arquivo de assinatura existente mas com lixo dentro
	lixo := criaExeFalso(t, dir, "app.exe.sig", "isto nao e uma assinatura")
	if err := VerificarAssinatura(exe, lixo); err != ErrAssinaturaInvalida {
		t.Errorf("assinatura corrompida: erro = %v, queria %v", err, ErrAssinaturaInvalida)
	}
}

func TestNaoSobrescreveChavePrivadaExistente(t *testing.T) {
	dir := t.TempDir()
	chave := filepath.Join(dir, "priv.key")
	if _, err := GerarParDeChaves(chave); err != nil {
		t.Fatal(err)
	}
	antes, _ := os.ReadFile(chave)
	if _, err := GerarParDeChaves(chave); err == nil {
		t.Fatal("gerar por cima de uma chave existente invalidaria todos os updates já publicados")
	}
	depois, _ := os.ReadFile(chave)
	if string(antes) != string(depois) {
		t.Fatal("a chave existente foi alterada")
	}
}

// Este build precisa ter chave pública embutida — sem ela o app não atualiza.
func TestBuildTemChavePublicaEmbutida(t *testing.T) {
	if !VerificacaoLigada() {
		t.Fatal("ChavePublicaAtualizacao vazia ou inválida: este build não consegue verificar update nenhum")
	}
}

// verificarCom valida usando a pública derivada de uma privada de teste, já que
// VerificarAssinatura usa a chave embutida no binário.
func verificarCom(t *testing.T, arquivoPrivada, exe, sig string) error {
	t.Helper()
	raw, err := os.ReadFile(arquivoPrivada)
	if err != nil {
		return err
	}
	priv, _ := hex.DecodeString(trimN(string(raw)))
	pub := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)

	digest, err := DigestArquivo(exe)
	if err != nil {
		return err
	}
	sraw, err := os.ReadFile(sig)
	if err != nil {
		return ErrSemAssinatura
	}
	assinatura, err := hex.DecodeString(trimN(string(sraw)))
	if err != nil || len(assinatura) != ed25519.SignatureSize {
		return ErrAssinaturaInvalida
	}
	if !ed25519.Verify(pub, digest, assinatura) {
		return ErrAssinaturaInvalida
	}
	return nil
}

func trimN(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
