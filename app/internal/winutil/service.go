//go:build windows

package winutil

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// StartTypeName traduz o codigo de inicializacao do servico para texto amigavel.
func StartTypeName(st uint32) string {
	switch st {
	case windows.SERVICE_BOOT_START:
		return "Boot"
	case windows.SERVICE_SYSTEM_START:
		return "System"
	case windows.SERVICE_AUTO_START:
		return "Automatic"
	case windows.SERVICE_DEMAND_START:
		return "Manual"
	case windows.SERVICE_DISABLED:
		return "Disabled"
	}
	return "Unknown"
}

// ServiceStartType retorna o tipo de inicializacao atual e se o servico existe.
func ServiceStartType(name string) (uint32, bool) {
	m, err := mgr.Connect()
	if err != nil {
		return 0, false
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return 0, false // servico ausente nesta maquina
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return 0, false
	}
	return cfg.StartType, true
}

// SetServiceStartType altera o tipo de inicializacao (ex.: Disabled). Mantem o
// resto da configuracao intacto. Requer elevacao.
func SetServiceStartType(name string, startType uint32) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	cfg.StartType = startType
	return s.UpdateConfig(cfg)
}

// StartTypeFromName converte texto ("Disabled", "Manual"...) para o codigo Win32.
func StartTypeFromName(name string) uint32 {
	switch name {
	case "Disabled":
		return windows.SERVICE_DISABLED
	case "Manual":
		return windows.SERVICE_DEMAND_START
	case "Automatic":
		return windows.SERVICE_AUTO_START
	case "Boot":
		return windows.SERVICE_BOOT_START
	case "System":
		return windows.SERVICE_SYSTEM_START
	}
	return windows.SERVICE_DEMAND_START
}

// ---- F11: Scanner de Serviços Pesados ---------------------------------------

// ServiceInfo descreve um serviço não-essencial para jogos.
type ServiceInfo struct {
	Nome    string `json:"nome"`
	Exibir  string `json:"exibir"`
	Cat     string `json:"cat"`
	Impacto string `json:"impacto"` // Alto | Médio | Baixo
	Desc    string `json:"desc"`
	Rodando bool   `json:"rodando"`
}

// candidatosServicos: serviços conhecidos por consumir recursos sem benefício direto para jogos.
var candidatosServicos = []ServiceInfo{
	{Nome: "SysMain", Exibir: "SysMain (Superfetch)", Cat: "Memória", Impacto: "Alto", Desc: "Pré-carrega apps em RAM e causa alto I/O de disco; desnecessário com SSD NVMe."},
	{Nome: "WSearch", Exibir: "Windows Search", Cat: "Indexação", Impacto: "Alto", Desc: "Indexa arquivos em segundo plano com alto I/O; pode ser parado durante sessões de jogo."},
	{Nome: "wuauserv", Exibir: "Windows Update", Cat: "Atualização", Impacto: "Alto", Desc: "Baixa atualizações — consome rede e CPU; para temporariamente durante jogos competitivos."},
	{Nome: "spooler", Exibir: "Print Spooler", Cat: "Impressão", Impacto: "Médio", Desc: "Gerencia a fila de impressão. Desnecessário se não há impressora em uso."},
	{Nome: "DiagTrack", Exibir: "Connected User Experiences (Telemetria)", Cat: "Telemetria", Impacto: "Médio", Desc: "Envia dados de uso e diagnóstico à Microsoft em segundo plano."},
	{Nome: "bits", Exibir: "BITS (Background Intelligent Transfer)", Cat: "Atualização", Impacto: "Médio", Desc: "Serviço de download em segundo plano usado pelo Windows Update e outros."},
	{Nome: "RemoteRegistry", Exibir: "Remote Registry", Cat: "Segurança", Impacto: "Baixo", Desc: "Permite que processos remotos leiam o registro. Recomendado manter parado."},
	{Nome: "XblAuthManager", Exibir: "Xbox Live Auth Manager", Cat: "Xbox", Impacto: "Baixo", Desc: "Autenticação Xbox Live — desnecessário se não usa Xbox ou Game Pass."},
	{Nome: "XblGameSave", Exibir: "Xbox Live Game Save", Cat: "Xbox", Impacto: "Baixo", Desc: "Sincroniza saves com Xbox Cloud — desnecessário fora do ecossistema Xbox."},
	{Nome: "XboxGipSvc", Exibir: "Xbox Accessory Management", Cat: "Xbox", Impacto: "Baixo", Desc: "Gerencia firmware de acessórios Xbox — desnecessário se não usa controles Xbox."},
	{Nome: "Fax", Exibir: "Fax", Cat: "Legado", Impacto: "Baixo", Desc: "Serviço de fax — raramente instalado ou utilizado em PCs modernos."},
}

// HeavyServices retorna os serviços não-essenciais presentes na máquina com estado rodando/parado.
func HeavyServices() []ServiceInfo {
	m, err := mgr.Connect()
	if err != nil {
		return []ServiceInfo{}
	}
	defer m.Disconnect()
	var out []ServiceInfo
	for _, cand := range candidatosServicos {
		s, err := m.OpenService(cand.Nome)
		if err != nil {
			continue // serviço não existe nesta máquina
		}
		st, qErr := s.Query()
		s.Close()
		info := cand
		if qErr == nil {
			info.Rodando = st.State == svc.Running
		}
		out = append(out, info)
	}
	if out == nil {
		return []ServiceInfo{}
	}
	return out
}

// StopServiceNow para um serviço (sem desabilitar permanentemente). Requer elevação.
func StopServiceNow(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	return err
}
