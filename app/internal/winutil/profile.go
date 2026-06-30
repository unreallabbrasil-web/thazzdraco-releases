//go:build windows

package winutil

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// BuildProfile monta o mini-perfil do PC (contexto + hardware) usado pelos gates
// e pelas regras consultivas. Tudo por API nativa do Windows; cada sonda e
// "best-effort" (falha vira valor ausente, nunca trava). Espelha Build-Profile
// do lib.ps1, mas sem WMI/PowerShell:
//   - RAM total ......... GlobalMemoryStatusEx
//   - RAM rated/config .. SMBIOS (GetSystemFirmwareTable, tabela tipo 17)
//   - notebook .......... SMBIOS tipo 3 (chassi) + bateria presente
//   - na_bateria ........ GetSystemPowerStatus
//   - display ........... EnumDisplaySettings (atual + maximo p/ a resolucao)
//   - discos ............ IOCTL_STORAGE_QUERY_PROPERTY (seek penalty => SSD/HDD)
//   - dns ............... GetAdaptersAddresses
func BuildProfile() map[string]any {
	tipo := "desktop"
	naBateria := false
	if temBateria, emBateria, ok := powerStatus(); ok {
		if temBateria {
			tipo = "notebook"
		}
		naBateria = emBateria
	}

	ramRated, ramConfigured, chassisLaptop := smbiosInfo()
	if chassisLaptop {
		tipo = "notebook"
	}

	ramTotalGB := totalRAMGB()
	refAtual, refMax := displayRefresh()
	discos := diskTypes()
	dns := dnsServers()
	gpuVendor := PrimaryGPUVendor()

	if ramConfigured == 0 && ramRated > 0 {
		ramConfigured = ramRated
	}

	return map[string]any{
		"contexto": map[string]any{
			"tipo":       tipo,
			"na_bateria": naBateria,
		},
		"hardware": map[string]any{
			"ram": map[string]any{
				"configured_mhz": nilIfZero(ramConfigured),
				"rated_mhz":      nilIfZero(ramRated),
				"total_gb":       nilIfZero(ramTotalGB),
			},
			"gpu": map[string]any{
				"vendor": gpuVendor,
			},
		},
		"display": map[string]any{
			"refresh_atual_hz": nilIfZero(refAtual),
			"refresh_max_hz":   nilIfZero(refMax),
		},
		"discos": discos,
		"rede":   map[string]any{"dns": dns},
	}
}

func nilIfZero(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

// ---- RAM total --------------------------------------------------------------

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemStatus   = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSysPowerStatus = kernel32.NewProc("GetSystemPowerStatus")
	procGetSysFirmware    = kernel32.NewProc("GetSystemFirmwareTable")
	user32                = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplay       = user32.NewProc("EnumDisplaySettingsW")
)

func totalRAMGB() int {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r1, _, _ := procGlobalMemStatus.Call(uintptr(unsafe.Pointer(&m)))
	if r1 == 0 {
		return 0
	}
	// Arredonda para o GB mais proximo (ex.: 16).
	return int((m.TotalPhys + (1 << 29)) / (1 << 30))
}

// ---- Energia / bateria ------------------------------------------------------

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// powerStatus -> (temBateria, naBateria, ok).
func powerStatus() (bool, bool, bool) {
	var s systemPowerStatus
	r1, _, _ := procGetSysPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	if r1 == 0 {
		return false, false, false
	}
	temBateria := s.BatteryFlag != 128 && s.BatteryFlag != 255 // 128=sem bateria,255=desconhecido
	naBateria := s.ACLineStatus == 0                           // 0=offline (na bateria)
	return temBateria, naBateria, true
}

// ---- SMBIOS (RAM speed + chassi) --------------------------------------------

const rsmbProvider = 0x52534D42 // 'RSMB'

