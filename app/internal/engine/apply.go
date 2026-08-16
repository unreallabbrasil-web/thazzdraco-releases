package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"thazzdraco/internal/winutil"
)

// ---- Modelo de undo (persistido em historico.json) --------------------------

type ServiceSnap struct {
	Name         string `json:"name"`
	Existed      bool   `json:"existed"`
	OldStartType uint32 `json:"old_start_type"`
}

type PowercfgUndo struct {
	Kind     string `json:"kind"` // "setactive" | "setacvalue"
	Scheme   string `json:"scheme,omitempty"`
	Sub      string `json:"sub,omitempty"`
	Setting  string `json:"setting,omitempty"`
	OldValue int64  `json:"old_value,omitempty"`
	HadValue bool   `json:"had_value,omitempty"`
}

type RuleUndo struct {
	RuleID   string                `json:"rule_id"`
	Titulo   string                `json:"titulo"`
	Regs     []winutil.RegSnapshot `json:"regs,omitempty"`
	Servicos []ServiceSnap         `json:"servicos,omitempty"`
	Powercfg []PowercfgUndo        `json:"powercfg,omitempty"`
}

type BatchRecord struct {
	ID     string     `json:"id"`
	Quando string     `json:"quando"`
	Origem string     `json:"origem"` // "selecao" | "verdes" | "preset:xyz"
	Rules  []RuleUndo `json:"rules"`
}

type ApplyReport struct {
	BatchID      string            `json:"batch_id"`
	Aplicadas    []string          `json:"aplicadas"`
	Puladas      []string          `json:"puladas"`
	Erros        map[string]string `json:"erros"`
	RequerReboot bool              `json:"requer_reboot"`
	RestorePoint bool              `json:"restore_point"`
	LimpezaMB    float64           `json:"limpeza_mb,omitempty"`
}

var histMu sync.Mutex

// dataDir e writeFileAtomic vivem em winutil (fs.go) desde que a sessao passou a
// gravar estado na mesma pasta — mantidos aqui como atalho para nao espalhar a
// chamada em todo o motor.
func dataDir() string { return winutil.DataDir() }

func histPath() string { return filepath.Join(dataDir(), "historico.json") }

// LoadHistory devolve os lotes aplicados (mais novos por ultimo).
func LoadHistory() []BatchRecord {
	histMu.Lock()
	defer histMu.Unlock()
	return loadHistoryLocked()
}

func loadHistoryLocked() []BatchRecord {
	var h []BatchRecord
	b, err := os.ReadFile(histPath())
	if err != nil {
		return h
	}
	_ = json.Unmarshal(b, &h)
	return h
}

func saveHistoryLocked(h []BatchRecord) {
	b, _ := json.MarshalIndent(h, "", "  ")
	writeFileAtomic(histPath(), b)
}

// hasData diz se um RuleUndo carrega algum snapshot reversível.
func (u *RuleUndo) hasData() bool {
	return u != nil && (len(u.Regs) > 0 || len(u.Servicos) > 0 || len(u.Powercfg) > 0)
}

// persistBatch grava o lote no histórico de forma incremental (write-ahead):
// insere ou substitui o registro com o mesmo ID. Chamado após CADA regra que
// produz undo, para que um crash/kill no meio do lote não apague os snapshots já
// escritos — a rede de segurança do undo exato precisa sobreviver a falha parcial.
func persistBatch(batch BatchRecord) {
	if len(batch.Rules) == 0 {
		return
	}
	histMu.Lock()
	defer histMu.Unlock()
	h := loadHistoryLocked()
	found := false
	for i := range h {
		if h[i].ID == batch.ID {
			h[i] = batch
			found = true
			break
		}
	}
	if !found {
		h = append(h, batch)
	}
	saveHistoryLocked(h)
}

func writeFileAtomic(path string, data []byte) error { return winutil.WriteFileAtomic(path, data) }

