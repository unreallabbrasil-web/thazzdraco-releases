# Arquitetura (v4.0 — Go nativo)

O ThazzDraco Optimizer é **um único executável Go**, portátil e sem dependências, que embute a UI
web e as regras, sobe um servidor HTTP local e abre uma janela do navegador em modo app. Tudo fala
**direto com a API do Windows** (Win32) — sem WMI/PowerShell, exceto uma função pontual do Defender.

> A versão **v3.0** (assistente PowerShell + `.bat` + WPF) está aposentada em `legacy/`. Este
> documento descreve a arquitetura **atual**.

---

## 1. Visão em camadas

```
┌──────────────────────────────────────────────────────────────┐
│  UI  (web/ — HTML/CSS/JS, "DRACO // PORTAL", 100% offline)    │
│  5 nós (ler → agir → registrar):                              │
│  Núcleo · Sessão · Máquina · Ações · Registro                 │
└───────────────┬──────────────────────────────────────────────┘
                │  fetch JSON (localhost:porta-efêmera)
┌───────────────▼──────────────────────────────────────────────┐
│  internal/server  — HTTP + API JSON + watchdog de janela      │
└───────────────┬──────────────────────────────────────────────┘
                │
┌───────────────▼────────────────┐
│  internal/sessao — trilho do    │  ordena o atendimento; guarda
│  atendimento (7 etapas + estado)│  o estado em disco. Fica POR
└───────────────┬────────────────┘  CIMA do motor, nunca dentro.
                │
┌───────────────▼───────────────┐   ┌──────────────────────────┐
│  internal/engine               │   │  internal/winutil        │
│  motor declarativo:            │──▶│  API Win32 pura:         │
│  rules.json (go:embed) →       │   │  registro, serviços,     │
│  detect → estado → apply/undo  │   │  powercfg, restore,      │
│  + backup/restore + log        │   │  perfil(SMBIOS), métricas│
└────────────────────────────────┘   │  saúde(SMART), GPU(NVML),│
                                      │  inicialização, jogos    │
┌─────────────────────────────────────────────────────────────┐
│  main.go — elevação UAC (manifesto), bootstrap, janela Edge  │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. `main.go` — bootstrap

- **Elevação**: o `.exe` embute um manifesto `requireAdministrator` (escudo UAC) gerado por
  `resgen.py` → `resource.syso`. Há também `RelaunchElevated()` (ShellExecute "runas") como fallback.
- **Servidor**: escuta em `127.0.0.1:0` (porta efêmera) e serve a UI embutida + a API.
- **Janela**: abre o **Edge em modo app** (`msedge --app=http://127.0.0.1:porta`) com `--user-data-dir`
  isolado (janela standalone, sem abas). Fallback: navegador padrão. Tamanho inicial **1380×940**.
- **Watchdog**: a UI faz `ping` a cada 4s; se ficar **>90s sem sinal de vida** (janela fechada), o
  processo encerra sozinho. **Robusto**: qualquer request marca atividade (não só o ping, que o
  navegador estrangula quando a janela perde o foco) e ele **nunca encerra com requisição em andamento**
  (`inFlight>0`) — senão mataria backup/reparo/benchmark no meio (era a causa do "Failed to fetch"). No
  `-headless` o watchdog **não** roda.
- **Modos CLI** (debug): `-profile`, `-scan`, `-headless -port N`.

---

## 3. `internal/winutil` — chão de fábrica (Win32 puro)

