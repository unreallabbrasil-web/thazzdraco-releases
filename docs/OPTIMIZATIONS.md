# Catálogo de Otimizações

> **Atualização v4.0 (jun/2026):** as otimizações hoje vivem como **regras declarativas** em
> `app/internal/engine/assets/rules.json` (**43 regras**), aplicadas pelo motor Go (não mais pelo
> `FPS_Optimizer_Script.ps1`, aposentado). O contrato está em [RULES-SCHEMA.md](RULES-SCHEMA.md). O
> conteúdo abaixo segue válido como **base de conhecimento** das otimizações.
>
> **Regras adicionadas na v4.0 (todas verdes/amarelas, reversíveis):**
> `win.startup-delay-off` (acelera apps na inicialização), `win.menu-show-delay` (menus instantâneos),
> `win.disable-tips` (dicas/propaganda do Windows), `win.disable-web-search` (busca do Iniciar só
> local), `win.background-apps-off` (apps em segundo plano), `svc.remote-registry-off`,
> `svc.nvidia-telemetry-off`, `svc.wmp-network-off`, `cpu.priority-separation`
> (Win32PrioritySeparation=38 — favorece o jogo em foco) e `gpu.driver-keep` (SearchOrderConfig=0 —
> impede o Windows de trocar o driver de vídeo sozinho).
>
> **Mudança de segurança:** a **limpeza** agora apaga o TEMP do **usuário interativo real** (via SID),
> não o do administrador.
>
> **Otimização por jogo** (registro HKCU/IFEO, reversível, por executável): **FSO off**
> (`DISABLEDXMAXIMIZEDWINDOWEDMODE` em AppCompatFlags\Layers), **GPU máx**
> (`GpuPreference=2` em DirectX\UserGpuPreferences), **CPU alta** (IFEO `PerfOptions\CpuPriorityClass=3`)
> e **excluir do antivírus** (pasta do jogo em exclusões do Defender, via `Add/Remove-MpPreference`).

Referência de **todas as otimizações** (catálogo histórico do `FPS_Optimizer_Script.ps1` v1.1 + bloco
avançado v3.0), base de conhecimento do motor de regras declarativas atual.

**Legenda**
- **Impacto:** Alto / Médio / Baixo (efeito esperado em FPS/latência).
- **Risco:** Baixo / Médio / Alto (chance de efeito colateral indesejado).
- **Rev.:** reversível com facilidade (Sim) ou exige cuidado/valor anterior (Parcial).
- **Reboot:** exige reiniciar para valer.

> ⚠️ Itens de **risco Alto** mexem em segurança/compatibilidade e, na plataforma, devem vir
> **desmarcados** com aviso explícito.

---

## 1. Plano de energia

| Tweak | Comando / Chave | Valor | Impacto | Risco | Rev. | Reboot |
|---|---|---|---|---|---|---|
| Plano Alto Desempenho | `powercfg /setactive 8c5e7fda-...` | ativo | Alto | Baixo | Sim | Não |
| Plano Máximo Desempenho (Ultimate) | `powercfg /duplicatescheme e9a42b02-...` | criado | Médio | Baixo | Sim | Não |
| Hibernação | `powercfg /hibernate off` | off | Baixo | Baixo | Sim | Não |
| CPU throttle min/max | `SUB_PROCESSOR PROCTHROTTLEMIN/MAX` | 100 / 100 | Alto | Médio¹ | Sim | Não |

¹ Em **notebook na bateria** aumenta consumo/calor — deve ser *gated* pelo scan.

## 2. Serviços desativados

`Stop-Service` + `StartupType Disabled` em: **SysMain** (Superfetch), **WSearch** (Windows
Search²), **DiagTrack** (telemetria), **dmwappushservice**, **MapsBroker**, **TabletInputService**,
**Fax**, **RetailDemo**, **XblAuthManager**, **XblGameSave**, **XboxNetApiSvc**, **lfsvc**
(geolocalização), **PhoneSvc**.

| | Impacto | Risco | Rev. | Reboot |
|---|---|---|---|---|
| Serviços acima | Médio | Médio² | Sim | Parcial |

² Desativar **WSearch** quebra a busca do menu Iniciar; **Xbox** afeta o Game Pass/Xbox app;
**SysMain** pode ajudar em HDD. Devem ser avaliados pelo perfil do PC.

## 3. Tweaks de registro (jogos / multimídia)

