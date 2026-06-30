package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"thazzdraco/internal/winutil"
)

// StartupState guarda o estado (ligado/desligado) de uma entrada de inicializacao.
type StartupState struct {
	Kind    string `json:"kind"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Backup e uma fotografia COMPLETA do estado atual de tudo que o app pode alterar
// (registro, servicos, powercfg, inicializacao) — para restaurar o PC exatamente
// como estava antes da sessao.
type Backup struct {
	ID       string                `json:"id"`
	Quando   string                `json:"quando"`
	Regs     []winutil.RegSnapshot `json:"regs"`
	Servicos []ServiceSnap         `json:"servicos"`
	Powercfg []PowercfgUndo        `json:"powercfg"`
	Startup  []StartupState        `json:"startup"`
}

// BackupInfo e o resumo (para listar).
type BackupInfo struct {
	ID      string `json:"id"`
	Quando  string `json:"quando"`
	Ajustes int    `json:"ajustes"`
	Startup int    `json:"startup"`
}

func backupsDir() string {
	d := filepath.Join(dataDir(), "backups")
	os.MkdirAll(d, 0o755)
	return d
}

// BuildBackup captura o estado atual de todas as regras + inicializacao.
func BuildBackup(rules []Rule, ctx Ctx) Backup {
	b := Backup{
		ID:     "backup-" + time.Now().Format("20060102-150405"),
		Quando: time.Now().Format("2006-01-02 15:04:05"),
	}
	for i := range rules {
		r := &rules[i]
		if r.Action == nil {
			continue
		}
		a := r.Action
		switch a.Tipo {
		case "registry":
			for _, v := range a.Valores {
				b.Regs = append(b.Regs, winutil.SnapshotValue(a.Hive, ctx.Sid, a.HkcuRealUser, a.Path, v.Name))
			}
		case "registry-foreach":
			subs, _ := winutil.ListSubkeys(a.Hive, ctx.Sid, false, a.Base)
			for _, s := range subs {
				full := a.Base + `\` + s
				for _, v := range a.Valores {
					b.Regs = append(b.Regs, winutil.SnapshotValue(a.Hive, ctx.Sid, false, full, v.Name))
				}
			}
		case "service":
			for _, n := range a.Servicos {
				old, ex := winutil.ServiceStartType(n)
				b.Servicos = append(b.Servicos, ServiceSnap{Name: n, Existed: ex, OldStartType: old})
			}
		case "powercfg":
			for _, cmd := range a.Comandos {
				if len(cmd) >= 2 && cmd[0] == "/setactive" {
					b.Powercfg = append(b.Powercfg, PowercfgUndo{Kind: "setactive", Scheme: winutil.ActiveScheme()})
				} else if len(cmd) >= 5 && cmd[0] == "/setacvalueindex" {
					old, had := winutil.PowercfgAcValue(cmd[2], cmd[3])
					b.Powercfg = append(b.Powercfg, PowercfgUndo{Kind: "setacvalue", Sub: cmd[2], Setting: cmd[3], OldValue: old, HadValue: had})
				}
			}
		}
	}
	for _, e := range winutil.ListStartup(ctx.Sid) {
		b.Startup = append(b.Startup, StartupState{Kind: e.Kind, Key: e.Key, Name: e.Name, Enabled: e.Enabled})
	}
	return b
}

// SaveBackup grava o backup em %ProgramData%\ThazzDraco\backups\<id>.json.
func SaveBackup(b Backup) error {
	data, _ := json.MarshalIndent(b, "", "  ")
	return writeFileAtomic(filepath.Join(backupsDir(), b.ID+".json"), data)
}

// ListBackups devolve o resumo dos backups existentes (mais novos por ultimo).
func ListBackups() []BackupInfo {
	var out []BackupInfo
	ents, err := os.ReadDir(backupsDir())
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := loadBackup(e.Name()[:len(e.Name())-5])
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{ID: b.ID, Quando: b.Quando, Ajustes: len(b.Regs) + len(b.Servicos) + len(b.Powercfg), Startup: len(b.Startup)})
	}
	return out
}

func loadBackup(id string) (Backup, error) {
	var b Backup
	data, err := os.ReadFile(filepath.Join(backupsDir(), id+".json"))
	if err != nil {
		return b, err
	}
	err = json.Unmarshal(data, &b)
	return b, err
}

// DeleteBackup remove um arquivo de backup.
func DeleteBackup(id string) error {
	if id == "" {
		return fmt.Errorf("id vazio")
	}
	return os.Remove(filepath.Join(backupsDir(), id+".json"))
}

// RestoreBackup restaura TODO o estado salvo (registro, servicos, powercfg,
// inicializacao). Conta SO os itens que voltaram de verdade: devolve (restaurados,
// falhas, err). Se rodar sem admin, a maioria das escritas HKLM/servicos falha — e
// o usuario PRECISA saber disso (a restauracao e a rede de seguranca; mentir "ok"
// quando nada mudou e o pior cenario possivel).
func RestoreBackup(id, sid string) (restored int, failed int, err error) {
	b, err := loadBackup(id)
	if err != nil {
		return 0, 0, err
	}
	bump := func(e error) {
		if e != nil {
			failed++
		} else {
			restored++
		}
	}
	for _, s := range b.Regs {
		bump(winutil.RestoreSnapshot(s))
	}
	for _, s := range b.Servicos {
		if s.Existed {
			bump(winutil.SetServiceStartType(s.Name, s.OldStartType))
		}
	}
	for _, p := range b.Powercfg {
		if p.Kind == "setacvalue" && p.HadValue {
			_, e := winutil.RunPowercfg("/setacvalueindex", "SCHEME_CURRENT", p.Sub, p.Setting, strconv.FormatInt(p.OldValue, 10))
			bump(e)
		}
	}
	winutil.RunPowercfg("/setactive", "SCHEME_CURRENT")
	for _, p := range b.Powercfg {
		if p.Kind == "setactive" && p.Scheme != "" {
			_, e := winutil.RunPowercfg("/setactive", p.Scheme)
			bump(e)
		}
	}
	for _, s := range b.Startup {
		bump(winutil.SetStartupEnabled(s.Kind, s.Key, s.Enabled, sid))
	}
	logOp(fmt.Sprintf("[%s] RESTAURADO backup %s (%d ok, %d falhas)", time.Now().Format("2006-01-02 15:04:05"), id, restored, failed))
	return restored, failed, nil
}