| Arquivo | Responsabilidade |
|---|---|
| `admin.go` | `IsAdmin` (token elevado), `RelaunchElevated` |
| `sid.go` | SID do usuário interativo real (dono do `explorer.exe`) — p/ HKCU correto quando elevado |
| `registry.go` | snapshot / read / write / remove / binário / enumerar; resolve HKLM/HKCU/HKEY_USERS\<SID> |
| `service.go` | tipo de inicialização de serviços (svc/mgr) |
| `powercfg.go` | esquema ativo + valores AC (via `powercfg.exe`, sem janela) |
| `restore.go` | ponto de restauração via `SRSetRestorePoint` (srclient.dll) |
| `profile.go` | perfil: RAM total (GlobalMemoryStatusEx), RAM speed/chassi (**SMBIOS**), bateria, refresh (EnumDisplaySettings), SSD/HDD (IOCTL seek-penalty, disco do sistema 1º), DNS (GetAdaptersAddresses) |
| `metrics.go` | CPU% (GetSystemTimes), RAM, top processos (Toolhelp+psapi), uptime |
| `health.go` | espaço por disco, **S.M.A.R.T.** (IOCTL predict-failure), bateria, uptime |
| `gpu.go` | **GPU NVIDIA via NVML** (temp/uso/VRAM real); nome universal (EnumDisplayDevices); breakdown de memória (GetPerformanceInfo). AMD via ADL: a validar |
| `startup.go` | inicialização: lista Run (HKCU/HKLM/Wow64) + pastas Startup; liga/desliga via `StartupApproved` |
| `games.go` | detecta Steam (libraryfolders.vdf + appmanifest) e Epic (manifests); acha o exe principal; tweaks por jogo (FSO/GPU/IFEO) + exclusão do Defender |
| `disco.go` | explorador de espaço: caminhada paralela com **fila limitada** (`sync.Cond`), tamanho somado por pasta + maiores arquivos; não segue junções |

**Princípio:** nada de WMI/PowerShell. A **única** exceção é a exclusão do Defender
(`Add/Remove-MpPreference`), porque não há API nativa limpa — e roda com janela oculta.

---

## 4. `internal/engine` — motor declarativo

- **`model.go`** — structs das regras + `go:embed` de `assets/rules.json` (61 regras) e
  `assets/presets.json`.
- **`gate.go`** — avaliação de `hardware_gate` (campos do perfil, ops `==`/`<`/`>=`/`contem`…).
- **`detect.go`** — detecção por tipo: `registry`, `registry-foreach`, `powercfg`,
  `powercfg-setting`, `service`, `cleanup-size`, `profile-compara`, `dns-check`.
- **`scan.go`** — varre todas as regras → `ScanResult` (score, totais, perfil, regras + campo
  `tecnico` com o que cada uma altera).
- **`apply.go`** — aplica com **snapshot por valor** (undo exato), ponto de restauração no início,
  histórico de lotes em `%ProgramData%\ThazzDraco\historico.json`, **log** em `operacoes.log`.
  Limpeza usa o TEMP do **usuário real** (não do admin).
- **`backup.go`** — `BuildBackup` fotografa registro/serviços/powercfg/inicialização →
  `%ProgramData%\ThazzDraco\backups\<id>.json`; `RestoreBackup` devolve tudo.

**Reversibilidade:** registro = regrava o valor antigo (ou apaga se não existia); serviço = restaura
o start type; powercfg = restaura esquema + valores AC; inicialização = restaura `StartupApproved`.

---

## 4.1 `internal/sessao` — o trilho do atendimento

Camada **acima** do motor (importa `engine`/`winutil`, nunca o contrário). Dá ordem ao que antes eram
telas soltas: 7 etapas (Medir · Ler · Entender · Planejar · Aplicar · Provar · Entregar), mais uma
etapa 0 automática que confere elevação e decide se a sessão nasce completa ou **Limitada** (sem
admin = só as etapas de leitura).

| Arquivo | Responsabilidade |
|---|---|
| `model.go` | Structs + catálogo de etapas + saneamento do texto vindo do usuário |
| `maquina.go` | Transições: concluir, pular (com **motivo obrigatório**), encerrar |
| `plano.go` | `GerarPlano` — **função pura**: varredura + gargalos + perfil → fila em 3 fases, com seleção padrão e score projetado |
| `executor.go` | Máquina da execução: o que entra na fila, o julgamento de cada escrita (**`aplicado` só depois de reler**) e a reconciliação de execução órfã |
| `evidencia.go` | Medições de antes/depois + **cenário**: só compara jogo e resolução idênticos; o resto vira motivo escrito |
| `reboot.go` | Continuidade através do reinício: marca a intenção antes de reiniciar e reconhece a volta **pelo uptime** (sem auto-start) |
| `store.go` | `sessoes\<id>.json` + ponteiro `sessao-atual.txt`, escrita atômica, id validado por regex |
| `maquina_test.go` · `plano_test.go` | Testes da máquina de etapas e do planejador |

