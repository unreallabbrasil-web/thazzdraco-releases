// Package sessao orquestra o atendimento completo de um PC: o trilho de etapas
// que da ordem ao que hoje sao telas soltas (medir, ler, entender, planejar,
// aplicar, provar, entregar).
//
// A sessao fica POR CIMA do motor: importa engine/winutil, nunca o contrario. O
// engine segue sendo motor puro (detect/apply/undo) e dono da rede de seguranca
// (snapshot + historico de undo); a sessao so decide a ORDEM e guarda o ESTADO.
//
// Esta e a fatia S1 de docs/SESSAO.md: modelo + persistencia + maquina de
// etapas. O plano (S3), as evidencias de antes/depois (S2) e o executor por fase
// (S5) entram nas fatias seguintes — por isso ainda nao existem aqui.
package sessao

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// SchemaVer versiona o arquivo da sessao. Contrato versionado e validado, para
// que divergencia falhe visivel e nao em silencio (docs/CONVENTIONS.md).
const SchemaVer = "1.0"

// Status de uma etapa.
const (
	StatusBloqueada   = "bloqueada"
	StatusDisponivel  = "disponivel"
	StatusEmAndamento = "em-andamento"
	StatusConcluida   = "concluida"
	StatusPulada      = "pulada"
)

// MotivoSemAdmin explica a etapa bloqueada no modo Limitada. Fica aqui (e nao na
// UI) porque vai junto para o relatorio.
const MotivoSemAdmin = "precisa de administrador — o app foi aberto sem elevação"

// EtapaDef descreve uma etapa do trilho. E catalogo fixo, mandado junto em toda
// resposta da API para a UI ter uma fonte unica de nome/resumo.
type EtapaDef struct {
	ID         string `json:"id"`
	Nome       string `json:"nome"`
	Resumo     string `json:"resumo"`
	ExigeAdmin bool   `json:"exige_admin"`
}

// Etapas e o trilho: 7 etapas visiveis. "Preparar" e a etapa 0 e nao aparece
// aqui de proposito — e um portao automatico, rodado em Nova(), que so define se
// a sessao nasce completa ou Limitada.
var Etapas = []EtapaDef{
	{ID: "medir", Nome: "Medir", Resumo: "Mede o PC como ele está agora. Sem essa foto, o ganho no fim vira opinião."},
	{ID: "ler", Nome: "Ler", Resumo: "Varredura completa: hardware, regras, inicialização e jogos. Nada é alterado."},
	{ID: "entender", Nome: "Entender", Resumo: "Cruza o que foi lido e aponta o que segura o FPS desta máquina."},
	// Planejar NAO exige admin: montar o plano e leitura + decisao, nao escreve
	// nada. Quem exige elevacao e Aplicar. Sem isso, um PC sem admin ficava sem
	// saber nem o que precisaria ser feito.
	{ID: "planejar", Nome: "Planejar", Resumo: "Monta a fila do que vai ser feito, na ordem certa. Você aprova antes de qualquer escrita."},
	{ID: "aplicar", Nome: "Aplicar", Resumo: "Executa o que foi aprovado, com ponto de restauração e undo exato.", ExigeAdmin: true},
	{ID: "provar", Nome: "Provar", Resumo: "Mede de novo, no mesmo cenário, e compara com o começo."},
	{ID: "entregar", Nome: "Entregar", Resumo: "Fecha a sessão e gera o relatório do que foi feito e do que mudou."},
}

// Intencoes aceitas (semeiam o plano na S3; aqui so ficam registradas).
var Intencoes = []string{"equilibrado", "competitivo", "streaming", "livre"}

type Cliente struct {
	Nome string `json:"nome"`
}

// EtapaState e o estado de uma etapa nesta sessao.
type EtapaState struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Quando string `json:"quando"`
	Motivo string `json:"motivo"` // obrigatorio quando pulada; explica quando bloqueada
}

// Sessao e um atendimento completo em um PC.
type Sessao struct {
	ID         string         `json:"id"`
	SchemaVer  string         `json:"schema_ver"`
	Criada     string         `json:"criada"`
	Atualizada string         `json:"atualizada"`
	PC         string         `json:"pc"`
	Cliente    Cliente        `json:"cliente"`
	Intencao   string         `json:"intencao"`
	Etapa      string         `json:"etapa"` // ate onde a sessao chegou (a etapa corrente)
	Etapas     []EtapaState   `json:"etapas"`
	Limitada   bool           `json:"limitada"` // etapa 0 reprovou: sem admin, so leitura
	Perfil     map[string]any `json:"perfil,omitempty"`
	Baseline   *Evidencia     `json:"baseline,omitempty"` // medido antes de mexer
	Depois     *Evidencia     `json:"depois,omitempty"`   // medido depois, para provar
	Plano      *Plano         `json:"plano,omitempty"`
	Execucao   *Execucao      `json:"execucao,omitempty"`
	Reboot     *RebootState   `json:"reboot,omitempty"`
	Encerrada  bool           `json:"encerrada"`
}

// Resumo e a visao curta usada na listagem de sessoes.
type Resumo struct {
	ID        string  `json:"id"`
	Criada    string  `json:"criada"`
	PC        string  `json:"pc"`
	Cliente   Cliente `json:"cliente"`
	Intencao  string  `json:"intencao"`
	Etapa     string  `json:"etapa"`
	Encerrada bool    `json:"encerrada"`
}

func (s *Sessao) resumo() Resumo {
	return Resumo{ID: s.ID, Criada: s.Criada, PC: s.PC, Cliente: s.Cliente,
		Intencao: s.Intencao, Etapa: s.Etapa, Encerrada: s.Encerrada}
}

func agora() string { return time.Now().Format("2006-01-02 15:04:05") }

// novoID monta "ses-<data>-<hora>-<4 hex>". O sufixo aleatorio existe para duas
// sessoes criadas no mesmo segundo nao gravarem por cima uma da outra.
func novoID() string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("ses-%s-%04x", time.Now().Format("20060102-150405"), time.Now().UnixNano()&0xffff)
	}
	return "ses-" + time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// DefEtapa devolve a definicao de uma etapa do catalogo.
func DefEtapa(id string) (EtapaDef, bool) {
	for _, d := range Etapas {
		if d.ID == id {
			return d, true
		}
	}
	return EtapaDef{}, false
}

// limpaTexto normaliza texto vindo do usuario: tira espaco nas pontas, mata
// caractere de controle (inclusive quebra de linha, que estragaria o relatorio)
// e corta no limite.
func limpaTexto(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max]))
	}
	return s
}

// NormalizaIntencao valida a intencao vinda de fora; desconhecida vira o padrao.
func NormalizaIntencao(v string) string { return normalizaIntencao(v) }

func normalizaIntencao(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, i := range Intencoes {
		if i == v {
			return v
		}
	}
	return "equilibrado"
}
