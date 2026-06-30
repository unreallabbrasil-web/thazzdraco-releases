//go:build windows

package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Ponto de restauracao do sistema via SRSetRestorePointW (srclient.dll) — API
// nativa, sem PowerShell. E uma rede de seguranca SECUNDARIA: a protecao primaria
// e o snapshot por-valor que o motor guarda. Se a Protecao do Sistema estiver
// desligada (ou houver throttle de 24h), a chamada falha em silencio e seguimos.

const (
	beginSystemChange = 100 // BEGIN_SYSTEM_CHANGE
	endSystemChange   = 101 // END_SYSTEM_CHANGE
	modifySettings    = 12  // MODIFY_SETTINGS
)

type restorePointInfoW struct {
	EventType      uint32
	RestorePtType  uint32
	SequenceNumber int64
	Description    [256]uint16
}

type stateMgrStatus struct {
	Status         uint32
	SequenceNumber int64
}

var (
	srclient            = windows.NewLazySystemDLL("srclient.dll")
	procSRSetRestorePnt = srclient.NewProc("SRSetRestorePointW")
)

// CreateRestorePoint cria um ponto de restauracao "MODIFY_SETTINGS" com a
// descricao dada. Retorna nil em sucesso; erro/efetividade nao sao garantidos
// (depende da Protecao do Sistema estar ativa). Nunca deve travar o fluxo.
func CreateRestorePoint(description string) error {
	if err := procSRSetRestorePnt.Find(); err != nil {
		return err
	}
	info := restorePointInfoW{
		EventType:      beginSystemChange,
		RestorePtType:  modifySettings,
		SequenceNumber: 0,
	}
	desc := windows.StringToUTF16(description)
	if len(desc) > len(info.Description) {
		desc = desc[:len(info.Description)]
	}
	copy(info.Description[:], desc)

	var status stateMgrStatus
	r1, _, e1 := procSRSetRestorePnt.Call(
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&status)),
	)
	if r1 == 0 {
		if status.Status != 0 {
			return windows.Errno(status.Status)
		}
		return e1
	}

	// Fecha a transacao do ponto de restauracao.
	info.EventType = endSystemChange
	info.SequenceNumber = status.SequenceNumber
	procSRSetRestorePnt.Call(
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&status)),
	)
	return nil
}
