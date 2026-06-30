//go:build windows

package winutil

import (
	"golang.org/x/sys/windows/registry"
)

// RegSnapshot guarda o estado anterior de um valor de registro para permitir undo
// exato (regrava o valor antigo ou apaga se ele nao existia antes).
type RegSnapshot struct {
	Hive     string `json:"hive"`
	Sid      string `json:"sid,omitempty"`
	HkcuReal bool   `json:"hkcu_real,omitempty"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Existed  bool   `json:"existed"`
	Type     uint32 `json:"type,omitempty"`
	DWord    uint32 `json:"dword,omitempty"`
	QWord    uint64 `json:"qword,omitempty"`
	Str      string `json:"str,omitempty"`
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
	}
	return snap
}

// RestoreSnapshot desfaz: regrava o valor antigo, ou apaga se ele nao existia.
func RestoreSnapshot(s RegSnapshot) error {
	root, sub := resolve(s.Hive, s.Sid, s.HkcuReal, s.Path)
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
	}
	return nil
}

// ---- Escrita (apply) ---------------------------------------------------------

// WriteDWord cria a chave se preciso e grava um DWORD.
func WriteDWord(hive, sid string, hkcuReal bool, path, name string, val uint32) error {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, _, err := registry.CreateKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue(name, val)
}

// WriteString cria a chave se preciso e grava um SZ.
func WriteString(hive, sid string, hkcuReal bool, path, name, val string) error {
	root, sub := resolve(hive, sid, hkcuReal, path)
	k, _, err := registry.CreateKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, val)
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
