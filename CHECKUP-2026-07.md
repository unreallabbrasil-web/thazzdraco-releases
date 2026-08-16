# Checkup geral — ThazzDraco Optimizer (jul/2026)

Auditoria profunda de todo o app Go (`app/`), UI web, motor de regras, módulos de
escrita/leitura no Windows, servidor HTTP, medidor de FPS, build e higiene do repo.
Cada achado abaixo foi **confirmado lendo o código** (não é especulação). Severidade:
🔴 crítico · 🟠 alto · 🟡 médio · ⚪ baixo.

O produto tem um princípio inegociável — **"só mostrar dado real" + tudo reversível com
undo exato**. Boa parte dos achados críticos são exatamente violações desses dois pilares.

---

## Sumário executivo — o que atacar primeiro

| # | Tema | Severidade | Por quê |
|---|------|-----------|---------|
| 1 | Servidor sem token de sessão (EoP local + DNS rebinding) | 🔴 | Processo elevado deixa qualquer app/página local disparar ações de admin |
| 2 | Auto-updater sem verificação de assinatura/hash | 🔴 | É RCE-como-admin se o repo de releases for comprometido |
| 3 | Undo perdido em falha/crash no meio do apply | 🔴 | Fere o invariante central: escrita parcial sem reversão |
| 4 | "Dado inventado" em várias métricas | 🔴 | Ping 0% loss, RAM 65535 MHz, VRAM errada, temp fake — fere o outro pilar |
| 5 | Crash determinístico do medidor de FPS | 🟠 | `NewCallback` vaza → app morre após ~2000 polls na frente do cliente |
| 6 | Overlay do HUD quebrado (`/api/metrics`) | 🔴* | Feature inteira não popula; correção de 1 linha |
| 7 | Higiene: docs não commitados + legado na raiz | 🔴 | `git clean`/clone perde README e docs; risco de editar rules.json errado |

\* crítico funcional (não de segurança), mas trivial de corrigir.

---

## 1. Segurança do servidor HTTP (roda ELEVADO)

O servidor escuta em `127.0.0.1:porta-efêmera` como admin e a **única** defesa é o
`sameOriginGuard`, que só barra navegadores (headers `Sec-Fetch-*`/`Origin`).

- 🔴 **Sem token de autenticação por sessão** (`server.go`). Um processo local **não-elevado**
  (curl, malware sem admin) não envia `Sec-Fetch-*`, então o guard não barra nada: ele
  descobre a porta via `netstat` e faz `POST /api/aplicar`, `/api/servicos/parar`,
  `/api/hosts/adicionar`, `/api/reiniciar` usando o app como **proxy elevado**. Escalação
  de privilégio local clássica.
- 🔴 **DNS rebinding** (`server.go:172-188`). O guard checa `Origin.Host == r.Host` mas
  **nunca valida que `r.Host` é `127.0.0.1`/`localhost`**. Uma página `evil.com` que rebinda
  para loopback passa no guard (vira `same-origin`) e dispara admin.
- 🟠 **Nenhum handler valida método HTTP** — endpoints destrutivos aceitam GET (ex.:
  `GET /api/reiniciar` → `shutdown /r`). GET é o mais fácil de abusar (`<img>`, navegação).
- 🟠 **`/api/servicos/parar` e `/api/servicos` param QUALQUER serviço** sem allowlist
  (`service.go:140` + `server.go:744`). `req.Nome` vem cru do JSON.
- 🟠 **Sem timeouts no `http.Server`** (`main.go:69-76`): `ReadHeaderTimeout`, `WriteTimeout`
  etc. ausentes → Slowloris local. Pior: conexões `/api/heartbeat` (SSE) mantêm
  `InFlight()>0` permanentemente e **neutralizam o watchdog**, deixando o processo elevado órfão.
- 🟡 **`json.Decode` sem `MaxBytesReader` e com erro ignorado** em ~25 handlers → body gigante
  e struct zerada aplicada em silêncio.
- 🟡 **`/api/rebuild` roda `CONSTRUIR.ps1 -ExecutionPolicy Bypass`** embutido no binário de
  produção (`server.go:261`). Deveria sair do build de produção ou ficar atrás de flag de dev.

**Correção-chave:** token gerado no boot, injetado na URL `--app=` do Edge e exigido em todo
`/api/`; allowlist de `Host` (127.0.0.1/localhost); exigir POST; allowlist de serviços.

## 2. Auto-updater = RCE-como-admin