// ---- Log de operacoes (auditoria antes/depois) ------------------------------

func logPath() string { return filepath.Join(dataDir(), "operacoes.log") }

func logOp(lines ...string) {
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, l := range lines {
		f.WriteString(l + "\r\n")
	}
}

func regOldStr(s winutil.RegSnapshot) string {
	if !s.Existed {
		return "(ausente)"
	}
	switch s.Type {
	case 4: // REG_DWORD
		return strconv.FormatInt(int64(int32(s.DWord)), 10)
	case 11: // REG_QWORD
		return strconv.FormatUint(s.QWord, 10)
	default:
		return s.Str
	}
}

func actionNewStr(r *Rule, name string) string {
	if r.Action == nil {
		return ""
	}
	for _, v := range r.Action.Valores {
		if v.Name == name {
			return asString(v.Value)
		}
	}
	return ""
}

func logApplied(ts string, r *Rule, undo *RuleUndo, freedMB float64) {
	lines := []string{fmt.Sprintf("[%s] APLICADO  %-34s %s", ts, r.ID, r.Titulo)}
	if undo != nil {
		for _, s := range undo.Regs {
			lines = append(lines, "    reg "+s.Path+`\`+s.Name+": "+regOldStr(s)+" => "+actionNewStr(r, s.Name))
		}
		for _, sv := range undo.Servicos {
			old := "ausente"
			if sv.Existed {
				old = winutil.StartTypeName(sv.OldStartType)
			}
			lines = append(lines, "    svc "+sv.Name+": "+old+" => Disabled")
		}
		for _, p := range undo.Powercfg {
			if p.Kind == "setacvalue" {
				lines = append(lines, fmt.Sprintf("    powercfg %s/%s: %d => (novo)", p.Sub, p.Setting, p.OldValue))
			} else if p.Kind == "setactive" {
				lines = append(lines, "    powercfg esquema: "+p.Scheme+" => (novo)")
			}
		}
	}
	if freedMB > 0 {
		lines = append(lines, fmt.Sprintf("    limpeza: %.1f MB liberados", freedMB))
	}
	logOp(lines...)
}

// ---- Aplicacao --------------------------------------------------------------

// ApplyRules aplica as regras cujos ids estao em `ids` (pula nao-aplicaveis, ja
// aplicadas e as que pedem consentimento sem `confirmar`). Cria 1 ponto de
// restauracao no inicio e registra snapshots para undo exato.
func ApplyRules(rules []Rule, ids []string, ctx Ctx, confirmar bool, origem string) ApplyReport {
	return ApplyRulesProgresso(rules, ids, ctx, confirmar, origem, nil)
}

// ApplyRulesProgresso e o ApplyRules com aviso de andamento: `progresso` e
// chamado antes de cada regra. Existe para a fila da sessao poder mostrar onde
// esta sem quebrar a garantia de UM lote (e UM ponto de restauracao) por
// chamada — que e o que mantem o undo por lote fazendo sentido.
func ApplyRulesProgresso(rules []Rule, ids []string, ctx Ctx, confirmar bool, origem string,
	progresso func(id, titulo string, feitos, total int)) ApplyReport {
	rep := ApplyReport{Erros: map[string]string{}}
	byID := map[string]*Rule{}
	for i := range rules {
		byID[rules[i].ID] = &rules[i]
	}

	// Dedupe preservando ordem: evita rodar a mesma regra 2x no mesmo lote (ex.:
	// cleanup, que sempre reexecuta por nao ter deteccao de "ja aplicado" — se o
	// id aparecer repetido na lista, limpava/aplicava em dobro sem necessidade).
	seen := map[string]bool{}
	dedup := ids[:0:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			dedup = append(dedup, id)
		}
	}
	ids = dedup

	// Ponto de restauracao (rede de seguranca secundaria; best-effort).
	if err := winutil.CreateRestorePoint("ThazzDraco Optimizer - antes de aplicar"); err == nil {
		rep.RestorePoint = true
	}

	batch := BatchRecord{
		ID:     "lote-" + time.Now().Format("20060102-150405"),
		Quando: time.Now().Format("2006-01-02 15:04:05"),
		Origem: origem,
	}
	logOp(fmt.Sprintf("=== %s  LOTE %s  origem=%s ===", batch.Quando, batch.ID, origem))

	for i, id := range ids {
		r := byID[id]
		if progresso != nil {
			titulo := id
			if r != nil {
				titulo = r.Titulo
			}
			progresso(id, titulo, i, len(ids))
		}
		if r == nil || r.Action == nil {
			rep.Puladas = append(rep.Puladas, id)
			continue
		}
		if !TestGate(r.HardwareGate, ctx.Profile) {
			rep.Puladas = append(rep.Puladas, id)
			continue
		}
		if r.RequerConsentimento && !confirmar {
			rep.Puladas = append(rep.Puladas, id)
			continue
		}
		// Idempotente: pula o que ja esta aplicado.
		if r.Action.Tipo != "cleanup" && DetectRule(r, ctx).Estado == "aplicado" {
			rep.Puladas = append(rep.Puladas, id)
			continue
		}

		undo, freed, err := applyAction(r, ctx)
		// Persiste o snapshot ANTES de tratar o erro: se a escrita falhou no meio
		// (ex.: 3ª de 4 chaves), os valores já escritos precisam ficar no histórico
		// para o undo exato — senão a escrita parcial fica órfã, sem reversão.
		if undo.hasData() {
			batch.Rules = append(batch.Rules, *undo)
			rep.BatchID = batch.ID
			persistBatch(batch) // write-ahead: cada regra é gravada na hora
		}
		if err != nil {
			rep.Erros[id] = err.Error()
			logApplied(batch.Quando, r, undo, 0)
			continue
		}
		rep.Aplicadas = append(rep.Aplicadas, id)
		rep.LimpezaMB += freed
		if r.RequerReboot {
			rep.RequerReboot = true
		}
		logApplied(batch.Quando, r, undo, freed)
	}
	return rep
}

// applyAction executa a action de uma regra e devolve os dados de undo.
func applyAction(r *Rule, ctx Ctx) (*RuleUndo, float64, error) {
	a := r.Action
	undo := &RuleUndo{RuleID: r.ID, Titulo: r.Titulo}

	switch a.Tipo {
	case "registry":
		for _, v := range a.Valores {
			snap := winutil.SnapshotValue(a.Hive, ctx.Sid, a.HkcuRealUser, a.Path, v.Name)
			undo.Regs = append(undo.Regs, snap)
			fallback, err := applyRegValue(a.Hive, ctx.Sid, a.HkcuRealUser, a.Path, v)
			undo.Regs[len(undo.Regs)-1].Fallback = fallback
			if err != nil {
				return undo, 0, err
			}
		}
		return undo, 0, nil

	case "registry-foreach":
		subs, err := winutil.ListSubkeys(a.Hive, ctx.Sid, false, a.Base)
		if err != nil {
			return undo, 0, err
		}
		for _, s := range subs {
			full := a.Base + `\` + s
			for _, v := range a.Valores {
				snap := winutil.SnapshotValue(a.Hive, ctx.Sid, false, full, v.Name)
				undo.Regs = append(undo.Regs, snap)
				fallback, err := applyRegValue(a.Hive, ctx.Sid, false, full, v)
				undo.Regs[len(undo.Regs)-1].Fallback = fallback
				if err != nil {
					return undo, 0, err
				}
			}
		}
		return undo, 0, nil

	case "service":
		st := winutil.StartTypeFromName(a.Startup)
		for _, n := range a.Servicos {
			old, exists := winutil.ServiceStartType(n)
			undo.Servicos = append(undo.Servicos, ServiceSnap{Name: n, Existed: exists, OldStartType: old})
			if !exists {
				continue
			}
			if err := winutil.SetServiceStartType(n, st); err != nil {
				return undo, 0, err
			}
		}
		return undo, 0, nil

	case "powercfg":
		for _, cmd := range a.Comandos {
			if len(cmd) >= 2 && cmd[0] == "/setactive" {
				undo.Powercfg = append(undo.Powercfg, PowercfgUndo{Kind: "setactive", Scheme: winutil.ActiveScheme()})
			} else if len(cmd) >= 5 && cmd[0] == "/setacvalueindex" {
				old, had := winutil.PowercfgAcValue(cmd[2], cmd[3])
				// Guarda o GUID do esquema ATIVO no momento da escrita: o undo grava
				// nele, não no "SCHEME_CURRENT" do momento do desfazer (que pode ser
				// outro plano — corrompia um esquema não tocado).
				undo.Powercfg = append(undo.Powercfg, PowercfgUndo{Kind: "setacvalue", Scheme: winutil.ActiveScheme(), Sub: cmd[2], Setting: cmd[3], OldValue: old, HadValue: had})
			}
		}
		for _, cmd := range a.Comandos {
			if _, err := winutil.RunPowercfg(cmd...); err != nil {
				return undo, 0, err
			}
		}
		return undo, 0, nil

	case "power-max":
		// Snapshot do plano anterior (undo restaura). Desktop -> Desempenho Máximo
		// (Ultimate); notebook -> Alto Desempenho (Ultimate fritaria a bateria).
		undo.Powercfg = append(undo.Powercfg, PowercfgUndo{Kind: "setactive", Scheme: winutil.ActiveScheme()})
		if ctxDesktop(ctx) {
			res := winutil.UltimatePerformance()
			if ok, _ := res["ok"].(bool); !ok {
				return undo, 0, fmt.Errorf("falha ao ativar plano de desempenho máximo")
			}
		} else {
			if _, err := winutil.RunPowercfg("/setactive", "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"); err != nil {
				return undo, 0, err
			}
		}
		return undo, 0, nil

	case "cleanup":
		freed := runCleanup(cleanupTargets(ctx, a.Alvos))
		return nil, freed, nil // limpeza nao tem undo (so apaga regeneravel)
	}
	return nil, 0, fmt.Errorf("action.tipo nao suportado: %s", a.Tipo)
}

// ctxDesktop diz se o PC é desktop (não notebook), a partir do perfil.
func ctxDesktop(ctx Ctx) bool {
	if c, ok := ctx.Profile["contexto"].(map[string]any); ok {
		if t, ok := c["tipo"].(string); ok {
			return t != "notebook"
		}
	}
	return true // sem info: assume desktop
}

func applyRegValue(hive, sid string, hkcuReal bool, path string, v RegVal) (fallback bool, err error) {
	switch v.ValueType {
	case "DWord":
		n, _ := numOf(v.Value)
		return winutil.WriteDWord(hive, sid, hkcuReal, path, v.Name, uint32(int32(int64(n))))
	case "String":
		return winutil.WriteString(hive, sid, hkcuReal, path, v.Name, asString(v.Value))
	}
	return false, fmt.Errorf("value_type nao suportado: %s", v.ValueType)
}

// isSafeCleanup so libera pastas temporarias/cache (allowlist). Espelha
// Test-SafeCleanupPath: protege contra apagar qualquer coisa fora de temp.
func isSafeCleanup(p string) bool {
	n := strings.ToLower(strings.TrimRight(p, `\`))
	return strings.HasSuffix(n, `\temp`) ||
		strings.HasSuffix(n, `\windows\temp`) ||
		strings.HasSuffix(n, `\prefetch`)
}

// runCleanup apaga SOMENTE arquivos dentro das pastas temporarias permitidas.
// Nunca remove as pastas em si nem toca em diretorios pessoais. Recebe os
// diretorios ja resolvidos (ver cleanupTargets).
func runCleanup(dirs []string) float64 {
	var freed int64
	for _, p := range dirs {
		if !isSafeCleanup(p) {
			continue // recusa qualquer caminho fora da allowlist
		}
		filepath.WalkDir(p, func(path string, dent os.DirEntry, err error) error {
			if err != nil || dent.IsDir() {
				return nil
			}
			info, e := dent.Info()
			if e != nil {
				return nil
			}
			if os.Remove(path) == nil {
				freed += info.Size()
			}
			return nil
		})
	}
	return float64(freed) / (1024 * 1024)
}

// ---- Desfazer ---------------------------------------------------------------

// revertRule tenta reverter uma regra e devolve erro se ALGUMA restauração falhar
// (ex.: rodou sem elevação e a escrita HKLM foi negada). O chamador usa isso para
// NÃO remover do histórico o que não foi de fato revertido — senão a UI mostraria
// "desfeito" com o sistema ainda alterado e a info de undo perdida para sempre.
// revertRule reverte uma regra. `skip` (pode ser nil) contém chaves de registro
// tocadas por lotes MAIS NOVOS: essas não são revertidas agora (o lote novo precisa
// ser desfeito primeiro, senão restauraríamos um valor intermediário) — a regra fica
// no histórico para nova tentativa.
func revertRule(ru RuleUndo, skip map[string]bool) error {
	var errs []string
	for _, snap := range ru.Regs {
		if skip != nil && skip[regKey(snap)] {
			errs = append(errs, "reg "+snap.Name+": desfaça primeiro o lote mais novo que alterou este valor")
			continue
		}
		if err := winutil.RestoreSnapshot(snap); err != nil {
			errs = append(errs, "reg "+snap.Name+": "+err.Error())
		}
	}
	for _, s := range ru.Servicos {
		if s.Existed {
			if err := winutil.SetServiceStartType(s.Name, s.OldStartType); err != nil {
				errs = append(errs, "svc "+s.Name+": "+err.Error())
			}
		}
	}
	// Powercfg em ordem reversa: restaura valores AC e reativa esquemas.
	for i := len(ru.Powercfg) - 1; i >= 0; i-- {
		p := ru.Powercfg[i]
		switch p.Kind {
		case "setacvalue":
			if p.HadValue {
				scheme := p.Scheme
				if scheme == "" {
					scheme = "SCHEME_CURRENT"
				}
				if _, err := winutil.RunPowercfg("/setacvalueindex", scheme, p.Sub, p.Setting, strconv.FormatInt(p.OldValue, 10)); err != nil {
					errs = append(errs, "powercfg "+p.Setting+": "+err.Error())
				}
				// Só re-aplica se esse esquema for o ativo agora — não troca o plano do usuário.
				if scheme == "SCHEME_CURRENT" || scheme == winutil.ActiveScheme() {
					winutil.RunPowercfg("/setactive", scheme)
				}
			}
		case "setactive":
			if p.Scheme != "" {
				if _, err := winutil.RunPowercfg("/setactive", p.Scheme); err != nil {
					errs = append(errs, "powercfg esquema: "+err.Error())
				}
			}
		}
	}
	if len(errs) > 0 {
		logOp(fmt.Sprintf("[%s] FALHA-UNDO %-34s %s", time.Now().Format("2006-01-02 15:04:05"), ru.RuleID, strings.Join(errs, "; ")))
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	logOp(fmt.Sprintf("[%s] DESFEITO  %-34s %s", time.Now().Format("2006-01-02 15:04:05"), ru.RuleID, ru.Titulo))
	return nil
}

// regKey identifica um valor de registro (para detectar sobreposição entre lotes).
func regKey(s winutil.RegSnapshot) string {
	return s.Hive + "|" + s.Sid + "|" + s.Path + "|" + s.Name
}

// newerRegKeys coleta as chaves de registro tocadas por um conjunto de lotes (os
// mais novos), para a disciplina LIFO do undo.
func newerRegKeys(batches []BatchRecord) map[string]bool {
	m := map[string]bool{}
	for _, b := range batches {
		for _, ru := range b.Rules {
			for _, s := range ru.Regs {
				m[regKey(s)] = true
			}
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// UndoBatch desfaz um lote inteiro e o remove do historico.
func UndoBatch(batchID string) (int, error) {
	histMu.Lock()
	defer histMu.Unlock()
	h := loadHistoryLocked()
	idx := -1
	for i := range h {
		if h[i].ID == batchID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, fmt.Errorf("lote nao encontrado: %s", batchID)
	}
	skip := newerRegKeys(h[idx+1:]) // valores tocados por lotes mais novos: LIFO
	var kept []RuleUndo
	var falhas []string
	reverted := 0
	for i := len(h[idx].Rules) - 1; i >= 0; i-- {
		if err := revertRule(h[idx].Rules[i], skip); err != nil {
			kept = append(kept, h[idx].Rules[i]) // mantém no histórico p/ nova tentativa
			falhas = append(falhas, h[idx].Rules[i].RuleID)
		} else {
			reverted++
		}
	}
	if len(kept) == 0 {
		h = append(h[:idx], h[idx+1:]...) // tudo revertido: remove o lote
	} else {
		h[idx].Rules = kept // guarda só o que faltou reverter
	}
	saveHistoryLocked(h)
	if len(falhas) > 0 {
		return reverted, fmt.Errorf("falha ao reverter %d regra(s) (sem permissão?): %s", len(falhas), strings.Join(falhas, ", "))
	}
	return reverted, nil
}

// UndoRule desfaz a aplicacao mais recente de uma regra especifica.
func UndoRule(ruleID string) error {
	histMu.Lock()
	defer histMu.Unlock()
	h := loadHistoryLocked()
	for bi := len(h) - 1; bi >= 0; bi-- {
		for ri := len(h[bi].Rules) - 1; ri >= 0; ri-- {
			if h[bi].Rules[ri].RuleID == ruleID {
				if err := revertRule(h[bi].Rules[ri], newerRegKeys(h[bi+1:])); err != nil {
					return err // não remove do histórico: reversão falhou (ou lote mais novo pendente)
				}
				h[bi].Rules = append(h[bi].Rules[:ri], h[bi].Rules[ri+1:]...)
				if len(h[bi].Rules) == 0 {
					h = append(h[:bi], h[bi+1:]...)
				}
				saveHistoryLocked(h)
				return nil
			}
		}
	}
	return fmt.Errorf("regra nao encontrada no historico: %s", ruleID)
}

// UndoAll desfaz todos os lotes (mais novos primeiro). Mantém no histórico só o
// que NÃO conseguiu reverter, para nova tentativa — nunca apaga o que continua
// aplicado. Devolve quantas regras foram de fato revertidas.
func UndoAll() int {
	histMu.Lock()
	defer histMu.Unlock()
	h := loadHistoryLocked()
	count := 0
	var restante []BatchRecord
	// Do MAIS NOVO ao mais antigo (LIFO): desfazer na ordem inversa garante que um
	// valor tocado por vários lotes volte ao estado original correto.
	for bi := len(h) - 1; bi >= 0; bi-- {
		var kept []RuleUndo
		for ri := len(h[bi].Rules) - 1; ri >= 0; ri-- {
			if err := revertRule(h[bi].Rules[ri], nil); err != nil {
				kept = append(kept, h[bi].Rules[ri])
			} else {
				count++
			}
		}
		if len(kept) > 0 {
			b := h[bi]
			b.Rules = kept
			restante = append(restante, b)
		}
	}
	saveHistoryLocked(restante)
	return count
}
