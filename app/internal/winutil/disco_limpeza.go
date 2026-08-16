//go:build windows

package winutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Exclusao de arquivos (docs/DISCO-LIMPEZA.md §3).
//
// Este e o unico codigo do app que APAGA, e apagar nao tem undo. Todo o resto do
// produto tem rede de seguranca — snapshot, ponto de restauracao, historico de
// lotes. Aqui nao ha. Entao a seguranca precisa estar ANTES: no que o codigo
// aceita fazer, nao no que ele consegue desfazer.
//
// Tres camadas, cada uma suficiente sozinha para barrar um erro:
//
//  1. O cliente manda ID DE CATEGORIA, nunca caminho. Nao existe endpoint que
//     apague um caminho arbitrario.
//  2. So categoria da classe "limpavel" e executavel. Pedir para limpar o
//     `windows-installer` (que esta no catalogo como intocavel) e recusado.
//  3. podeApagar() confere cada caminho na porta, mesmo vindo do catalogo. Se um
//     dia alguem adicionar uma categoria errada, ela nao passa daqui.

var (
	ErrPrecisaConfirmarLimpeza = errors.New("a limpeza precisa de confirmação explícita")
	ErrCategoriaNaoLimpavel    = errors.New("esta categoria não é limpável — o app não apaga isso")
	ErrCategoriaDesconhecida   = errors.New("categoria desconhecida")
)

// negados sao trechos de caminho que NUNCA podem ser apagados, mesmo que uma
// categoria futura aponte para eles por engano. Cada linha tem um motivo real:
// sao os lugares onde "liberar espaco" quebra a maquina ou apaga o cliente.
var negados = []string{
	`\windows\installer`,                                                     // sem ele, desinstalar/reparar programas quebra
	`\windows\system32`,                                                      // sistema
	`\windows\syswow64`,                                                      // sistema
	`\windows\winsxs`,                                                        // componentes; só via DISM
	`\windows\servicing`,                                                     // motor de manutenção do Windows
	`\program files`,                                                         // programas instalados
	`\programdata\package cache`,                                             // instaladores usados para reparo
	`\system volume information`,                                             // pontos de restauração: a rede de segurança do app
	`\$recycle.bin`,                                                          // Lixeira tem caminho próprio (Limpeza profunda)
	`\boot`,                                                                  // inicialização
	`\recovery`,                                                              // ambiente de recuperação
	`\desktop`, `\documents`, `\downloads`, `\pictures`, `\videos`, `\music`, // pessoal
	`\onedrive`, `\dropbox`, `\google drive`, // nuvem sincronizada = dado pessoal
	`\appdata\roaming\microsoft\windows\start menu`, // atalhos do usuário
	`\saved games`, `\savegames`, // saves de jogo
}

// arquivosNegados nunca sao apagados, em nenhuma circunstancia.
var arquivosNegados = []string{"pagefile.sys", "hiberfil.sys", "swapfile.sys", "ntuser.dat", "bootmgr"}