- 🔴 `updater.go:172-267` + `update.go`: `DownloadExe` baixa o `.exe` do release do GitHub e
  `SelfReplaceAndRestart` **substitui o binário e relança como admin** — **sem verificar
  assinatura Authenticode, sem hash esperado, sem pinning de host/esquema**. Se a conta/repo
  de releases for comprometida (ou um asset malicioso publicado), toda instalação baixa e roda
  binário arbitrário com privilégio de admin.
- 🟠 `update.go:39` estado global `dl` compartilhado + reentrância: `/api/update/apply` chamado
  2× pode lançar dois self-replace concorrentes; e o `os.Exit(0)` **não checa `s.InFlight()`**
  — aborta backup/reparo em andamento.

**Correção-chave:** validar `https` + host allowlist e **verificar assinatura Authenticode do
.exe antes de aplicar**; guardar `InFlight()`; serializar `dl`.

## 3. Reversibilidade / undo (o invariante central)

- 🔴 **Snapshot descartado quando o apply falha no meio** (`apply.go:234-238`). `applyAction`
  escreve valor a valor; se a 3ª de 4 escritas falha, as 2 primeiras já foram alteradas e o
  `undo` parcial é jogado fora (`ApplyRules` faz `continue` sem gravar em `batch.Rules`).
- 🔴 **Histórico só persiste no FIM do lote** (`apply.go:250-256`). Crash/kill/queda de energia
  na regra 15 de 30 → as 14 já aplicadas **não têm registro** em `historico.json`. Não há
  backup automático antes do apply (é endpoint manual). Persistir incrementalmente (write-ahead).
- 🟠 **Undo ignora todos os erros e apaga o histórico mesmo tendo falhado** (`apply.go:399-485`).
  Sem elevação, restaurações HKLM falham em silêncio, a UI mostra sucesso e o lote **some** do
  histórico → sistema alterado + info de undo destruída. Contraste: `RestoreBackup` conta
  `restored/failed` honestamente — o undo precisa do mesmo tratamento.
- 🟠 **Preset custom aplica com `confirmar=true` hardcoded** (`server.go:1068`) → bypass do
  consentimento de regras de risco.
- 🟡 **Snapshot não cobre REG_MULTI_SZ / REG_BINARY** (`registry.go:112-169`). Marca
  `Existed=true` sem capturar dados; `RestoreSnapshot` cai no `default` e retorna `nil`
  (sucesso falso) — undo deixa o valor tweakado e perde o original. *(2 agentes confirmaram.)*
- 🟡 **Undo de powercfg regrava no `SCHEME_CURRENT` do momento do undo**, não no esquema onde
  escreveu (`apply.go:414`) — corrompe um plano que não foi tocado.
- 🟡 **Undo de lote sem disciplina LIFO** (`apply.go:427`) — desfazer um lote antigo por cima de
  um novo que tocou o mesmo valor corrompe os snapshots.

## 4. "Dado inventado" — viola "só mostrar dado real"

- 🔴 **Ping: host que não resolve vira "5/5 recebidos, 0% de perda, 0 ms"** (`pinger.go:46-48`).
  Sem linha de perda na saída, assume `Recebidos = Enviados`; a guarda final não pega porque
  `Recebidos` já é 5. *(Verificado à mão.)* Ex.: testar "Riot BR" offline mostra ping perfeito.
- 🟠 **SMBIOS 0xFFFF (velocidade desconhecida) tratado como 65535 MHz** (`profile.go:176` +
  `diagnose.go:74`) → gargalo "ative o XMP para subir a 65535 MHz". Filtrar `!= 0xFFFF`.
- 🟠 **Falha na query de seek penalty vira "HDD"** (`profile.go:387`) → NVMe atrás de RAID/USB
  aparece como HDD e gera "mova o jogo para um SSD" para um jogo já em SSD.
- 🟠 **"VRAM total" via PDH não é capacidade** e é somada entre adaptadores (`gpu_usage.go:184`
  + `gpu.go:230`) → iGPU+dGPU mostram a mesma "VRAM" que não corresponde a nenhuma.
- 🟠 **AdapterRAM do WMI é uint32** (`amd_panel.go`) → RX 7800 XT 16 GB mostra "4096 MB".
- 🟠 **nvidia-smi `[N/A]` e falhas viram 0** (`gpu_panel.go:43`) → 0 W / 0 °C como leitura real;
  e `SplitN(...,11)` quebra com 2 GPUs.
