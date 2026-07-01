//go:build windows

package winutil

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Busca e instala drivers via Windows Update Agent (WUA) — a mesma engine que
// o próprio Windows usa em "Configurações > Windows Update > Atualizações
// opcionais > Atualizações de driver". Cobre qualquer classe de dispositivo
// (GPU, rede, áudio, chipset, USB...), não só GPU, e a versão/tamanho vem
// direto do catálogo da Microsoft — dado real, não estimado por data.
//
// Todo objeto COM da WUA vive numa apartment de thread único (STA): a
// goroutine que cria a sessão precisa ficar no MESMO thread do SO do início
// ao fim (runtime.LockOSThread), então cada operação roda self-contained na
// sua própria goroutine travada — não dá pra guardar objetos COM entre
// chamadas HTTP diferentes.

// DriverUpdateInfo descreve um update de driver pendente encontrado pela WUA.
type DriverUpdateInfo struct {
	ID          string  `json:"id"`
	Titulo      string  `json:"titulo"`
	Descricao   string  `json:"descricao"`
	TamanhoMB   float64 `json:"tamanho_mb"`
}

type driverUpdMgr struct {
	mu           sync.Mutex
	state        string // idle | buscando | pronto | baixando | instalando | concluido | erro
	updates      []DriverUpdateInfo
	errMsg       string
	rebootNeeded bool
	started      time.Time
}

var driverUpd = &driverUpdMgr{state: "idle"}

// DriverUpdMgr expõe o gerenciador singleton pro server chamar.
func DriverUpdMgr() *driverUpdMgr { return driverUpd }

// StartSearch dispara a busca de drivers pendentes em background.
func (m *driverUpdMgr) StartSearch() error {
	m.mu.Lock()
	if m.state == "buscando" || m.state == "baixando" || m.state == "instalando" {
		m.mu.Unlock()
		return fmt.Errorf("já tem uma operação de driver em andamento")
	}
	m.state = "buscando"
	m.updates = nil
	m.errMsg = ""
	m.started = time.Now()
	m.mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil {
			defer ole.CoUninitialize()
		}
		updates, err := wuaSearchDriverUpdates()
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.state = "erro"
			m.errMsg = err.Error()
			return
		}
		m.updates = updates
		m.state = "pronto"
	}()
	return nil
}

// StartInstall baixa e instala os updates selecionados (por ID) em background.
func (m *driverUpdMgr) StartInstall(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("nenhum driver selecionado")
	}
	m.mu.Lock()
	if m.state == "buscando" || m.state == "baixando" || m.state == "instalando" {
		m.mu.Unlock()
		return fmt.Errorf("já tem uma operação de driver em andamento")
	}
	m.state = "baixando"
	m.errMsg = ""
	m.rebootNeeded = false
	m.started = time.Now()
	m.mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil {
			defer ole.CoUninitialize()
		}
		m.mu.Lock()
		m.state = "instalando"
		m.mu.Unlock()
		reboot, err := wuaInstallDriverUpdates(ids)
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.state = "erro"
			m.errMsg = err.Error()
			return
		}
		m.rebootNeeded = reboot
		m.state = "concluido"
	}()
	return nil
}

// Status devolve o andamento pra UI consultar via poll.
func (m *driverUpdMgr) Status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	resp := map[string]any{
		"estado":         m.state,
		"updates":        m.updates,
		"reboot_necessario": m.rebootNeeded,
	}
	if m.state == "erro" {
		resp["erro"] = m.errMsg
	}
	return resp
}

/* ---- COM / Windows Update Agent ------------------------------------------ */

// wuaSession abre Microsoft.Update.Session e devolve o IDispatch (chamador
// controla o Release). Precisa rodar já dentro de uma apartment STA inicializada.
func wuaSession() (*ole.IDispatch, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.Session")
	if err != nil {
		return nil, fmt.Errorf("Windows Update indisponível: %w", err)
	}
	defer unknown.Release()
	session, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("Windows Update indisponível: %w", err)
	}
	return session, nil
}

// wuaSearchDriverUpdates roda a busca de drivers pendentes (não instalados,
// não ocultos) via IUpdateSearcher — o mesmo critério do painel "Atualizações
// opcionais" do Windows.
func wuaSearchDriverUpdates() ([]DriverUpdateInfo, error) {
	session, err := wuaSession()
	if err != nil {
		return nil, err
	}
	defer session.Release()

	searcherRaw, err := oleutil.CallMethod(session, "CreateUpdateSearcher")
	if err != nil {
		return nil, fmt.Errorf("não consegui iniciar a busca: %w", err)
	}
	searcher := searcherRaw.ToIDispatch()
	defer searcher.Release()

	resultRaw, err := oleutil.CallMethod(searcher, "Search", "IsInstalled=0 and Type='Driver' and IsHidden=0")
	if err != nil {
		return nil, fmt.Errorf("busca do Windows Update falhou (precisa do serviço wuauserv ativo): %w", err)
	}
	result := resultRaw.ToIDispatch()
	defer result.Release()

	updates, release, err := wuaResultUpdates(result)
	if err != nil {
		return nil, err
	}
	defer release()

	count, err := wuaCount(updates)
	if err != nil {
		return nil, err
	}

	out := make([]DriverUpdateInfo, 0, count)
	for i := 0; i < count; i++ {
		item, err := wuaItem(updates, i)
		if err != nil {
			continue
		}
		info := DriverUpdateInfo{
			Titulo:    propStr(item, "Title"),
			Descricao: propStr(item, "Description"),
		}
		if idDisp, err := propDisp(item, "Identity"); err == nil {
			info.ID = propStr(idDisp, "UpdateID")
			idDisp.Release()
		}
		if size, err := oleutil.GetProperty(item, "MaxDownloadSize"); err == nil {
			info.TamanhoMB = float64(size.Val) / 1048576
		}
		item.Release()
		if info.ID != "" {
			out = append(out, info)
		}
	}
	return out, nil
}

