//go:build windows

package winutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sort"
)

// Duplicados grandes (docs/DISCO-LIMPEZA.md §5).
//
// Dois arquivos de 8,73 GB com o mesmo tamanho quase certamente sao a mesma
// coisa — mas "quase certamente" nao serve para apontar o dedo e dizer "apaga
// esse". Por isso a confirmacao tem dois passos e o app NUNCA apaga:
//
//  1. tamanho exato igual (filtro barato, feito durante a varredura);
//  2. hash do INICIO e do FIM do arquivo (2 MB lidos por arquivo).
//
// Ler os 8,73 GB inteiros de cada um para ter prova matematica custaria minutos
// e travaria a tela. As pontas pegam qualquer diferenca real de conteudo — o que
// escaparia seria um arquivo forjado para bater nas pontas e diferir no meio, e
// isso nao acontece com VM, jogo ou video. A tela DIZ como foi conferido, para
// ninguem apagar achando que houve comparacao byte a byte.

const amostraDup = 1 << 20 // 1 MB de cada ponta

// GrupoDuplicado sao arquivos confirmados como iguais.
type GrupoDuplicado struct {
	Bytes       int64    `json:"bytes"`       // tamanho de cada cópia
	Caminhos    []string `json:"caminhos"`    // todas as cópias encontradas
	Desperdicio int64    `json:"desperdicio"` // o que sobraria mantendo uma só
	Conferido   string   `json:"conferido"`   // como a igualdade foi verificada
}

// Duplicados confirma e devolve os grupos de arquivos grandes iguais, do maior
// desperdicio para o menor.
func (g *gerenciadorDisco) Duplicados() []GrupoDuplicado {
	g.mu.Lock()
	res := g.resultado
	g.mu.Unlock()
	if res == nil || len(res.dups) == 0 {
		return nil
	}

	var out []GrupoDuplicado
	for tamanho, caminhos := range res.dups {
		if len(caminhos) < 2 {
			continue
		}
		// agrupa por hash das pontas: mesmo tamanho pode ser coincidência
		porHash := map[string][]string{}
		for _, c := range caminhos {
			h, err := hashPontas(c, tamanho)
			if err != nil {
				continue // sumiu ou está travado: some da lista em vez de mentir
			}
			porHash[h] = append(porHash[h], c)
		}
		for _, iguais := range porHash {
			if len(iguais) < 2 {
				continue
			}
			sort.Strings(iguais)
			out = append(out, GrupoDuplicado{
				Bytes:       tamanho,
				Caminhos:    iguais,
				Desperdicio: tamanho * int64(len(iguais)-1),
				Conferido:   "mesmo tamanho e mesmo conteúdo no primeiro e no último MB",
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Desperdicio > out[j].Desperdicio })
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

// hashPontas devolve o hash do inicio + fim do arquivo. Le no maximo 2 MB,
// independente do tamanho do arquivo.
func hashPontas(caminho string, tamanho int64) (string, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.CopyN(h, f, amostraDup); err != nil && err != io.EOF {
		return "", err
	}
	if tamanho > 2*amostraDup {
		if _, err := f.Seek(-amostraDup, io.SeekEnd); err != nil {
			return "", err
		}
		if _, err := io.CopyN(h, f, amostraDup); err != nil && err != io.EOF {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