// smbiosInfo -> (ratedMHz, configuredMHz, chassiNotebook).
func smbiosInfo() (int, int, bool) {
	size, _, _ := procGetSysFirmware.Call(uintptr(rsmbProvider), 0, 0, 0)
	if size == 0 {
		return 0, 0, false
	}
	buf := make([]byte, size)
	n, _, _ := procGetSysFirmware.Call(uintptr(rsmbProvider), 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) > len(buf) {
		return 0, 0, false
	}
	data := buf[:n]
	if len(data) < 8 {
		return 0, 0, false
	}
	// Cabecalho RawSMBIOSData: 8 bytes; tabela comeca em data[8:].
	tables := data[8:]

	rated, configured := 0, 0
	laptop := false
	laptopChassis := map[byte]bool{8: true, 9: true, 10: true, 11: true, 12: true,
		14: true, 18: true, 21: true, 30: true, 31: true, 32: true}

	i := 0
	for i+4 <= len(tables) {
		stype := tables[i]
		slen := int(tables[i+1])
		if slen < 4 || i+slen > len(tables) {
			break
		}
		formatted := tables[i : i+slen]

		switch stype {
		case 17: // Memory Device
			if slen > 0x16 {
				sp := int(u16(formatted, 0x15))
				if sp > 0 && sp > rated {
					rated = sp
				}
			}
			if slen > 0x21 {
				cs := int(u16(formatted, 0x20))
				if cs > 0 && cs > configured {
					configured = cs
				}
			}
		case 3: // System Enclosure
			if slen > 0x05 {
				ct := formatted[0x05] & 0x7f
				if laptopChassis[ct] {
					laptop = true
				}
			}
		}

		// Pula a area de strings ate o duplo-nulo.
		j := i + slen
		for j+1 < len(tables) && !(tables[j] == 0 && tables[j+1] == 0) {
			j++
		}
		j += 2
		if j <= i {
			break
		}
		i = j
		if stype == 127 { // End-of-Table
			break
		}
	}
	return rated, configured, laptop
}

func u16(b []byte, off int) uint16 {
	if off+1 >= len(b) {
		return 0
	}
	return uint16(b[off]) | uint16(b[off+1])<<8
}

// ---- Display (taxa de atualizacao) ------------------------------------------

type devmodeW struct {
	DeviceName         [32]uint16
	SpecVersion        uint16
	DriverVersion      uint16
	Size               uint16
	DriverExtra        uint16
	Fields             uint32
	PositionX          int32
	PositionY          int32
	DisplayOrientation uint32
	DisplayFixedOutput uint32
	Color              int16
	Duplex             int16
	YResolution        int16
	TTOption           int16
	Collate            int16
	FormName           [32]uint16
	LogPixels          uint16
	BitsPerPel         uint32
	PelsWidth          uint32
	PelsHeight         uint32
	DisplayFlags       uint32
	DisplayFrequency   uint32
	ICMMethod          uint32
	ICMIntent          uint32
	MediaType          uint32
	DitherType         uint32
	Reserved1          uint32
	Reserved2          uint32
	PanningWidth       uint32
	PanningHeight      uint32
}

const enumCurrentSettings = 0xFFFFFFFF // ENUM_CURRENT_SETTINGS

// displayRefresh -> (atualHz, maxHzParaAResolucaoAtual).
func displayRefresh() (int, int) {
	var cur devmodeW
	cur.Size = uint16(unsafe.Sizeof(cur))
	r1, _, _ := procEnumDisplay.Call(0, uintptr(enumCurrentSettings), uintptr(unsafe.Pointer(&cur)))
	if r1 == 0 {
		return 0, 0
	}
	atual := int(cur.DisplayFrequency)
	maxHz := atual

	// Varre todos os modos com a mesma resolucao e pega a maior frequencia.
	for idx := uint32(0); ; idx++ {
		var dm devmodeW
		dm.Size = uint16(unsafe.Sizeof(dm))
		ok, _, _ := procEnumDisplay.Call(0, uintptr(idx), uintptr(unsafe.Pointer(&dm)))
		if ok == 0 {
			break
		}
		if dm.PelsWidth == cur.PelsWidth && dm.PelsHeight == cur.PelsHeight {
			if int(dm.DisplayFrequency) > maxHz {
				maxHz = int(dm.DisplayFrequency)
			}
		}
	}
	return atual, maxHz
}

// ---- Discos (SSD x HDD) -----------------------------------------------------

const (
	ioctlStorageQueryProperty   = 0x002d1400
	ioctlVolumeDiskExtents      = 0x00560000 // IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS
	seekPenaltyProperty         = 7          // StorageDeviceSeekPenaltyProperty
	propertyStandardQuery       = 0
	fileShareReadWrite          = 0x00000003
	openExisting                = 3
)

type storagePropertyQuery struct {
	PropertyId           uint32
	QueryType            uint32
	AdditionalParameters [1]byte
}

type deviceSeekPenaltyDescriptor struct {
	Version           uint32
	Size              uint32
	IncursSeekPenalty byte
	_                 [3]byte
}