// wuaInstallDriverUpdates refaz a busca (objetos COM não sobrevivem entre
// chamadas HTTP), filtra pelos IDs pedidos, aceita EULA se preciso, baixa e
// instala. Devolve se o Windows pede reinício pra concluir.
func wuaInstallDriverUpdates(ids []string) (rebootRequired bool, err error) {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}

	session, err := wuaSession()
	if err != nil {
		return false, err
	}
	defer session.Release()

	searcherRaw, err := oleutil.CallMethod(session, "CreateUpdateSearcher")
	if err != nil {
		return false, fmt.Errorf("não consegui iniciar a busca: %w", err)
	}
	searcher := searcherRaw.ToIDispatch()
	defer searcher.Release()

	resultRaw, err := oleutil.CallMethod(searcher, "Search", "IsInstalled=0 and Type='Driver' and IsHidden=0")
	if err != nil {
		return false, fmt.Errorf("busca do Windows Update falhou: %w", err)
	}
	result := resultRaw.ToIDispatch()
	defer result.Release()

	updates, release, err := wuaResultUpdates(result)
	if err != nil {
		return false, err
	}
	defer release()

	count, err := wuaCount(updates)
	if err != nil {
		return false, err
	}

	collUnknown, err := oleutil.CreateObject("Microsoft.Update.UpdateColl")
	if err != nil {
		return false, fmt.Errorf("falha ao montar lista de instalação: %w", err)
	}
	defer collUnknown.Release()
	coll, err := collUnknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return false, err
	}
	defer coll.Release()

	matched := 0
	for i := 0; i < count; i++ {
		item, err := wuaItem(updates, i)
		if err != nil {
			continue
		}
		idDisp, err := propDisp(item, "Identity")
		if err != nil {
			item.Release()
			continue
		}
		updID := propStr(idDisp, "UpdateID")
		idDisp.Release()
		if !wanted[updID] {
			item.Release()
			continue
		}
		// Driver updates às vezes exigem aceitar a EULA antes de baixar.
		if eula, err := oleutil.GetProperty(item, "EulaAccepted"); err == nil {
			if b, ok := eula.Value().(bool); ok && !b {
				oleutil.CallMethod(item, "AcceptEula")
			}
		}
		oleutil.CallMethod(coll, "Add", item)
		matched++
		item.Release()
	}
	if matched == 0 {
		return false, fmt.Errorf("os drivers selecionados não estão mais disponíveis (rode a busca de novo)")
	}

	downloaderRaw, err := oleutil.CallMethod(session, "CreateUpdateDownloader")
	if err != nil {
		return false, fmt.Errorf("falha ao preparar o download: %w", err)
	}
	downloader := downloaderRaw.ToIDispatch()
	defer downloader.Release()
	if _, err := oleutil.PutProperty(downloader, "Updates", coll); err != nil {
		return false, err
	}
	if _, err := oleutil.CallMethod(downloader, "Download"); err != nil {
		return false, fmt.Errorf("download falhou: %w", err)
	}

	installerRaw, err := oleutil.CallMethod(session, "CreateUpdateInstaller")
	if err != nil {
		return false, fmt.Errorf("falha ao preparar a instalação: %w", err)
	}
	installer := installerRaw.ToIDispatch()
	defer installer.Release()
	if _, err := oleutil.PutProperty(installer, "Updates", coll); err != nil {
		return false, err
	}
	instResultRaw, err := oleutil.CallMethod(installer, "Install")
	if err != nil {
		return false, fmt.Errorf("instalação falhou (talvez precise rodar como administrador): %w", err)
	}
	instResult := instResultRaw.ToIDispatch()
	defer instResult.Release()

	reboot := false
	if v, err := oleutil.GetProperty(instResult, "RebootRequired"); err == nil {
		if b, ok := v.Value().(bool); ok {
			reboot = b
		}
	}
	return reboot, nil
}

/* ---- helpers COM ----------------------------------------------------------- */

func wuaResultUpdates(searchResult *ole.IDispatch) (updates *ole.IDispatch, release func(), err error) {
	updatesRaw, err := oleutil.GetProperty(searchResult, "Updates")
	if err != nil {
		return nil, nil, fmt.Errorf("resultado da busca inválido: %w", err)
	}
	updates = updatesRaw.ToIDispatch()
	return updates, func() { updates.Release() }, nil
}

func wuaCount(coll *ole.IDispatch) (int, error) {
	c, err := oleutil.GetProperty(coll, "Count")
	if err != nil {
		return 0, err
	}
	return int(c.Val), nil
}

func wuaItem(coll *ole.IDispatch, i int) (*ole.IDispatch, error) {
	raw, err := oleutil.CallMethod(coll, "Item", i)
	if err != nil {
		return nil, err
	}
	return raw.ToIDispatch(), nil
}

func propStr(disp *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(disp, name)
	if err != nil || v == nil {
		return ""
	}
	return v.ToString()
}

func propDisp(disp *ole.IDispatch, name string) (*ole.IDispatch, error) {
	v, err := oleutil.GetProperty(disp, name)
	if err != nil {
		return nil, err
	}
	d := v.ToIDispatch()
	if d == nil {
		return nil, fmt.Errorf("propriedade %s não é um objeto", name)
	}
	return d, nil
}
