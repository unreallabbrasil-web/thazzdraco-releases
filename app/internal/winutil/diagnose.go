//go:build windows

package winutil

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// Gargalo e um achado do diagnostico de desempenho: o que foi medido (dado
// real), por que afeta o FPS e como corrigir. Severidade ordena a exibicao.
type Gargalo struct {
	ID         string `json:"id"`
	Titulo     string `json:"titulo"`
	Severidade string `json:"severidade"` // critico | atencao | bom | info
	Detectado  string `json:"detectado"`  // valor real medido
	Impacto    string `json:"impacto"`    // por que importa para FPS
	Correcao   string `json:"correcao"`   // como resolver
	Acao       string `json:"acao"`       // "manual" | "regra:<id>" | "pagina:<nome>" | ""
}

// Diagnose roda todas as sondas e devolve os gargalos que impactam o FPS,
// junto com um resumo. Tudo via leitura nativa — nenhum dado e estimado.
func Diagnose(sid string) map[string]any {
	var g []Gargalo
	g = appendIf(g, checkXMP())
	g = appendIf(g, checkRefresh())
	g = append(g, checkGamesOnHDD(sid)...)
	g = appendIf(g, checkHAGS())
	g = appendIf(g, checkPowerPlan())
	g = appendIf(g, checkDiskSpace())
	g = appendIf(g, checkRAM())
	g = appendIf(g, checkStartupCount(sid))
	g = appendIf(g, checkDGPUNotebook())

	rank := map[string]int{"critico": 0, "atencao": 1, "info": 2, "bom": 3}
	// ordena por severidade (estavel o suficiente para a UI)
	for i := 1; i < len(g); i++ {
		for j := i; j > 0 && rank[g[j].Severidade] < rank[g[j-1].Severidade]; j-- {
			g[j], g[j-1] = g[j-1], g[j]
		}
	}

	cr, at, bo := 0, 0, 0
	for _, x := range g {
		switch x.Severidade {
		case "critico":
			cr++
		case "atencao":
			at++
		case "bom":
			bo++
		}
	}
	return map[string]any{
		"resumo":   map[string]any{"criticos": cr, "atencoes": at, "bons": bo, "total": len(g)},
		"gargalos": g,
	}
}

func appendIf(g []Gargalo, x *Gargalo) []Gargalo {
	if x != nil {
		g = append(g, *x)
	}
	return g
}

// ---- Checagens --------------------------------------------------------------

// checkXMP: RAM rodando abaixo da velocidade nominal (XMP/EXPO desligado na BIOS).
func checkXMP() *Gargalo {
	rated, configured, _ := smbiosInfo()
	if rated == 0 || configured == 0 {
		return nil
	}
	if float64(configured) < float64(rated)*0.97 {
		return &Gargalo{
			ID: "ram.xmp", Titulo: "XMP/EXPO desligado", Severidade: "critico",
			Detectado: fmt.Sprintf("RAM rodando a %d MHz, mas o módulo suporta %d MHz", configured, rated),
			Impacto:   "Memória lenta segura a CPU — em jogos CPU-bound (COD, Battlefield, Fortnite) isso custa 15–40% de FPS.",
			Correcao:  fmt.Sprintf("Entre na BIOS/UEFI e ative o perfil XMP (Intel) / EXPO ou DOCP (AMD) para subir a RAM a %d MHz.", rated),
			Acao:      "manual",
		}
	}
	sev, imp := "bom", "A RAM está na velocidade nominal — ótimo para jogos CPU-bound."
	if rated < 3000 {
		sev = "info"
		imp = "A RAM já roda na velocidade nominal; é um módulo mais lento, mas não há ganho fácil sem trocar o pente."
	}
	return &Gargalo{
		ID: "ram.xmp", Titulo: "Velocidade da RAM", Severidade: sev,
		Detectado: fmt.Sprintf("RAM a %d MHz (nominal %d MHz)", configured, rated),
		Impacto:   imp, Correcao: "Nada a fazer.", Acao: "",
	}
}

// checkRefresh: monitor não está na maior taxa de atualização que suporta.
func checkRefresh() *Gargalo {
	atual, max := displayRefresh()
	if atual == 0 || max == 0 {
		return nil
	}
	if atual < max-1 {
		return &Gargalo{
			ID: "display.refresh", Titulo: "Tela abaixo da taxa máxima", Severidade: "critico",
			Detectado: fmt.Sprintf("Monitor a %d Hz, mas suporta %d Hz", atual, max),
			Impacto:   fmt.Sprintf("Você está jogando a %d Hz num painel de %d Hz — joga fora quase metade da fluidez que o monitor entrega.", atual, max),
			Correcao:  fmt.Sprintf("Configurações → Sistema → Vídeo → Vídeo avançado → Taxa de atualização → %d Hz.", max),
			Acao:      "manual",
		}
	}
	sev := "bom"
	if max < 75 {
		sev = "info"
	}
	return &Gargalo{
		ID: "display.refresh", Titulo: "Taxa de atualização", Severidade: sev,
		Detectado: fmt.Sprintf("Monitor a %d Hz (máximo)", atual),
		Impacto:   "O monitor já está na taxa máxima.", Correcao: "Nada a fazer.", Acao: "",
	}
}