func diskTypes() []map[string]any {
	sysDisk := systemDiskNumber() // disco fisico que hospeda o Windows (-1 se desconhecido)
	var discos []map[string]any
	for n := 0; n < 16; n++ {
		path := fmt.Sprintf(`\\.\PhysicalDrive%d`, n)
		t, ok := seekPenalty(path)
		if !ok {
			if n == 0 {
				continue // primeiro indice pode falhar; tenta os proximos
			}
			break
		}
		disco := map[string]any{"tipo": t, "midia": t, "numero": n, "sistema": n == sysDisk}
		// O disco do sistema vai para o indice 0 (gates de SSD usam discos[0]).
		if n == sysDisk {
			discos = append([]map[string]any{disco}, discos...)
		} else {
			discos = append(discos, disco)
		}
	}
	return discos
}

// systemDiskNumber retorna o numero do PhysicalDrive que hospeda %SystemDrive%
// (normalmente C:), via IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS. -1 se nao descobrir.
func systemDiskNumber() int {
	sysDrive := "C:"
	if v := os.Getenv("SystemDrive"); v != "" {
		sysDrive = v
	}
	p, err := windows.UTF16PtrFromString(`\\.\` + sysDrive)
	if err != nil {
		return -1
	}
	h, err := windows.CreateFile(p, 0, fileShareReadWrite, nil, openExisting, 0, 0)
	if err != nil {
		return -1
	}
	defer windows.CloseHandle(h)

	// VOLUME_DISK_EXTENTS: NumberOfDiskExtents (DWORD) + extents. Reservamos espaco
	// para varios extents (volumes em spanned/RAID).
	buf := make([]byte, 16+24*8)
	var bytesReturned uint32
	err = windows.DeviceIoControl(h, ioctlVolumeDiskExtents,
		nil, 0, &buf[0], uint32(len(buf)), &bytesReturned, nil)
	if err != nil {
		return -1
	}
	count := uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	if count == 0 {
		return -1
	}
	// Primeiro extent comeca em offset 8 (apos DWORD + padding p/ alinhar LARGE_INTEGER).
	diskNum := uint32(buf[8]) | uint32(buf[9])<<8 | uint32(buf[10])<<16 | uint32(buf[11])<<24
	return int(diskNum)
}

func seekPenalty(path string) (string, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	h, err := windows.CreateFile(p, 0, fileShareReadWrite, nil, openExisting, 0, 0)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)

	q := storagePropertyQuery{PropertyId: seekPenaltyProperty, QueryType: propertyStandardQuery}
	var d deviceSeekPenaltyDescriptor
	var bytesReturned uint32
	err = windows.DeviceIoControl(h, ioctlStorageQueryProperty,
		(*byte)(unsafe.Pointer(&q)), uint32(unsafe.Sizeof(q)),
		(*byte)(unsafe.Pointer(&d)), uint32(unsafe.Sizeof(d)),
		&bytesReturned, nil)
	if err != nil {
		return "HDD", true // sem info de seek penalty: assume HDD (conservador)
	}
	if d.IncursSeekPenalty == 0 {
		return "SSD", true
	}
	return "HDD", true
}

// ---- DNS --------------------------------------------------------------------

func dnsServers() []string {
	var size uint32
	const family = windows.AF_UNSPEC
	// Primeira chamada para descobrir o tamanho.
	windows.GetAdaptersAddresses(family, 0, 0, nil, &size)
	if size == 0 {
		return []string{}
	}
	buf := make([]byte, size)
	aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
	if err := windows.GetAdaptersAddresses(family, 0, 0, aa, &size); err != nil {
		return []string{}
	}

	seen := map[string]bool{}
	var out []string
	for cur := aa; cur != nil; cur = cur.Next {
		if cur.OperStatus != windows.IfOperStatusUp {
			continue
		}
		for dns := cur.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			ip := sockaddrIPv4(dns.Address.Sockaddr)
			if ip != "" && !seen[ip] {
				seen[ip] = true
				out = append(out, ip)
			}
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func sockaddrIPv4(sa *syscall.RawSockaddrAny) string {
	if sa == nil || sa.Addr.Family != syscall.AF_INET {
		return ""
	}
	in4 := (*syscall.RawSockaddrInet4)(unsafe.Pointer(sa))
	a := in4.Addr
	return fmt.Sprintf("%d.%d.%d.%d", a[0], a[1], a[2], a[3])
}