- 🟡 **`cpu_temp_c` é zona térmica ACPI**, não a CPU (`metrics.go:33`) — muitas vezes estática.
  Renomear para "temp. zona térmica (ACPI)" ou remover (o próprio `health.go` já admite isso).
- 🟡 **`risco_falha: 85` inventado** (`health.go:124`). O SMART devolve booleano; "85%" cria
  precisão que não existe. Mostrar só "falha prevista pelo SMART".
- 🟡 **`cpuPercent` devolve 0 em falha e na 1ª amostra** (`metrics.go:54`) — 0% como se fosse
  CPU ociosa. Deveria ser -1/N/A como as demais.
- 🟡 **DPC latency é ruído** (`dpclatency.go:82-95`): não espera 1s entre amostras (o `-w` do
  ping não é duração); e o contador PDH tem nome provavelmente errado ("DPC" vs "DPCs
  Queued/sec"), fazendo cair sempre no fallback PowerShell.

## 5. Medidor de FPS (PresentMon)

- 🟠 **Vazamento de `syscall.NewCallback` em `Games()`** (`fps.go:326`) — o runtime Go **nunca
  libera** callbacks (limite ~2000/processo). Se a UI faz poll da lista de jogos, o app morre com
  `panic: too many callback functions` após ~1h de uso. Criar o callback 1× em nível de pacote.
- 🟠 **Filtro `v <= 1000` descarta congelamentos reais >1s** (`stats.go:29`) — hitches de shader/
  streaming (1–3 s) somem do 1%/0.1% low, do stutter e da duração. Esconde exatamente os piores
  momentos que o produto promete medir. Descartar só o warm-up por swapchain, não por teto.
- 🟡 **0.1% low sem lastro em capturas curtas** (`stats.go:55`) — em 30 s a 60 FPS é a média de
  2 frames; em <15 s, 1 frame (= FPSMin). Omitir quando `n < ~2000`.
- 🟡 **CSV e sessão ETW com nome fixo** (`fps.go:61,148`) — duas instâncias do app se sabotam
  (matam a sessão uma da outra, disputam o mesmo CSV). Usar `os.CreateTemp` + PID no nome.
- 🟡 **Erro do `exec` engolido** (`fps.go:169`) — AV em quarentena do PresentMon → só
  "não gerou dados" sem a causa. Sessão ETW órfã após kill por timeout (usar Job Object).

## 6. UI web (`app.js` / `overlay.html`)

- 🔴* **Overlay chama `/api/metrics` (inexistente)** (`overlay.html:82`) — CPU/RAM/GPU/Temp ficam
  em "—" para sempre. É `/api/metricas`. *(Verificado à mão. Correção de 1 linha.)*
- 🟠 **FPS: botão preso + poll não retomado** (`app.js:1859`) — iniciar captura, trocar de aba e
  voltar deixa "Medir FPS" desabilitado e o resultado nunca aparece. Copiar o padrão do Reparo
  (`showSub` reconsulta `/api/fps/status`).
- 🟠 **Auditoria/instalação de drivers: `setInterval` órfão + race de duplo clique**
  (`app.js:3125,3244`) — polls concorrentes no mesmo status global; botão reabilitado cedo demais.
  Registrar em `state` e limpar em `stopLiveJobs`.
- 🟠 **Fechar o painel de update durante o download cancela o apply em silêncio** (`app.js:3448`) —
  `closeUpdatePanel` mata `state.updatePoll`; o `/api/update/apply` nunca dispara.
- 🟡 **`vram_usada_mb` não existe** (`app.js:1515`) — a página GPU sempre mostra "VRAM N/D" (é
  `vram_uso_mb`).
- 🟡 **`relatorio.aplicados`** (`app.js:2869`) — é `aplicadas` (array). Toast sempre diz "0 regras
  ativadas" mesmo aplicando 12.
- 🟡 **Excluir backup sem confirmação** (`app.js:1075`) — botão "✕" colado no "Restaurar"; um clique
  errado apaga a rede de segurança do cliente.
- 🟡 **Modo Game ao vivo sem guarda de duplo clique** (`app.js:1438`) — dessincroniza UI × backend.
- 🟡 Código morto: `renderProfile` (referencia `#profile` inexistente → TypeError se chamado),
  `gameProfile`/`data-gprof`, `state.admin`. Ícones `data-icon` não populados em `renderDiag`/
  `renderDriver` (botões sem ícone).
- Escape XSS está **bem** aplicado (~40 pontos); única exceção é `data-guide` (dado estático, não
  explorável hoje).

## 7. Localização PT-BR quebra parsing (público-alvo do produto)

- 🟠 **Saída do SFC é UTF-16LE** (`repair.go:142`) — o `bufio.Scanner` lê bytes crus, `rePct`
  nunca casa, progresso fica em 0 e o veredito final ("corrigiu corrupção") vira lixo ilegível.
- 🟡 **powercfg escreve em codepage OEM (CP850)** (`powercfg.go:50`) — "Desempenho máximo" não é
  UTF-8 válido, `strings.Contains(..."máximo")` nunca casa → `UltimatePerformance()` **cria uma
  cópia nova do plano a cada clique** e a UI mostra mojibake. Decodificar OEM ou casar por GUID.
- 🟡 **DPC/ping fallback**: `ParseFloat` quebra com vírgula decimal do PowerShell PT-BR → falha
  de medição vira "latência normal" (nota verde).
- 🟡 **Detecção de perda de ping só cobre EN/PT** (`pinger.go:33`) — Windows es/fr/de cai no bug
  do "0% de perda inventado".

## 8. Escritas de sistema sem rede de segurança

- 🟠 **`netsh int ip reset` apaga IP estático/gateway/DNS sem backup** (`maintenance.go:38`) — PC de
  cliente com IP fixo volta sem rede após reboot e você perde o TeamViewer. Exportar
  `netsh -c interface dump` antes + avisar se houver IP estático.
- 🟠 **WUA (drivers) sem timeout nem cancelamento** (`driverupdate.go:62`) — chamadas COM
  síncronas; se o Windows Update travar (comuníssimo em PC de cliente), a feature trava até
  reiniciar o app. Usar API assíncrona ou watchdog. E **resultado da instalação não é
  verificado** (`:317`) — driver que falhou vira "concluído".
- 🟠 **FullscreenMode invertido no default UE** (`gameconfig.go:82`) — "Tela cheia" vira borderless,
  "Janela" vira fullscreen exclusivo. Afeta o **Fortnite** (usa `ueFields(nil)`); VALORANT/PUBG já
  corrigem localmente. *(Verificado à mão: o default 1/2/0 conflita com o mapeamento correto 0/1/2
  que o VALORANT usa.)*
- 🟠 **`SetGameTweaks` destrói flags de compat pré-existentes** (`games.go:411`) — sobrescreve
  `AppCompatFlags\Layers` inteiro (apaga um `RUNASADMIN HIGHDPIAWARE` do usuário) e no undo deleta
  o valor todo em vez do token. Idem `GpuPreference`.
- 🟡 **hosts**: `isIPLike` aceita string com espaço (injeta 2º mapeamento); `comment` não sanitiza
  `\n` (injeta linhas); rewrite sem backup e sem write-temp-rename (`hostseditor.go:88`).
- 🟡 **DNS sem snapshot do anterior + parsing por idioma** (`dns.go:28`) — DNS estático do cliente
  perdido; aplica em VPN; só IPv4.
- 🟡 **Debloat opera no usuário elevado, não no real** (`debloat.go:53`) — se o técnico elevar com
  outra conta, remove do perfil errado e diz "removido".
- 🟡 **Browser cleaner sem trava `confirmar` e sem backup** (`browsercleaner.go:140`) — desloga de
  tudo na hora; browser aberto → SQLite travado → deleção parcial corrompe o perfil.
- 🟡 **Reparo: estado "erro" é inatingível** (`repair.go:35`) — SFC/DISM que falha (ex.: DISM
  0x800f081f) termina como "concluído/100%"; o exit code (o veredito) é descartado.
- 🟡 **WUPausar/WURetomar nunca checam se o `sc` funcionou** (`winupdate.go:33`) — sempre retorna
  "pausado" mesmo com falha; e desabilitar `UsoSvc` pode quebrar a Store até alguém Retomar.

## 9. Regras do `rules.json` — obsoletas/mortas (efeito nulo, vendidas como "verde acionável")

Cada uma escreve uma chave que o Windows 10/11 moderno **ignora**, mas o detect passa a reportar
"aplicado" (= inventar dado):
- 🟠 `win.modern-standby-off` — `CsEnabled` morto desde o Win10 2004.
- 🟡 `win.disable-web-search` — `BingSearchEnabled` não funciona desde o 2004 (é
  `DisableSearchBoxSuggestions`).
- 🟡 `win.copilot-off` / `win.cortana-off` — Copilot virou app da Store; Cortana removida em 2023.
- 🟡 `svc.nvidia-telemetry-off` — `NvTelemetryContainer` não existe nos drivers desde ~2019.
- 🟡 `mem.prefetch-superfetch-off` — `EnableSuperfetch` inerte; duplica `svc.sysmain-off`.
- 🟡 `net.smb1-off` — só desliga o lado servidor; em Win moderno já vem removido → detect marca
  "pendente" e derruba o score em máquinas já seguras.
- 🟡 `display.hdcp-off` — `DisableHDCP` não é chave documentada do Windows (HDCP é por driver).
- 🟠 `gpu.driver-keep` (`SearchOrderConfig=0`) **conflita com a própria feature de atualização de
  drivers do app** (v4.4.x) — bloqueia a aquisição via Windows Update.
- ⚪ `net.qos-reserve-off` — mito da "reserva de 20%"; não libera banda para jogos.

Além disso, **7 regras detectam só um subconjunto do que escrevem** (`reg.games-task-priority`,
`win.game-mode`, `win.game-dvr-off`, `win.disable-tips`, `net.tcp-params` etc.) → estado nunca
converge para o prometido e a regra some do histórico.

## 10. Higiene do repo, build e docs

- 🔴 **Docs não estão no git**: `README.md`, `docs/` inteiro (incl. o canônico
  `GUIA-OTIMIZACAO-GAMING.md`) e `AUDIT.md` estão untracked. Um `git clean`/clone perde tudo.
  **Commitar já.**
- 🔴 **`rules\rules.json` (raiz, 41 regras) está desatualizado** vs `app\internal\engine\assets\
  rules.json` (**62 regras**). O motor Go só embute o do app (`go:embed`); editar o da raiz não
  faz nada. Risco real de editar o arquivo errado — mover `rules\` para `legacy\` ou deletar.
- 🟠 **~20 arquivos da v3.0 PowerShell ainda na raiz** (ThazzDraco.ps1, motor.ps1, executor.ps1,
  lib.ps1, coletar/metricas/servidor.ps1, painel.html, Dashboard.html, os .bat, estado.json,
  metricas.json, build-exe.ps1) — o README diz que a v3.0 foi para `legacy\`, mas só tem 1 arquivo lá.
- 🟠 **`lancar.ps1` sobrescreve o entregável `dist\ThazzDraco.exe` SEM `-ldflags="-H windowsgui -s
  -w"`** → binário com console e sem strip no caminho de distribuição.
- 🟠 **Versão hardcoded 4.0.0.0** no `resgen.py` enquanto `VERSION`=4.4.3 → todo exe mostra 4.0.0.0
  nos Detalhes. `versioninfo.json` é arquivo morto (nada lê). CHANGELOG parado no 4.0.0.
- 🟠 **"43 regras"** repetido em README + 4 docs; o real é **62**. README diz "v4.0 jun/2026"
  (real 4.4.3) e "~7,9 MB" (real 8,8 MB).
- 🟡 **CI (`release.yml`) usa Go 1.22** vs `go.mod` que exige **1.26**, e **não roda `resgen.py`**
  → depende do `resource.syso` commitado (stale, 4.0.0.0). O exe do Release pode divergir do local.
- 🟡 **`AUDIT.md` anterior (35 itens de UI, 29/jun) está marcado como ✅ mas quase nada foi aplicado**
  no código (amostrei 8 itens; 7 não implementados). Tratar como backlog aberto, não trabalho feito.
- 🟡 **3 caminhos de build divergentes** (`build.ps1`, `lancar.ps1`, `CONSTRUIR.ps1`) — consolidar.
- ⚪ Binário morto rastreado: `app/internal/fps/bin/PresentMon.exe` (380 KB) — o código embute só o
  `.gz`. `resource.syso` gerado é rastreado. `go mod tidy` não rodado (deps marcadas `// indirect`).

---

## Nota de método

Os achados vieram de 7 auditorias paralelas (motor, escritas, leituras, servidor, FPS, UI, build),
cada uma lendo os arquivos da sua área por completo, com os 4 mais impactantes reverificados à mão.
Onde 2 agentes convergiram no mesmo ponto (ex.: token de sessão, undo de MULTI_SZ, `/api/metrics`,
quoting de `admin.go`), a confiança é alta.