| Tweak | Chave | Valor | Impacto | Risco | Reboot |
|---|---|---|---|---|---|
| Game DVR off | `HKCU\System\GameConfigStore\GameDVR_Enabled` | 0 | Alto | Baixo | Não |
| FSE behavior | `...\GameDVR_FSEBehaviorMode` | 2 | Médio | Baixo | Não |
| AppCapture off | `HKCU\...\GameDVR\AppCaptureEnabled` | 0 | Alto | Baixo | Não |
| Game Mode on | `HKCU\Software\Microsoft\GameBar\AutoGameModeEnabled` | 1 | Médio | Baixo | Não |
| GPU Priority | `HKLM\...\SystemProfile\Tasks\Games\GPU Priority` | 8 | Médio | Baixo | Sim |
| Priority | `...\Tasks\Games\Priority` | 6 | Médio | Baixo | Sim |
| Scheduling Category | `...\Tasks\Games\Scheduling Category` | High | Médio | Baixo | Sim |
| SFIO Priority | `...\Tasks\Games\SFIO Priority` | High | Baixo | Baixo | Sim |
| Network Throttling off | `HKLM\...\SystemProfile\NetworkThrottlingIndex` | 0xffffffff | Médio | Baixo | Sim |
| System Responsiveness | `...\SystemProfile\SystemResponsiveness` | 0 | Médio | Baixo | Sim |
| Timer Resolution | `...\kernel\GlobalTimerResolutionRequests` | 1 | Médio | Médio | Sim |
| Efeitos visuais | `HKCU\...\VisualEffects\VisualFXSetting` | 2 | Baixo | Baixo | Não |

## 4. Boot / Timers (bcdedit)

| Tweak | Comando | Impacto | Risco | Reboot |
|---|---|---|---|---|
| Remover platform clock | `bcdedit /deletevalue useplatformclock` | Médio | Médio³ | Sim |
| Platform tick | `bcdedit /set useplatformtick yes` | Médio | Médio³ | Sim |
| Disable dynamic tick | `bcdedit /set disabledynamictick yes` | Médio | Médio³ | Sim |

³ Tweaks de timer/tick podem causar **instabilidade** em alguns sistemas; é dos itens mais
controversos. Manter reversível e bem sinalizado.

## 5. Rede

| Tweak | Chave / Comando | Valor | Impacto | Risco | Reboot |
|---|---|---|---|---|---|
| Nagle off (por interface) | `...\Tcpip\...\Interfaces\*\TcpAckFrequency` / `TCPNoDelay` | 1 / 1 | Médio⁴ | Baixo | Sim |
| Default TTL | `...\Tcpip\Parameters\DefaultTTL` | 64 | Baixo | Baixo | Sim |
| TCP timestamps | `...\Tcpip\Parameters\Tcp1323Opts` | 1 | Baixo | Baixo | Sim |
| Autotuning | `netsh int tcp set global autotuninglevel=normal` | normal | Baixo | Baixo | Não |
| Chimney / ECN / heuristics | `netsh int tcp ... disabled` | disabled | Baixo | Baixo | Não |

⁴ Nagle off reduz latência em jogos, mas pode reduzir eficiência em downloads — efeito depende do uso.

## 6. GPU

| Tweak | Chave | Valor | Impacto | Risco | Reboot |
|---|---|---|---|---|---|
| HAGS (GPU Scheduling) | `HKLM\...\GraphicsDrivers\HwSchMode` | 2 | Médio | Médio⁵ | Sim |

⁵ Depende de GPU/driver compatível; em alguns casos pode piorar. Idealmente *gated* pelo fabricante/driver.

## 7. Memória

| Tweak | Chave | Valor | Impacto | Risco | Reboot |
|---|---|---|---|---|---|
| Clear pagefile no shutdown | `...\Memory Management\ClearPageFileAtShutdown` | 0 | Baixo | Baixo | Sim |
| Disable Paging Executive | `...\DisablePagingExecutive` | 1 | Médio | Baixo⁶ | Sim |
| Large System Cache | `...\LargeSystemCache` | 0 | Baixo | Baixo | Sim |
| Prefetcher off | `...\PrefetchParameters\EnablePrefetcher` | 0 | Baixo | Médio⁷ | Sim |
| Superfetch off | `...\PrefetchParameters\EnableSuperfetch` | 0 | Baixo | Médio⁷ | Sim |

⁶ Requer RAM suficiente. ⁷ **Em HDD, Prefetch/Superfetch ajudam** — não desativar; *gating* por SSD/HDD.

## 8. Limpeza de temporários