**Não duplica a rede de segurança:** snapshot, undo e histórico continuam no `engine`. A sessão só
guarda a ordem e o estado — o item aplicado guarda o `batch_id` que o liga ao lote do histórico.

A execução de uma fase roda em goroutine (`internal/server/sessao_exec.go`) segurando `s.mu`, o mesmo
cadeado de `/api/aplicar` — nunca há duas escritas simultâneas. `/api/sessao/status` é o único
handler de sessão que **não** pega `s.mu`, porque precisa responder o poll enquanto a fase corre.

Desenho completo e fatias seguintes em [SESSAO](SESSAO.md).

---

## 5. `internal/server` — API JSON

Endpoints principais: `/api/escanear`, `/api/aplicar`, `/api/aplicar-verdes`, `/api/aplicar-preset`,
`/api/desfazer`, `/api/historico`, `/api/presets`, `/api/saude`, `/api/metricas`,
`/api/inicializacao` (+`/set`), `/api/jogos` (+`/tweak`), `/api/backup/*`, `/api/reiniciar`,
`/api/ping`, `/api/info`, além de `/api/diagnostico`, `/api/driver` (+`/limpar`), `/api/reparo/*`,
`/api/debloat/*`, `/api/limpeza-profunda/*`, `/api/ferramentas/*`, `/api/fps/*`, `/api/benchmark` e
`/api/sessao*` (trilho do atendimento — rotas que mudam estado exigem **POST**).
Operações do motor são serializadas por `s.mu`; as destrutivas pesadas (debloat/limpeza/driver) por
`s.opMu`. Um middleware (`instrument`) conta requisições em andamento (`inFlight`) e marca atividade a
cada chamada — base do watchdog robusto. Guard de **origem** (`Sec-Fetch-Site`/`Origin`) fecha CSRF.

---

## 6. `web/` — UI "DRACO // PORTAL"

HTML/CSS/JS vanilla, **fontes nativas do Windows** (Bahnschrift/Consolas → 100% offline). Estética
cinematográfica (portal rúnico + dragão + raios + atmosfera), navegação por **HUD de comando** no
rodapé.

**Navegação (5 nós).** A tabela `NOS` em `app.js` é a fonte única: monta o HUD, monta a barra de
sub-abas e diz onde cada destino mora. É camada de apresentação — por baixo continuam `showPage`
(página) e `showSub` (pane), e `navTo(nome-lógico)` é o contrato dos deep-links, que não muda quando
as telas se movem. Uma aba pode empilhar panes irmãos (`extras`), que foi como *Limpar* e *Reparar*
juntaram telas que faziam a mesma coisa. `app.js` orquestra: roteamento, **cerimônia de scan** (varredura profunda animada), toggles,
modais (detalhes/consentimento/backup), relatório, modo performance, responsivo por altura. Ícones
em `icons.js`. A varredura **não** é automática — o usuário clica em "Escanear PC".

---

## 7. Build & empacotamento

- Toolchain **Go portátil** em `E:\CLAUDE AI\_toolchain` (wrapper `gob.sh`; `GOPATH`/`GOCACHE` locais).
- `resgen.py` gera `resource.syso` (COFF com ícone do dragão + manifesto admin) **sem** windres/rsrc.
- `build.ps1`: `go build -ldflags="-H windowsgui -s -w"` → `dist/ThazzDraco.exe`.

---

## 8. Notas

- Houve trabalho em **paralelo** (outra sessão) que implementou métricas/inicialização/relatório —
  reconciliado no `server.go`/`winutil`.
- Bugs conhecidos: a captura de tela do preview headless trava com as animações pesadas (não afeta o
  app real — validação feita por `eval`). Temperatura de **CPU** não é exposta de forma confiável sem
  driver de terceiro → mostrada como N/A de propósito.