// podeApagar decide se um caminho pode ter o conteudo apagado. Recusa por
// padrao: qualquer duvida vira erro, nao permissao.
func podeApagar(caminho string) error {
	c := strings.TrimSpace(caminho)
	if c == "" {
		return errors.New("caminho vazio")
	}
	if !filepath.IsAbs(c) {
		return fmt.Errorf("caminho relativo não é aceito: %q", c)
	}
	limpo := filepath.Clean(c)
	if strings.Contains(limpo, "..") {
		return fmt.Errorf("caminho com salto de pasta: %q", c)
	}
	vol := filepath.VolumeName(limpo)
	if vol == "" {
		return fmt.Errorf("caminho sem unidade: %q", c)
	}
	// Raiz de unidade jamais. "C:\" tem que ser impossível de passar por aqui.
	if strings.TrimRight(limpo, `\`) == strings.TrimRight(vol, `\`) {
		return fmt.Errorf("a raiz da unidade nunca é apagável: %q", c)
	}
	// Profundidade mínima: no mínimo dois níveis abaixo da raiz. Isso sozinho já
	// barra "C:\Windows" e "C:\Users".
	partes := strings.Split(strings.Trim(strings.TrimPrefix(limpo, vol), `\`), `\`)
	if len(partes) < 2 {
		return fmt.Errorf("caminho raso demais para ser apagado com segurança: %q", c)
	}
	low := strings.ToLower(limpo)
	for _, n := range negados {
		if strings.Contains(low+`\`, n+`\`) || strings.HasSuffix(low, n) {
			return fmt.Errorf("caminho protegido (%s): %q", strings.Trim(n, `\`), c)
		}
	}
	st, err := os.Lstat(limpo)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("só o conteúdo de pastas é apagado, não arquivos soltos: %q", c)
	}
	if ehReparsePoint(st) {
		return fmt.Errorf("é uma junção/link, não a pasta de verdade: %q", c)
	}
	return nil
}

// LimpezaCategoria e o resultado por categoria.
type LimpezaCategoria struct {
	ID       string `json:"id"`
	Nome     string `json:"nome"`
	Bytes    int64  `json:"bytes"`
	Arquivos int64  `json:"arquivos"`
	Travados int64  `json:"travados"` // em uso: não deu para apagar
	Erro     string `json:"erro,omitempty"`
}

// ResultadoLimpeza e o que a UI mostra depois.
type ResultadoLimpeza struct {
	Quando       string             `json:"quando"`
	Liberado     int64              `json:"liberado"`
	Categorias   []LimpezaCategoria `json:"categorias"`
	Recusadas    []string           `json:"recusadas,omitempty"`
	TotalTravado int64              `json:"total_travado"`
}

// Limpar apaga o conteudo das categorias pedidas. So aceita ID de categoria
// LIMPAVEL — qualquer outra coisa e recusada com o motivo.
func Limpar(sid string, ids []string, confirmar bool) (*ResultadoLimpeza, error) {
	if !confirmar {
		return nil, ErrPrecisaConfirmarLimpeza
	}
	if len(ids) == 0 {
		return nil, errors.New("nenhuma categoria escolhida")
	}
	porID := map[string]catAchado{}
	for _, c := range montaCatalogo(sid) {
		porID[c.id] = c
	}

	res := &ResultadoLimpeza{Quando: time.Now().Format("2006-01-02 15:04:05")}
	LogOperacao(fmt.Sprintf("=== %s  LIMPEZA DE DISCO  categorias=%s ===",
		res.Quando, strings.Join(ids, ",")))

	for _, id := range ids {
		c, ok := porID[id]
		if !ok {
			res.Recusadas = append(res.Recusadas, id+": "+ErrCategoriaDesconhecida.Error())
			continue
		}
		if c.classe != ClasseLimpavel {
			// A trava que importa: mesmo que a UI mande, o backend recusa.
			res.Recusadas = append(res.Recusadas, c.nome+": "+ErrCategoriaNaoLimpavel.Error())
			LogOperacao("    RECUSADO  " + c.id + " (classe " + c.classe + ")")
			continue
		}

		lc := LimpezaCategoria{ID: c.id, Nome: c.nome}
		for _, p := range c.pastas {
			if p == "" {
				continue
			}
			if err := podeApagar(p); err != nil {
				if !os.IsNotExist(err) {
					res.Recusadas = append(res.Recusadas, err.Error())
					LogOperacao("    RECUSADO  " + err.Error())
				}
				continue
			}
			b, n, travados := apagaConteudo(p)
			lc.Bytes += b
			lc.Arquivos += n
			lc.Travados += travados
			LogOperacao(fmt.Sprintf("    %s  %.1f MB em %d arquivos%s",
				p, float64(b)/(1<<20), n, seTravados(travados)))
		}
		res.Liberado += lc.Bytes
		res.TotalTravado += lc.Travados
		res.Categorias = append(res.Categorias, lc)
	}
	LogOperacao(fmt.Sprintf("    TOTAL LIBERADO: %.1f MB", float64(res.Liberado)/(1<<20)))
	return res, nil
}

func seTravados(n int64) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d em uso, mantidos)", n)
}

// apagaConteudo apaga os ARQUIVOS de dentro de uma pasta, recursivamente, e
// remove as subpastas que ficaram vazias. A pasta de cima NAO e removida: os
// programas esperam que ela exista (o npm quebra se o npm-cache sumir inteiro).
//
// Nunca entra em junction/symlink: seguir um levaria a exclusao para fora da
// pasta autorizada, que e exatamente o pior acidente possivel aqui.
func apagaConteudo(dir string) (bytes int64, arquivos int64, travados int64) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0
	}
	for _, e := range ents {
		alvo := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if ehReparsePoint(info) {
			continue // não segue e não apaga o link
		}
		if e.IsDir() {
			b, n, t := apagaConteudo(alvo)
			bytes += b
			arquivos += n
			travados += t
			os.Remove(alvo) // só sai se ficou vazia
			continue
		}
		if ehArquivoNegado(e.Name()) {
			continue
		}
		tam := info.Size()
		if err := os.Remove(alvo); err != nil {
			travados++
			continue
		}
		bytes += tam
		arquivos++
	}
	return bytes, arquivos, travados
}

func ehArquivoNegado(nome string) bool {
	n := strings.ToLower(nome)
	for _, x := range arquivosNegados {
		if n == x {
			return true
		}
	}
	return false
}