`Remove-Item` em `%TEMP%`, `%TMP%`, `C:\Windows\Temp`, `C:\Windows\Prefetch`, `%LOCALAPPDATA%\Temp`.

| | Impacto | Risco | Rev. | Reboot |
|---|---|---|---|---|
| Limpeza temp | Baixo | Baixo | Sim (são temporários) | Não |

## 9. Startup apps

Remove dos registros `Run` (HKCU+HKLM): **OneDrive, Spotify, Discord, EpicGamesLauncher, Steam**.

| | Impacto | Risco | Rev. | Reboot |
|---|---|---|---|---|
| Remover do startup | Baixo | Baixo (preferência do usuário) | Parcial⁸ | Não |

⁸ Reversível, mas o valor original do `Run` precisa ser salvo para restaurar.

## 10. Mouse / Exibição

| Tweak | Chave | Valor | Impacto | Risco | Reboot |
|---|---|---|---|---|---|
| Aceleração off (raw input) | `HKCU\Control Panel\Mouse\MouseSpeed/Threshold1/2` | 0/0/0 | Médio | Baixo | Não⁹ |

⁹ Aplica no próximo logon. Preferência pessoal — alguns usuários querem aceleração.

## 11. DNS

| Tweak | Comando | Impacto | Risco | Reboot |
|---|---|---|---|---|
| Flush DNS | `ipconfig /flushdns` | Baixo | Baixo | Não |

---

## Bloco avançado (v3.0)

| Tweak | Chave / Comando | Valor | Impacto | Risco |
|---|---|---|---|---|
| Core Parking off | `SUB_PROCESSOR 0cc5b647.../ea062031...` | 100 | Médio | Médio |
| C-States baixa latência | `...\Control\Processor\Capabilities` | 0x0007e066 | Médio | Médio |
| Power Throttling off | `...\Power\PowerThrottling\PowerThrottlingOff` | 1 | Médio | Médio |
| NIC Interrupt Moderation off | `...\Class\{4d36e972-...}\*InterruptModeration` | 0 | Médio | Médio |
| NIC FlowControl / EEE off | `...\*FlowControl` / `*EEE` | 0 / 0 | Baixo | Médio |
| MSI Mode GPU | `...\Interrupt Management\...\MSISupported` | 1 | Médio | Médio |
| MSI Mode NVMe/SSD | idem para storage | 1 | Baixo | Médio |
| Prioridade CPU/IO por jogo (IFEO) | `...\Image File Execution Options\<jogo>\PerfOptions` | Cpu=3, Io=3, Page=5 | Médio | Médio |
| **MPO off** | `HKLM\...\Dwm\OverlayTestMode` | 5 | Médio | Médio |
| **HDCP off** | `...\GraphicsDrivers\DisableHDCP` | 1 | Baixo | **Alto**¹⁰ |
| **Mitigações (ASLR audit)** | `...\kernel\MitigationAuditOptions` | 0 | Baixo | **Alto**¹¹ |
| GPU Preemption | `...\GraphicsDrivers\Scheduler\EnablePreemption` | 1 | Baixo | Médio |

Jogos cobertos pela prioridade IFEO: `cs2.exe`, `csgo.exe`, `valorant.exe`,
`FortniteClient-Win64-Shipping.exe`, `cod.exe`, `ModernWarfare.exe`, `Warzone.exe`,
`RainbowSix.exe`, `r5apex.exe`.

¹⁰ **HDCP off pode quebrar reprodução de vídeo protegido** (Netflix/Prime/Disney+). Risco alto.
¹¹ Mexer em **mitigações de exploit** reduz segurança do sistema. Risco alto — só com consentimento explícito.

---

## Reversão (estado atual vs. alvo)

- **Hoje:** o script **não** cria ponto de restauração nem salva valores anteriores — não há undo.
- **Alvo (Fase 3):** antes de aplicar, criar ponto de restauração + exportar `.reg` das chaves
  afetadas e salvar o valor anterior de cada tweak, habilitando *undo* individual e em bloco.
  Ver [ROADMAP.md](ROADMAP.md#fase-3--aplicação-segura--restauração).

### Como reverter manualmente os itens de maior risco (hoje)
- **HDCP:** remover/`1→ausente` em `GraphicsDrivers\DisableHDCP`.
- **Mitigações:** restaurar padrão em *Segurança do Windows → Controle de app e navegador →
  Proteção contra exploração*.
- **Serviços:** `Set-Service <Nome> -StartupType Automatic` e iniciar.
- **bcdedit (tick/clock):** `bcdedit /deletevalue useplatformtick` / `disabledynamictick`.
