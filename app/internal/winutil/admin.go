//go:build windows

// Package winutil concentra todo o acesso nativo ao Windows (registro, servicos,
// powercfg, ponto de restauracao, perfil de hardware) e a elevacao UAC.
// Nenhuma chamada a PowerShell: tudo via API Win32 (golang.org/x/sys/windows).
package winutil

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// IsAdmin retorna true se o processo atual roda elevado (token elevado).
// Precisamos disso para escrever HKLM, alterar servicos e criar ponto de restauracao.
func IsAdmin() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}

// RelaunchElevated reinicia o proprio executavel pedindo elevacao (verbo "runas"),
// preservando os argumentos. Dispara o prompt do UAC.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	verbPtr, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	cwdPtr, _ := windows.UTF16PtrFromString(cwd)
	var argPtr *uint16
	if args != "" {
		argPtr, _ = windows.UTF16PtrFromString(args)
	}
	const swNormal = 1
	return windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, swNormal)
}
