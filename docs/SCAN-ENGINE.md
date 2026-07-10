# Motor de Varredura (Scan Engine)

> **Status v4.0:** implementado em **Go nativo** (`internal/winutil/profile.go` para o perfil de
> hardware via Win32/SMBIOS, e `internal/engine/scan.go` + `detect.go` para a detecção das 41
> regras). O perfil é montado em memória (`Build-Profile` → `winutil.BuildProfile()`) e devolvido
> junto da varredura em `/api/escanear` (não há mais `system-profile.json` em arquivo). Os conceitos
> abaixo (read-only, detecção de contexto, gates) seguem válidos.

Especificação do **scanner read-only** que lê as configurações do PC e produz o perfil — base para
o motor de recomendação.

> **Regra de ouro:** o scanner **nunca escreve nada**. Ele só lê. Toda decisão e ação acontecem
> em camadas posteriores (ver [ARCHITECTURE.md](ARCHITECTURE.md)).

---

## Objetivo

Responder, para qualquer PC: *"o que existe aqui e em que estado está?"* — com profundidade
suficiente para que o motor de regras decida o que pode ser otimizado, sem aplicar nada às cegas.

## Domínios de varredura

Cada domínio lista **o que coletar** e a **fonte** sugerida (PowerShell/WMI/CIM/registro/CLI).
Os domínios marcados com 🆕 ainda não existem no `coletar.ps1`/`metricas.ps1` atuais.

### 1. Hardware
- CPU: modelo, núcleos, threads, clock base/atual, virtualização. — `Win32_Processor`
- RAM: total, por pente (slot, capacidade, clock, tipo DDR, fabricante). — `Win32_PhysicalMemory`
- GPU: modelo, VRAM, fabricante (NVIDIA/AMD/Intel), versão do driver. — `Win32_VideoController`
- Placa-mãe / BIOS: fabricante, versão, modo (UEFI/Legacy), Secure Boot. 🆕 — `Win32_BIOS`, `Confirm-SecureBootUEFI`
- 🆕 **Chassi**: detectar **notebook vs desktop**. — `Win32_SystemEnclosure.ChassisTypes`

### 2. Sistema operacional
- Edição, build, arquitetura, data de instalação, uptime, último boot. — `Win32_OperatingSystem`
- 🆕 Idioma/região, fuso, status de ativação.

### 3. Serviços 🆕 (hoje só checa 13)
- **Todos** os serviços: nome, display, status, tipo de inicialização, conta. — `Get-Service` / `Win32_Service`
- Classificar contra uma base de conhecimento: "seguro desativar", "depende do uso", "não mexer".

### 4. Inicialização (startup)
- Apps de startup: registro `Run`/`RunOnce` (HKCU+HKLM), pasta Startup, itens via `Win32_StartupCommand`.
- 🆕 **Tarefas agendadas** relevantes (atualizadores, telemetria). — `Get-ScheduledTask`

### 5. Drivers 🆕
- Lista de drivers, versão e data; sinalizar drivers antigos. — `Get-CimInstance Win32_PnPSignedDriver`

### 6. Energia
- Plano ativo e planos disponíveis, hibernação, throttle min/max da CPU. — `powercfg`
- 🆕 Em notebook: estado da bateria e se está na tomada (muda recomendações).

### 7. Rede
- Adaptadores ativos: nome, tipo (Ethernet/Wi-Fi), IP, MAC, velocidade. — `Win32_NetworkAdapter[Configuration]`
- Estado do Nagle por interface, parâmetros TCP, config do `netsh int tcp`.
- 🆕 DNS configurado, latência básica (ping a um alvo conhecido).

### 8. Registro (estado dos tweaks)
- Ler o valor atual de cada chave do catálogo ([OPTIMIZATIONS.md](OPTIMIZATIONS.md)) para saber o
  que já está otimizado e o que está pendente. Já é parcialmente feito por `coletar.ps1`.

### 9. Aplicativos e bloatware 🆕
- Programas instalados (registro `Uninstall`, `Get-Package`), apps da Store/UWP. — `Get-AppxPackage`
- Sinalizar bloatware OEM e apps redundantes.

### 10. Discos e armazenamento
- Por volume: total, livre, uso%. — `Win32_LogicalDisk`
- Tipo **SSD vs HDD** de forma confiável. — `Get-PhysicalDisk` (`MediaType`)
- 🆕 Saúde **SMART** e alerta de disco quase cheio. — `Get-PhysicalDisk`/`Get-StorageReliabilityCounter`

### 11. Segurança 🆕
- Estado das mitigações (ASLR/DEP), Defender, firewall, BitLocker, UAC.
- Importante porque vários tweaks de "FPS" mexem em segurança — precisa avisar o usuário.

### 12. Sensores (temperatura/uso)
- CPU/GPU temp e uso, via LibreHardwareMonitor/OpenHardwareMonitor se presente. Já em `metricas.ps1`.
- 🆕 Tornar o sensor opcional embarcável para não depender de download manual.

### 13. Processos
- Top processos por CPU/RAM (já em `metricas.ps1`).

---

## Saída: `system-profile.json` (esboço de schema)

```jsonc
{
  "schema_version": "1.0",
  "gerado_em": "2026-06-20T14:00:00",
  "pc": "DESKTOP-XXXX",
  "contexto": {
    "tipo": "desktop",            // desktop | notebook
    "na_bateria": false,
    "virtualizado": false
  },
  "hardware": { "cpu": {}, "ram": {}, "gpu": {}, "bios": {} },
  "so": {},
  "energia": {},
  "rede": { "adaptadores": [] },
  "discos": [ { "letra": "C:", "tipo": "SSD", "saude": "OK", "uso_pct": 53 } ],
  "servicos": [ { "nome": "SysMain", "status": "Running", "classe": "seguro-desativar" } ],
  "startup": [],
  "tarefas": [],
  "drivers": [],
  "apps": [],
  "seguranca": {},
  "registro": { /* estado atual das chaves do catálogo */ },
  "sensores": {},
  "processos": []
}
```

## Do profile para a recomendação

O motor de regras lê o profile e, para cada regra, avalia a condição `aplica_se`. Exemplos de
decisões conscientes do hardware que **só são possíveis com um scan completo**:

- **HDD detectado** → *não* recomendar desativar Prefetcher/Superfetch (em HDD ajuda).
- **Notebook na bateria** → *não* recomendar plano de Máximo Desempenho como padrão.
- **GPU Intel** → pular tweaks específicos de NVIDIA/AMD.
- **Disco quase cheio** → priorizar limpeza antes de tweaks de performance.
- **Mitigações de segurança ativas** → marcar tweaks que as desativam como risco alto e desmarcados.

## Princípios de implementação

- **Idempotente e read-only**: rodar o scan N vezes não muda nada no PC.
- **Tolerante a falha**: cada coletor isola erros (try/catch) e marca `N/A` sem derrubar o scan.
- **Versionado**: `schema_version` no topo para evoluir o formato sem quebrar consumidores.
- **Sem PII desnecessária**: evitar coletar dados pessoais que não sirvam à recomendação.
