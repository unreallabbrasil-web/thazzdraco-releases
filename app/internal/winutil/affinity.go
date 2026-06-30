//go:build windows

package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcessAffinityInfo descreve um processo e sua afinidade de CPU.
type ProcessAffinityInfo struct {
	PID        uint32 `json:"pid"`
	Nome       string `json:"nome"`
	Mask       uint64 `json:"mask"`        // bitmask dos cores habilitados para este processo
	CoresAtivos int   `json:"cores_ativos"` // popcount(mask)
	TotalCores int    `json:"total_cores"`  // núcleos lógicos totais do sistema
}

var (
	procGetProcessAffinityMask = kernel32.NewProc("GetProcessAffinityMask")
	procSetProcessAffinityMask = kernel32.NewProc("SetProcessAffinityMask")
)

type systemInfoStruct struct {
	ProcessorArchitecture     uint16
	_                         uint16
	PageSize                  uint32
	MinimumApplicationAddress uintptr
	MaximumApplicationAddress uintptr
	ActiveProcessorMask       uintptr
	NumberOfProcessors        uint32
	ProcessorType             uint32
	AllocationGranularity     uint32
	ProcessorLevel            uint16
	ProcessorRevision         uint16
}

var procGetSystemInfoW = kernel32.NewProc("GetSystemInfo")

func logicalCoreCount() (int, uint64) {
	var si systemInfoStruct
	procGetSystemInfoW.Call(uintptr(unsafe.Pointer(&si)))
	return int(si.NumberOfProcessors), uint64(si.ActiveProcessorMask)
}

func popcount64(n uint64) int {
	count := 0
	for n != 0 {
		count += int(n & 1)
		n >>= 1
	}
	return count
}

// ListProcesses enumera processos visíveis e retorna nome + máscara de afinidade.
// Exclui processos de sistema (PID ≤ 8) e processos inacessíveis.
func ListProcesses() []ProcessAffinityInfo {
	totalCores, _ := logicalCoreCount()

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	var result []ProcessAffinityInfo
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		pid := pe.ProcessID
		if pid <= 8 {
			continue // System Idle Process e System
		}
		name := windows.UTF16ToString(pe.ExeFile[:])

		const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
		h, e := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if e != nil {
			continue
		}

		var procMask, sysMask uintptr
		r, _, _ := procGetProcessAffinityMask.Call(uintptr(h), uintptr(unsafe.Pointer(&procMask)), uintptr(unsafe.Pointer(&sysMask)))
		windows.CloseHandle(h)

		if r == 0 {
			continue
		}

		mask := uint64(procMask)
		result = append(result, ProcessAffinityInfo{
			PID:         pid,
			Nome:        name,
			Mask:        mask,
			CoresAtivos: popcount64(mask),
			TotalCores:  totalCores,
		})
	}
	return result
}

// SetProcessAffinity define a máscara de afinidade de um processo pelo PID.
func SetProcessAffinity(pid uint32, mask uint64) error {
	const PROCESS_SET_INFORMATION = 0x0200
	h, err := windows.OpenProcess(PROCESS_SET_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("acesso negado (processo pode ser de sistema ou já encerrado)")
	}
	defer windows.CloseHandle(h)

	r, _, e := procSetProcessAffinityMask.Call(uintptr(h), uintptr(mask))
	if r == 0 {
		return fmt.Errorf("SetProcessAffinityMask: %v", e)
	}
	return nil
}
