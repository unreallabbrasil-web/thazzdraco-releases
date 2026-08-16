//go:build windows

package winutil

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// RegSnapshot guarda o estado anterior de um valor de registro para permitir undo
// exato (regrava o valor antigo ou apaga se ele nao existia antes).
type RegSnapshot struct {
	Hive     string `json:"hive"`
	Sid      string `json:"sid,omitempty"`
	HkcuReal bool   `json:"hkcu_real,omitempty"`
	// Fallback marca que a ESCRITA de fato foi feita em HKEY_CURRENT_USER (nao em
	// HKEY_USERS\<Sid>) porque o caminho preferido negou acesso — ver
	// createKeyForWrite. O undo precisa saber disso pra restaurar na MESMA colmeia
	// onde o valor realmente foi alterado, senao RestoreSnapshot tentaria escrever
	// de volta no caminho negado e falharia do mesmo jeito.
	Fallback bool   `json:"fallback,omitempty"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Existed  bool     `json:"existed"`
	Type     uint32   `json:"type,omitempty"`
	DWord    uint32   `json:"dword,omitempty"`
	QWord    uint64   `json:"qword,omitempty"`
	Str      string   `json:"str,omitempty"`
	Multi    []string `json:"multi,omitempty"` // REG_MULTI_SZ
	Bin      []byte   `json:"bin,omitempty"`   // REG_BINARY
}

// resolve mapeia (hive, sid, hkcuReal, path) para a chave-raiz e o subcaminho.
// Base PRIMARIA usada para escrita/snapshot/restore (um caminho so), espelhando
// Resolve-RegBase do lib.ps1.
func resolve(hive, sid string, hkcuReal bool, path string) (registry.Key, string) {
	switch hive {
	case "HKLM":
		return registry.LOCAL_MACHINE, path
	default: // HKCU
		if sid != "" && hkcuReal {
			return registry.USERS, sid + `\` + path
		}
		return registry.CURRENT_USER, path
	}
}

// ---- Leitura (detect) --------------------------------------------------------

// ReadInteger le um DWORD/QWORD. DWORD e interpretado como inteiro com sinal para
// suportar sentinelas como NetworkThrottlingIndex = -1 (0xFFFFFFFF). Faz fallback
// de HKEY_USERS\<SID> para HKCU, como o detect do PowerShell.
func ReadInteger(hive, sid string, hkcuReal bool, path, name string) (int64, bool) {
	root, sub := resolve(hive, sid, hkcuReal, path)
	if v, ok := readIntFrom(root, sub, name); ok {
		return v, true
	}
	if hive != "HKLM" && hkcuReal {
		if v, ok := readIntFrom(registry.CURRENT_USER, path, name); ok {
			return v, true
		}
	}
	return 0, false
}

func readIntFrom(root registry.Key, sub, name string) (int64, bool) {
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, false
	}
	return int64(int32(uint32(v))), true
}

// ReadString le um valor SZ/EXPAND_SZ, com o mesmo fallback de HKCU.
func ReadString(hive, sid string, hkcuReal bool, path, name string) (string, bool) {
	root, sub := resolve(hive, sid, hkcuReal, path)
	if v, ok := readStrFrom(root, sub, name); ok {
		return v, true
	}
	if hive != "HKLM" && hkcuReal {
		if v, ok := readStrFrom(registry.CURRENT_USER, path, name); ok {
			return v, true
		}
	}
	return "", false
}

func readStrFrom(root registry.Key, sub, name string) (string, bool) {
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return "", false
	}
	return v, true
}

// ListSubkeys retorna os nomes das subchaves (usado por registry-foreach nas
// interfaces de rede do Tcpip).
func ListSubkeys(hive, sid string, hkcuReal bool, path string) ([]string, error) {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, err := registry.OpenKey(root, sub, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	return k.ReadSubKeyNames(-1)
}

// ---- Snapshot / Restore (undo) ----------------------------------------------

// SnapshotValue captura o estado atual de um valor antes de alterar.
func SnapshotValue(hive, sid string, hkcuReal bool, path, name string) RegSnapshot {
	snap := RegSnapshot{Hive: hive, Sid: sid, HkcuReal: hkcuReal, Path: path, Name: name}
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return snap // existed=false
	}
	defer k.Close()

	_, valtype, err := k.GetValue(name, nil)
	if err != nil && err != registry.ErrShortBuffer {
		return snap // valor ausente
	}
	snap.Existed = true
	snap.Type = valtype
	switch valtype {
	case registry.DWORD:
		v, _, _ := k.GetIntegerValue(name)
		snap.DWord = uint32(v)
	case registry.QWORD:
		v, _, _ := k.GetIntegerValue(name)
		snap.QWord = v
	case registry.SZ, registry.EXPAND_SZ:
		s, _, _ := k.GetStringValue(name)
		snap.Str = s
	case registry.MULTI_SZ:
		ss, _, _ := k.GetStringsValue(name)
		snap.Multi = ss
	case registry.BINARY:
		b, _, _ := k.GetBinaryValue(name)
		snap.Bin = b
	}
	return snap
}

// RestoreSnapshot desfaz: regrava o valor antigo, ou apaga se ele nao existia.
func RestoreSnapshot(s RegSnapshot) error {
	root, sub := resolve(s.Hive, s.Sid, s.HkcuReal, s.Path)
	if s.Fallback {
		root, sub = registry.CURRENT_USER, s.Path
	}
	if !s.Existed {
		k, err := registry.OpenKey(root, sub, registry.SET_VALUE)
		if err != nil {
			return nil // chave sumiu; nada a apagar
		}
		defer k.Close()
		_ = k.DeleteValue(s.Name)
		return nil
	}
	k, _, err := registry.CreateKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	switch s.Type {
	case registry.DWORD:
		return k.SetDWordValue(s.Name, s.DWord)
	case registry.QWORD:
		return k.SetQWordValue(s.Name, s.QWord)
	case registry.SZ:
		return k.SetStringValue(s.Name, s.Str)
	case registry.EXPAND_SZ:
		return k.SetExpandStringValue(s.Name, s.Str)
	case registry.MULTI_SZ:
		return k.SetStringsValue(s.Name, s.Multi)
	case registry.BINARY:
		return k.SetBinaryValue(s.Name, s.Bin)
	}
	// Tipo não suportado (RESOURCE_LIST etc.): sinaliza em vez de fingir sucesso —
	// o undo mantém o item no histórico para nova tentativa em vez de perdê-lo.
	return fmt.Errorf("tipo de registro %d não suportado no undo (%s)", s.Type, s.Name)
}

// ---- Escrita (apply) ---------------------------------------------------------

// createKeyForWrite abre/cria a chave em HKEY_USERS\<SID>\... (colmeia real do
// usuario). Se isso falhar — perfil sem SID resolvido, hive nao carregada do
// jeito esperado, ACL que nega ao processo elevado o que so o dono concederia
// via HKCU normal — cai para HKEY_CURRENT_USER, espelhando o fallback que
// ReadInteger/ReadString ja fazem na leitura. Sem isso, a escrita ficava
// "Access is denied" numa maquina onde a leitura (via HKCU) funcionaria.
func createKeyForWrite(hive, sid string, hkcuReal bool, path string) (k registry.Key, fallback bool, err error) {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, _, err = registry.CreateKey(root, sub, registry.SET_VALUE)
	if err == nil {
		return k, false, nil
	}
	if hive != "HKLM" && hkcuReal {
		if k2, _, err2 := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE); err2 == nil {
			return k2, true, nil
		}
	}
	return registry.Key(0), false, err
}

// WriteDWord cria a chave se preciso e grava um DWORD. `fallback` indica se a
// escrita caiu para HKEY_CURRENT_USER — o chamador precisa gravar isso no
// RegSnapshot pra o undo mirar a mesma colmeia.
func WriteDWord(hive, sid string, hkcuReal bool, path, name string, val uint32) (fallback bool, err error) {
	k, fallback, err := createKeyForWrite(hive, sid, hkcuReal, path)
	if err != nil {
		return false, err
	}
	defer k.Close()
	return fallback, k.SetDWordValue(name, val)
}

// WriteString cria a chave se preciso e grava um SZ. `fallback` indica se a
// escrita caiu para HKEY_CURRENT_USER — o chamador precisa gravar isso no
// RegSnapshot pra o undo mirar a mesma colmeia.
func WriteString(hive, sid string, hkcuReal bool, path, name, val string) (fallback bool, err error) {
	k, fallback, err := createKeyForWrite(hive, sid, hkcuReal, path)
	if err != nil {
		return false, err
	}
	defer k.Close()
	return fallback, k.SetStringValue(name, val)
}

// NameVal é um par nome→valor (string) de um valor de registro.
type NameVal struct {
	Name  string
	Value string
}

// EnumStringValues lista os valores (como string) de uma chave — usado para ler
// as entradas de inicializacao (chaves Run).
func EnumStringValues(hive, sid string, hkcuReal bool, path string) []NameVal {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	var out []NameVal
	for _, n := range names {
		if v, _, err := k.GetStringValue(n); err == nil {
			out = append(out, NameVal{n, v})
		}
	}
	return out
}

// ReadBinary le um valor REG_BINARY (usado no StartupApproved).
func ReadBinary(hive, sid string, hkcuReal bool, path, name string) ([]byte, bool) {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return nil, false
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue(name)
	if err != nil {
		return nil, false
	}
	return b, true
}

// WriteBinary grava um valor REG_BINARY (cria a chave se preciso).
func WriteBinary(hive, sid string, hkcuReal bool, path, name string, data []byte) error {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, _, err := registry.CreateKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetBinaryValue(name, data)
}

// RemoveValue apaga um valor de registro (usado para reverter tweaks por-jogo).
func RemoveValue(hive, sid string, hkcuReal bool, path, name string) {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, err := registry.OpenKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(name)
}

// DeleteKeyIfEmpty apaga a chave se ela não tiver mais valores nem subchaves. Usado
// para não deixar chaves IFEO (Image File Execution Options) vazias para trás ao
// reverter a prioridade por-jogo — chaves IFEO residuais disparam heurística de AV.
func DeleteKeyIfEmpty(hive, sid string, hkcuReal bool, path string) {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	vals, _ := k.ReadValueNames(1)
	subs, _ := k.ReadSubKeyNames(1)
	k.Close()
	if len(vals) == 0 && len(subs) == 0 {
		_ = registry.DeleteKey(root, sub)
	}
}

// RealUserProfileDir devolve a pasta de perfil (ex.: C:\Users\fulano) do usuario
// dono do SID, lendo ProfileImagePath em ProfileList. Usado para limpar o TEMP
// do usuario interativo correto quando o app roda elevado (e nao o do admin).
func RealUserProfileDir(sid string) string {
	if sid == "" {
		return ""
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+sid, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ProfileImagePath")
	if err != nil {
		return ""
	}
	if exp, err := registry.ExpandString(v); err == nil {
		return exp
	}
	return v
}