// checkGamesOnHDD: jogos instalados num HD mecânico (stutter e loadings longos).
func checkGamesOnHDD(sid string) []Gargalo {
	games := DetectGames(sid)
	if len(games) == 0 {
		return nil
	}
	media := diskMediaByNumber()
	var hddGames []string
	for _, gm := range games {
		if n, ok := pathDiskNumber(gm.Pasta); ok {
			if media[n] == "HDD" {
				hddGames = append(hddGames, gm.Nome)
			}
		}
	}
	if len(hddGames) == 0 {
		return nil
	}
	lista := strings.Join(hddGames, ", ")
	if len(hddGames) > 4 {
		lista = strings.Join(hddGames[:4], ", ") + fmt.Sprintf(" e mais %d", len(hddGames)-4)
	}
	return []Gargalo{{
		ID: "disk.game-on-hdd", Titulo: "Jogo num HD mecânico", Severidade: "atencao",
		Detectado: fmt.Sprintf("%d jogo(s) no HDD: %s", len(hddGames), lista),
		Impacto:   "HD mecânico causa engasgo ao carregar texturas (stutter) e tempos de loading muito maiores que um SSD.",
		Correcao:  "Mova o jogo para um SSD (no Steam: Propriedades → Arquivos locais → Mover pasta de instalação).",
		Acao:      "manual",
	}}
}

