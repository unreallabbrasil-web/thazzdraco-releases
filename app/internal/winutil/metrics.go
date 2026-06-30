//go:build windows

package winutil

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetSystemTimes = kernel32.NewProc("GetSystemTimes")
	procGetTickCount64 = kernel32.NewProc("GetTickCount64")
	psapi              = windows.NewLazySystemDLL("psapi.dll")
	procGetProcMemInfo = psapi.NewProc("GetProcessMemoryInfo")
)

// ProcInfo: processo + uso de memoria (working set) em MB.
type ProcInfo struct {
	Nome  string `json:"nome"`
	MemMB int    `json:"mem_mb"`
}

// Metrics devolve uma fotografia ao vivo do sistema (CPU/RAM/processos/uptime).
func Metrics() map[string]any {
	totalGB, usedGB, ramPct := ramInfo()
	return map[string]any{
		"cpu_pct":    cpuPercent(),
		"cpu_temp_c": CPUTempC(), // F4: temperatura via PDH thermal zone (-1 = N/A)
		"ram":        map[string]any{"total_gb": totalGB, "usado_gb": usedGB, "pct": ramPct},
		"uptime":     uptime(),
		"processos":  topProcesses(7),
		"gpus":       GPUs(),
		"mem_detail": MemBreakdown(),
	}
}

// ---- CPU% (amostragem entre chamadas de GetSystemTimes) ---------------------

var (
	cpuMu                          sync.Mutex
	prevIdle, prevKernel, prevUser uint64
	cpuReady                       bool
)

func ftU64(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

func cpuPercent() int {
	var idle, kernel, user windows.Filetime
	r1, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r1 == 0 {
		return 0
	}
	i, k, u := ftU64(idle), ftU64(kernel), ftU64(user)

	cpuMu.Lock()
	defer cpuMu.Unlock()
	if !cpuReady {
		prevIdle, prevKernel, prevUser, cpuReady = i, k, u, true
		return 0 // primeira amostra: sem delta
	}
	idleD := i - prevIdle
	total := (k - prevKernel) + (u - prevUser) // kernel ja inclui idle
	prevIdle, prevKernel, prevUser = i, k, u
	if total == 0 {
		return 0
	}
	pct := int((total - idleD) * 100 / total)
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	return pct
}

// ---- RAM --------------------------------------------------------------------

func ramInfo() (float64, float64, int) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r1, _, _ := procGlobalMemStatus.Call(uintptr(unsafe.Pointer(&m)))
	if r1 == 0 {
		return 0, 0, 0
	}
	total := float64(m.TotalPhys) / (1 << 30)
	used := float64(m.TotalPhys-m.AvailPhys) / (1 << 30)
	return math.Round(total*10) / 10, math.Round(used*10) / 10, int(m.MemoryLoad)
}

// ---- Uptime -----------------------------------------------------------------

func uptime() string {
	r1, _, _ := procGetTickCount64.Call()
	s := uint64(r1) / 1000
	d := s / 86400
	h := (s % 86400) / 3600
	m := (s % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %02dm", d, h, m)
	}
	return fmt.Sprintf("%dh %02dm", h, m)
}

// ---- Top processos por memoria ----------------------------------------------

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func topProcesses(n int) []ProcInfo {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []ProcInfo{}
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var all []ProcInfo
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		ws := workingSet(pe.ProcessID)
		if ws == 0 {
			continue
		}
		all = append(all, ProcInfo{Nome: windows.UTF16ToString(pe.ExeFile[:]), MemMB: int(ws / (1024 * 1024))})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].MemMB > all[j].MemMB })
	if len(all) > n {
		all = all[:n]
	}
	if all == nil {
		return []ProcInfo{}
	}
	return all
}

func workingSet(pid uint32) uint64 {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var pmc processMemoryCounters
	pmc.Cb = uint32(unsafe.Sizeof(pmc))
	r1, _, _ := procGetProcMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.Cb))
	if r1 == 0 {
		return 0
	}
	return uint64(pmc.WorkingSetSize)
}
