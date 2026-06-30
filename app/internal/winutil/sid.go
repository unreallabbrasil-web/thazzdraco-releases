//go:build windows

package winutil

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RealUserSid descobre o SID do usuario interativo real (dono do explorer.exe).
// Quando o app roda elevado, HKCU aponta para a colmeia do Administrador, nao a
// do usuario logado; com o SID conseguimos escrever em HKEY_USERS\<SID>\... que e
// a colmeia HKCU correta do usuario. Retorna "" se nao encontrar (cai p/ HKCU).
func RealUserSid() string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := windows.UTF16ToString(pe.ExeFile[:])
		if strings.EqualFold(name, "explorer.exe") {
			if sid := sidFromPID(pe.ProcessID); sid != "" {
				return sid
			}
		}
	}
	return ""
}

func sidFromPID(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return ""
	}
	defer token.Close()

	tu, err := token.GetTokenUser()
	if err != nil {
		return ""
	}
	return tu.User.Sid.String()
}