// checkHAGS: Agendamento de GPU por Hardware desligado.
func checkHAGS() *Gargalo {
	v, ok := ReadInteger("HKLM", "", false, `SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "HwSchMode")
	if !ok {
		return nil
	}
	if v != 2 {
		return &Gargalo{
			ID: "gpu.hags", Titulo: "Agendamento de GPU por Hardware (HAGS) desligado", Severidade: "atencao",
			Detectado: "HAGS está desativado",
			Impacto:   "O HAGS reduz a latência e pode melhorar o frametime/1% low em GPUs recentes.",
			Correcao:  "Ative o HAGS (precisa de reinício).",
			Acao:      "regra:win.gpu-scheduling-hags",
		}
	}
	return &Gargalo{
		ID: "gpu.hags", Titulo: "Agendamento de GPU por Hardware (HAGS)", Severidade: "bom",
		Detectado: "HAGS ativado", Impacto: "Latência de GPU otimizada.", Correcao: "Nada a fazer.", Acao: "",
	}
}

// checkPowerPlan: plano de energia não é de alto desempenho. Casa por NOME
// (não só GUID) porque o Desempenho Máximo/Ultimate ganha um GUID aleatório.
func checkPowerPlan() *Gargalo {
	guid := strings.ToLower(ActiveScheme())
	nome := ActiveSchemeName()
	if guid == "" {
		return nil
	}
	low := strings.ToLower(nome)
	high := "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
	ultimate := "e9a42b02-d5df-448d-aa00-03f14749eb61"
	bom := guid == high || guid == ultimate || strings.Contains(low, "performance") || strings.Contains(low, "desempenho")
	if bom {
		rotulo := nome
		if rotulo == "" {
			rotulo = "Alto Desempenho"
		}
		return &Gargalo{
			ID: "power.plan", Titulo: "Plano de energia", Severidade: "bom",
			Detectado: "Plano ativo: " + rotulo, Impacto: "CPU livre para manter o clock alto.", Correcao: "Nada a fazer.", Acao: "",
		}
	}
	return &Gargalo{
		ID: "power.plan", Titulo: "Plano de energia não é de alto desempenho", Severidade: "atencao",
		Detectado: "O plano ativo economiza energia (não é Alto Desempenho)",
		Impacto:   "Planos equilibrado/econômico deixam a CPU baixar o clock, causando quedas de FPS e stutter.",
		Correcao:  "Ative o plano Alto Desempenho.",
		Acao:      "regra:power.high-performance",
	}
}

// checkDiskSpace: disco do sistema quase cheio.
func checkDiskSpace() *Gargalo {
	sysDrive := "C:"
	if v := os.Getenv("SystemDrive"); v != "" {
		sysDrive = v
	}
	for _, d := range diskSpaces() {
		if !strings.EqualFold(d.Letra, sysDrive) {
			continue
		}
		freePct := 100 - d.UsadoPct
		if freePct < 12 {
			return &Gargalo{
				ID: "disk.space", Titulo: "Disco do sistema quase cheio", Severidade: "atencao",
				Detectado: fmt.Sprintf("Só %d%% livre em %s (%.0f GB)", freePct, d.Letra, d.LivreGB),
				Impacto:   "Disco do sistema cheio deixa o Windows e os jogos lentos (sem espaço para cache/pagefile/shaders).",
				Correcao:  "Libere espaço com a Limpeza (temporários, cache, Windows Update).",
				Acao:      "pagina:limpeza",
			}
		}
		return &Gargalo{
			ID: "disk.space", Titulo: "Espaço no disco do sistema", Severidade: "bom",
			Detectado: fmt.Sprintf("%d%% livre em %s", freePct, d.Letra), Impacto: "Espaço saudável.", Correcao: "Nada a fazer.", Acao: "",
		}
	}
	return nil
}

// checkRAM: pouca memória para jogos modernos.
func checkRAM() *Gargalo {
	gb := totalRAMGB()
	if gb == 0 {
		return nil
	}
	if gb < 16 {
		return &Gargalo{
			ID: "ram.total", Titulo: "Pouca memória RAM", Severidade: "atencao",
			Detectado: fmt.Sprintf("%d GB de RAM", gb),
			Impacto:   "Jogos modernos pedem 16 GB; com menos, o sistema usa o disco (pagefile) e gera stutter.",
			Correcao:  "Considere adicionar mais memória (idealmente em dual channel).",
			Acao:      "manual",
		}
	}
	return &Gargalo{
		ID: "ram.total", Titulo: "Memória RAM", Severidade: "bom",
		Detectado: fmt.Sprintf("%d GB de RAM", gb), Impacto: "Memória suficiente para jogos.", Correcao: "Nada a fazer.", Acao: "",
	}
}

// checkStartupCount: muitos programas subindo com o Windows.
func checkStartupCount(sid string) *Gargalo {
	n := 0
	for _, s := range ListStartup(sid) {
		if s.Enabled {
			n++
		}
	}
	if n > 8 {
		return &Gargalo{
			ID: "startup.count", Titulo: "Muitos programas na inicialização", Severidade: "atencao",
			Detectado: fmt.Sprintf("%d programas sobem com o Windows", n),
			Impacto:   "Programas em segundo plano comem CPU, RAM e disco — atrapalham o boot e roubam recursos do jogo.",
			Correcao:  "Desligue o que não precisa subir junto com o Windows.",
			Acao:      "pagina:inicializacao",
		}
	}
	return nil
}

// checkDGPUNotebook: notebook com GPU dedicada — lembrar de usá-la nos jogos.
func checkDGPUNotebook() *Gargalo {
	_, _, laptop := smbiosInfo()
	if !laptop {
		return nil
	}
	hasDGPU := false
	for _, g := range GPUs() {
		if g.Vendor == "NVIDIA" || g.Vendor == "AMD" {
			hasDGPU = true
		}
	}
	if !hasDGPU {
		return nil
	}
	return &Gargalo{
		ID: "gpu.dedicated", Titulo: "Garanta a GPU dedicada nos jogos", Severidade: "info",
		Detectado: "Notebook com GPU dedicada + gráficos integrados",
		Impacto:   "Se o jogo rodar na GPU integrada por engano, o FPS despenca.",
		Correcao:  "Defina a preferência de GPU 'Alto desempenho' por jogo.",
		Acao:      "pagina:jogos",
	}
}

// ---- Helpers de disco -------------------------------------------------------

// diskMediaByNumber mapeia numero do PhysicalDrive -> "SSD"/"HDD".
func diskMediaByNumber() map[int]string {
	m := map[int]string{}
	for _, d := range diskTypes() {
		if n, ok := d["numero"].(int); ok {
			if media, ok := d["midia"].(string); ok {
				m[n] = media
			}
		}
	}
	return m
}

// pathDiskNumber devolve o numero do PhysicalDrive que hospeda um caminho.
func pathDiskNumber(p string) (int, bool) {
	if len(p) < 2 || p[1] != ':' {
		return 0, false
	}
	drive := p[:2]
	return volumeDiskNumber(drive)
}

// volumeDiskNumber: numero do PhysicalDrive de um volume (ex.: "D:"). -1/false se falhar.
func volumeDiskNumber(drive string) (int, bool) {
	p, err := windows.UTF16PtrFromString(`\\.\` + drive)
	if err != nil {
		return 0, false
	}
	h, err := windows.CreateFile(p, 0, fileShareReadWrite, nil, openExisting, 0, 0)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	buf := make([]byte, 16+24*8)
	var br uint32
	if err := windows.DeviceIoControl(h, ioctlVolumeDiskExtents, nil, 0, &buf[0], uint32(len(buf)), &br, nil); err != nil {
		return 0, false
	}
	count := uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	if count == 0 {
		return 0, false
	}
	diskNum := uint32(buf[8]) | uint32(buf[9])<<8 | uint32(buf[10])<<16 | uint32(buf[11])<<24
	return int(diskNum), true
}
