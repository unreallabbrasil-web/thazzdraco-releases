//go:build windows

package winutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Assinatura das atualizacoes.
//
// O PROBLEMA: o app baixa um .exe do GitHub, se substitui e relanca COMO
// ADMINISTRADOR. Sem verificacao, quem controlar aquele repositorio de releases
// controla execucao de codigo como admin na maquina de todo cliente. E o achado
// mais grave do checkup (RCE-como-admin).
//
// POR QUE HASH NAO RESOLVE: publicar o SHA-256 junto do release protege contra
// corrupcao no caminho — que o HTTPS ja cobre. Nao protege contra o cenario que
// importa: se o repositorio for comprometido, o atacante publica o exe dele E o
// hash dele. Hash ao lado do arquivo so prova que o arquivo e o que quem
// publicou quis publicar.
//
// A SOLUCAO: assinatura Ed25519 com a chave publica EMBUTIDA no binario. A
// chave privada fica offline, fora do repositorio. Repositorio comprometido ->
// o atacante nao consegue forjar a assinatura -> o app recusa a atualizacao.
// Custa zero e nao depende de comprar certificado.
//
// O QUE E ASSINADO: o SHA-256 do arquivo, nao o arquivo inteiro — o digest e
// calculado em streaming, entao um exe de 9 MB nao vai inteiro para a memoria.

// ChavePublicaAtualizacao e a chave que valida os updates (hex de 32 bytes).
// Trocar esta chave invalida todas as assinaturas antigas de propósito: e o
// caminho de revogacao se a privada vazar.
const ChavePublicaAtualizacao = "0ab067e2a07df92dd8298bb178c93bd2c072b171ffd762e2bdf7d2e7f4d9d05f"

var (
	ErrSemAssinatura      = errors.New("a atualização não veio assinada")
	ErrAssinaturaInvalida = errors.New("a assinatura da atualização não confere — o arquivo não foi publicado por quem tem a chave")
	ErrSemChavePublica    = errors.New("este build não tem chave pública de atualização embutida")
)

// VerificacaoLigada diz se este build sabe verificar assinatura.
func VerificacaoLigada() bool { return len(chavePublica()) == ed25519.PublicKeySize }

func chavePublica() ed25519.PublicKey {
	b, err := hex.DecodeString(strings.TrimSpace(ChavePublicaAtualizacao))
	if err != nil {
		return nil
	}
	return ed25519.PublicKey(b)
}

// DigestArquivo devolve o SHA-256 do arquivo, lido em streaming.
func DigestArquivo(caminho string) ([]byte, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// VerificarAssinatura confere que o arquivo foi assinado pela chave privada
// correspondente a ChavePublicaAtualizacao. Recusa por padrao: sem chave, sem
// assinatura ou assinatura errada, o update NAO acontece.
func VerificarAssinatura(caminhoArquivo, caminhoSig string) error {
	pub := chavePublica()
	if len(pub) != ed25519.PublicKeySize {
		return ErrSemChavePublica
	}
	sigHex, err := os.ReadFile(caminhoSig)
	if err != nil {
		return ErrSemAssinatura
	}
	sig, err := hex.DecodeString(strings.TrimSpace(string(sigHex)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrAssinaturaInvalida
	}
	digest, err := DigestArquivo(caminhoArquivo)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, digest, sig) {
		return ErrAssinaturaInvalida
	}
	return nil
}

// ---- Ferramentas de publicacao (rodam na maquina de quem publica) -----------

// GerarParDeChaves cria o par e grava a privada em `arquivoPrivada`. Devolve a
// publica em hex, para ser embutida no proximo build.
//
// A privada NUNCA pode entrar no repositorio: quem a tiver assina updates que
// todos os clientes vao instalar como administrador.
func GerarParDeChaves(arquivoPrivada string) (publicaHex string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(arquivoPrivada); err == nil {
		return "", fmt.Errorf("já existe uma chave em %s — apagar isso invalida a publicação de updates", arquivoPrivada)
	}
	if err := os.WriteFile(arquivoPrivada, []byte(hex.EncodeToString(priv)+"\n"), 0o600); err != nil {
		return "", err
	}
	return hex.EncodeToString(pub), nil
}

// AssinarArquivo assina o SHA-256 do arquivo com a chave privada e grava
// `caminho + ".sig"`. E o passo que roda depois do build, antes de publicar.
func AssinarArquivo(caminho, arquivoPrivada string) (string, error) {
	raw, err := os.ReadFile(arquivoPrivada)
	if err != nil {
		return "", fmt.Errorf("ler chave privada: %w", err)
	}
	priv, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		return "", errors.New("chave privada inválida")
	}
	digest, err := DigestArquivo(caminho)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), digest)
	destino := caminho + ".sig"
	if err := os.WriteFile(destino, []byte(hex.EncodeToString(sig)+"\n"), 0o644); err != nil {
		return "", err
	}
	return destino, nil
}
