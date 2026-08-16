//go:build windows

package sessao

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"thazzdraco/internal/winutil"
)

// Persistencia da sessao:
//
//	%ProgramData%\ThazzDraco\sessoes\<id>.json   uma sessao por arquivo
//	%ProgramData%\ThazzDraco\sessao-atual.txt    ponteiro para a sessao aberta
//
// Grava em toda transicao, com escrita atomica, pelo mesmo motivo do
// write-ahead do historico de undo: um crash no meio nao pode deixar o arquivo
// mentindo sobre o que ja foi feito.

var mu sync.Mutex

// reID trava o formato do id. Isso e defesa, nao estetica: o id chega do corpo
// JSON em /api/sessao/retomar e vira nome de arquivo — sem a trava, um
// "..\..\algo" sairia da pasta de sessoes.
var reID = regexp.MustCompile(`^ses-\d{8}-\d{6}-[0-9a-f]{4}$`)

// IDValido diz se o id tem o formato esperado.
func IDValido(id string) bool { return reID.MatchString(id) }

var ErrIDInvalido = errors.New("id de sessão inválido")

func dirSessoes() string {
	d := filepath.Join(winutil.DataDir(), "sessoes")
	os.MkdirAll(d, 0o755)
	return d
}

func caminho(id string) string { return filepath.Join(dirSessoes(), id+".json") }
func caminhoPonteiro() string  { return filepath.Join(winutil.DataDir(), "sessao-atual.txt") }

// Salvar grava a sessao em disco (atomico).
func Salvar(s *Sessao) error {
	if s == nil {
		return errors.New("sessão vazia")
	}
	if !IDValido(s.ID) {
		return ErrIDInvalido
	}
	mu.Lock()
	defer mu.Unlock()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return winutil.WriteFileAtomic(caminho(s.ID), b)
}

// Carregar le uma sessao pelo id.
func Carregar(id string) (*Sessao, error) {
	if !IDValido(id) {
		return nil, ErrIDInvalido
	}
	mu.Lock()
	defer mu.Unlock()
	return carregaLocked(id)
}

func carregaLocked(id string) (*Sessao, error) {
	b, err := os.ReadFile(caminho(id))
	if err != nil {
		return nil, err
	}
	// Tolera BOM: o arquivo e JSON legivel e um humano (ou o PowerShell, que
	// escreve UTF-8 com BOM por padrao) pode acabar salvando por cima. Um BOM
	// nao pode custar o atendimento inteiro.
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var s Sessao
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("arquivo da sessão %s ilegível: %w", id, err)
	}
	return &s, nil
}

// Atual devolve a sessao aberta, ou nil se nao houver nenhuma.
func Atual() *Sessao {
	s, _ := AtualDetalhado()
	return s
}

// AtualDetalhado devolve tambem o motivo quando o ponteiro aponta para uma
// sessao que existe mas NAO abre (JSON corrompido, disco com problema, alguem
// editou o arquivo). "Nao ha sessao" e "a sessao esta la mas nao abre" sao
// coisas diferentes: tratar a segunda como a primeira faz o atendimento sumir
// da tela sem uma palavra — exatamente o tipo de mentira silenciosa que o
// produto nao aceita.
func AtualDetalhado() (*Sessao, error) {
	mu.Lock()
	defer mu.Unlock()
	b, err := os.ReadFile(caminhoPonteiro())
	if err != nil {
		return nil, nil // sem ponteiro = sem sessao aberta, e normal
	}
	id := strings.TrimSpace(string(b))
	if !IDValido(id) {
		return nil, fmt.Errorf("o ponteiro da sessão atual está inválido (%q)", id)
	}
	s, err := carregaLocked(id)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("a sessão %s não está mais na pasta de sessões", id)
		}
		return nil, err
	}
	return s, nil
}

// DefinirAtual aponta o ponteiro para esta sessao.
func DefinirAtual(id string) error {
	if !IDValido(id) {
		return ErrIDInvalido
	}
	mu.Lock()
	defer mu.Unlock()
	return winutil.WriteFileAtomic(caminhoPonteiro(), []byte(id))
}

// LimparAtual solta o ponteiro (sessao encerrada). O arquivo da sessao fica —
// e o historico do atendimento.
func LimparAtual() error {
	mu.Lock()
	defer mu.Unlock()
	err := os.Remove(caminhoPonteiro())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// txMu serializa leitura-modificacao-escrita da sessao atual. O executor roda
// numa goroutine e os handlers HTTP continuam atendendo (o poll de status, por
// exemplo) — sem esta transacao, os dois carregariam do disco, mexeriam em
// copias diferentes e a ultima escrita apagaria a outra.
var txMu sync.Mutex

// Transacao carrega a sessao atual, deixa fn mexer e grava o resultado. Se nao
// ha sessao aberta, devolve ErrSemSessao. Se fn devolve erro, nada e gravado.
func Transacao(fn func(*Sessao) error) (*Sessao, error) {
	txMu.Lock()
	defer txMu.Unlock()
	s := Atual()
	if s == nil {
		return nil, ErrSemSessao
	}
	if err := fn(s); err != nil {
		return s, err
	}
	if err := Salvar(s); err != nil {
		return s, err
	}
	return s, nil
}

var ErrSemSessao = errors.New("nenhuma sessão aberta")

// Listar devolve as sessoes gravadas, da mais nova para a mais velha. Arquivo
// ilegivel e ignorado em silencio: uma sessao corrompida nao pode derrubar a
// listagem inteira.
func Listar(max int) []Resumo {
	mu.Lock()
	defer mu.Unlock()
	ents, err := os.ReadDir(dirSessoes())
	if err != nil {
		return nil
	}
	var out []Resumo
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if !IDValido(id) {
			continue
		}
		s, err := carregaLocked(id)
		if err != nil {
			continue
		}
		out = append(out, s.resumo())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID }) // id comeca com a data
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}
