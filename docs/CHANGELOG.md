# Changelog

Histórico de versões do ThazzDraco Optimizer. Formato baseado em *Keep a Changelog*.

---

## [4.0.0] — Fix: adaptadores virtuais (Parsec/TeamViewer) poluíam o Desempenho

Sintoma do usuário: a tela "Desempenho ao vivo" mostrava um **"Parsec Virtual Display Adapter"** como
placa de vídeo (com o mesmo "uso 83%" do contador geral), mesmo o usuário acessando por **TeamViewer**, não
Parsec. Causa: o Parsec deixa um **driver de display virtual instalado** mesmo sem estar rodando; o
`GPUs()` (`winutil/gpu.go`) listava todos os adaptadores do `EnumDisplayDevices` **sem filtrar virtuais**
(o filtro existia só no `driver.go`, para a detecção de driver).

**Correção (`gpu.go`):** novo `isVirtualGPU()` reaproveita a lista `virtualGPUMarks` (virtual/parsec/idd/
rdp/teamviewer/vmware/etc.) e o loop de `GPUs()` pula esses adaptadores. Só GPU física aparece no
Desempenho. Validado via `/api/metricas`: lista voltou só com a NVIDIA real (uso 9% · 39°C), sem o Parsec.
`go vet` limpo, build 8,15 MB.

---

## [4.0.0] — Medição: velocímetro de FPS + bench honesto no acesso remoto

Pedido do usuário (atende clientes via TeamViewer): provar o ganho de FPS de forma **visual** e **fácil**,
sem o vai-e-vem manual de medir média. Decisão honesta firmada: **FPS de jogo real via PresentMon** é a
única prova que o cliente acredita E que funciona via TeamViewer (mede a GPU física, ignora o cap do
stream); benchmark gráfico no navegador (WebGL/rAF) **não** serve remoto (o stream limita o FPS). Mantidos
os **dois** caminhos.

- **Velocímetro de FPS** (`fpsGauge()` em `web/app.js` + estilos em `app.css`): no resultado da medição de
  FPS, um ponteiro semicircular mostra o **FPS médio** com escala dinâmica, **marcador laranja do "antes"**,
  **ganho %** em destaque e **ponteiro animado** (CSS transform, do "antes" até o medido). O **gráfico de
  frametime** continua abaixo. O card redundante de "FPS médio" saiu (o velocímetro já o mostra). Math
  validada sem NaN (FPS baixo/alto/sem-base).
- **Bench honesto no TeamViewer** (`renderBench()`): CPU/memória/disco são nativos = **sempre reais**, mesmo
  remoto. Quando a GPU é estressada e o **uso volta 0%** (sinal de acesso remoto/render por software), o card
  da GPU agora avisa explicitamente que o número da GPU pode não refletir a placa física — em vez de exibir
  "pico 0%" que parecia bug. Mantém o princípio "só dado real".
- Validado: `node --check` limpo, build 8,15 MB, assets embutidos conferidos (fpsGauge/fpsNeedle/CSS/endpoint
  headless 200).

---

## [4.0.0] — Fix: Backup "não abria" no PC do cliente

Sintoma do usuário: no PC do cliente, clicar em **Backup** não abria nada. Causa raiz: `openBackup()`
**esperava** o `GET /api/backup/lista` responder (`await api(...)`) **antes** de mostrar o modal
(`$("#modal").classList.add("show")`). Se essa chamada travasse/demorasse (rede local engasgada,
backend reanimando do watchdog, primeira chamada lenta), o `await` nunca retornava e o modal **nunca
abria** — falha silenciosa, sem nem um toast.

**Correção (`web/app.js`):** o modal agora **abre na hora** com um placeholder ("Carregando backups…")
e a lista carrega **depois**, assíncrona (`loadBackupList()`). Se a API falhar, mostra um **estado de
erro claro** dentro do modal ("Não consegui carregar os backups… você ainda pode criar um novo") em vez
de travar; se vier vazia, "Nenhum backup ainda". Botões Restaurar/Excluir seguem via delegação (não
quebrou). Validado: `/api/backup/lista` responde 200 em ~24 ms (2 backups), fix presente no `app.js`
embutido, `node --check` limpo, build 8,14 MB.

---

## [4.0.0] — Confiança do binário: reduzir alertas de SmartScreen/Defender

Sintoma do usuário: ao testar o `.exe` num **PC novo**, o Windows mostrava a tela roxa do **SmartScreen**
("O Windows protegeu o computador") e o **Defender** colocava em quarentena ("contém um vírus ou software
possivelmente indesejado") — risco de o técnico passar vergonha na frente do cliente. Diagnóstico honesto:
não é bug do build; o app aciona vários gatilhos heurísticos de malware ao mesmo tempo (sem assinatura +
manifesto admin + extrai PE em runtime + `Add-MpPreference` + edita registro/serviços + é um "otimizador"
= categoria PUA). Sem **assinatura de código** não há como zerar o SmartScreen; mas dá para reduzir muito
a heurística do Defender de graça:

- **Metadados de versão no `.exe`** (`resgen.py` → recurso `RT_VERSION`): a aba *Detalhes* não é mais em
  branco (sinal clássico de suspeito). Traz Empresa/Produto/Descrição/Versão/Copyright. Validado:
  `(Get-Item ThazzDraco.exe).VersionInfo` mostra "ThazzDraco Optimizer / ThazZDraco FPS Otimizacao".
- **PresentMon embutido compactado (gzip)** (`internal/fps/fps.go`): o cabeçalho PE (`MZ…`) de um `.exe`
  não aparece mais cru dentro do nosso binário (sinal forte de "dropper"). Descompacta em runtime (uma vez,
  `sync.Once`). Bônus: build **8,14 MB** (~200 KB menor). `go vet` limpo, testes do `fps` passando.
- **Novo doc [DISTRIBUICAO-CONFIANCA.md](DISTRIBUICAO-CONFIANCA.md)**: causa dos dois alertas, submissão à
  Microsoft como falso-positivo (grátis), playbook de campo (pendrive exFAT mata a marca-da-web do
  SmartScreen; desbloquear; liberar da quarentena) e a rota definitiva (certificado **EV**, ~US$ 250–500/
  ano + CNPJ — único que dá reputação instantânea no SmartScreen).
- **Recomendado (a fazer):** tornar a exclusão do Defender (`Add-MpPreference`) opcional e fora do "⚡ Tudo"
  — é o gatilho comportamental nº 1 e o ganho de FPS é pequeno.

---

## [4.0.0] — "Failed to fetch" no backup: watchdog robusto

Sintoma do usuário: criar backup (e às vezes outras operações) dava **"Failed to fetch"**. Causa raiz:
o watchdog encerrava o backend (`os.Exit`) após só **12s sem ping**, e o navegador **estrangula os
timers** (o `setInterval` do ping) quando a janela do app perde o foco — chegando a 1x/min depois de
~5 min em segundo plano. Resultado: janela parada um tempo → ping para → watchdog mata o servidor → o
próximo clique bate num backend morto → "Failed to fetch". Intermitente e **invisível no headless**
(o modo `-headless` nem roda o watchdog → "funcionava" no teste).

**Correção (4 camadas, `server.go`/`main.go`/`app.js`):**
- **Qualquer request marca atividade** — um middleware (`instrument`) trata toda chamada como sinal de
  vida (não só o `/api/ping`), então o próprio clique já reanima o watchdog.
- **Nunca encerra com operação em andamento** — contador atômico `inFlight`: backup/reparo/benchmark
  seguram a conexão sem ping e **não podem mais ser mortos no meio**.
- **Janela de inatividade 12s → 90s** — sobrevive ao estrangulamento (1x/min) e ainda limpa o processo
  órfão logo após fechar a janela.
- **Ping na volta do foco** (cliente) — `visibilitychange`/`focus` pingam na hora, reanimando o backend
  no instante em que o usuário volta.
- Validado: criar backup pelo novo middleware = 200/ok; rajada de requests concorrentes = todas 200;
  zero erros de console. Builds `tz.exe` + `dist` 7,9 MB.

---

## [4.0.0] — Auditoria de robustez: restauração honesta + travas de segurança

Checkup geral (revisor sênior no backend Go inteiro). **Zero bugs críticos** — o motor
aplicar→snapshot→desfazer é sólido e o fix do powercfg está confirmado. Corrigidos os achados que
tocam as **promessas centrais** do app (tudo reversível / nunca dado pessoal):

- **Restauração honesta** (`backup.go` + `server.go` + `app.js`): `RestoreBackup` contava itens como
  restaurados **mesmo quando falhavam** (ex.: sem admin → escritas HKLM/serviços falham) e dizia
  "sucesso". Para uma rede de segurança isso é o pior cenário. Agora **conta só o que voltou de
  verdade** e devolve `(restaurados, falhas)`; a UI avisa "Nada foi restaurado" / "Restauração parcial
  · N falharam".
- **Trava de servidor na limpeza irreversível** (`deepclean.go` + `server.go`): esvaziar a **Lixeira**
  e apagar a **Windows.old** dependiam só do popup da UI. Agora o backend **exige `confirmar` explícito**
  — um request solto/malformado não apaga mais esse tipo de coisa sem confirmação.
- **Escrita atômica** (`apply.go` + `backup.go`): histórico e backups gravam em `*.tmp` + `rename`
  (atômico no NTFS) — um crash no meio não corrompe mais os dados de undo/backup.
- **PDH de GPU à prova de panic** (`gpu_usage.go`): guarda em todos os procs do `pdh.dll` (`.Find()`)
  antes de chamar — um símbolo ausente não derruba mais a goroutine de `/api/metricas`.

---

## [4.0.0] — Passada de copy sênior (voz da marca) + 3 bugs de texto

Revisão profissional dos textos da UI ("copy master sênior"): uma **voz só**, confiante e direta, no
léxico da marca (**núcleo/CORE**, raio, destravar), mantendo o invariante "só promessa mensurável"
(nada de hype vazio). Registro padronizado em **você-imperativo** ("Meça", "Destrave"). Voz registrada
na memória do projeto (`voz-copy-thazzdraco`).

- **Tela inicial / hero** (`app.js`): "Pronto para escanear" → **"Hora de acordar a máquina"**; estados
  por score reescritos ("Tem FPS na mesa", "Máquina no talo", "Dragão ainda dormindo"); HUD "Sistema
  online" → **"Núcleo online"** (casa com DRACO // CORE); scan "Escaneando" → **"Rastreando gargalos"**.
- **3 bugs de copy caçados:** preset Competitivo dizia **"Latência extrema"** (lia ao contrário — extrema
  = ruim) → **"Latência mínima"**; botão "Rodar estresse" mas texto-guia dizia "Rodar benchmark" →
  unificado; rótulo truncado **"Oportun."** no HUD → **"Extras"**.
- Validado ao vivo no backend real: idle + estados de score (61 → "Tem FPS na mesa") + presets + bench
  renderizando certo, **zero erros de console** (screenshot conferido).

---

## [4.0.0] — BUG corrigido: regras de powercfg liam o valor errado

Sintoma do usuário: "aplico e diz que aplicou, mas fica disponível pra aplicar de novo, não mostra
aplicado". Causa raiz (encontrada reproduzindo): `PowercfgAcValue` pegava o **primeiro `0x`** da saída
do `powercfg /q`, que é o **"Minimum Possible Setting"** (0), não o **"Current AC Power Setting Index"**
(o valor real). Resultado:
- **power.processor-max-performance** (esperado 100): lia 0 → **nunca** mostrava aplicado (escrevia certo,
  mas a detecção não reconhecia → toggle ficava pendente pra sempre). Era o bug do usuário.
- **power.usb-selective-suspend-off** (esperado 0): lia 0 → mostrava aplicado **mesmo quando não estava**
  (falso positivo).

**Correção:** ler o **penúltimo `0x`** da saída (a estrutura termina sempre em AC e depois DC; os
primeiros hexes são Min/Max/incremento). Independe do idioma do Windows. Validado: processor-max passou a
ler AC=100 → "aplicado". Auditoria completa das 39 regras acionáveis: detect×action consistentes, leitura
de DWord trata 0xFFFFFFFF como -1 corretamente, serviços/registro/foreach todos com estado coerente —
**era só o powercfg**.

---

## [4.0.0] — Feedback claro quando falta administrador

Bug relatado: "aplico várias coisas e nada acontece / não mostra aplicado". Causa: a maioria das
otimizações mexe em **HKLM/serviços/powercfg** e exige admin; sem ele, falham com **"Access is denied"**
e o toggle não liga — mas o feedback era um toast críptico que sumia rápido (parecia "nada acontece").

- **`adminWarn()`**: ao detectar erro de acesso negado num apply (toggle, "aplicar verdes", preset),
  mostra um **modal claro** ("Rode como administrador" — explica o porquê e como fazer) + reforça o
  banner. Erros de "denied/negado" agora viram essa mensagem acionável, não o texto cru.
- `state.admin` guardado do `/api/info`. Confirmado: sem admin, regra HKLM → erro "Access is denied",
  estado fica "pendente" (correto); regras de usuário (Game Mode, transparência) aplicam sem admin —
  por isso "algumas sim, outras não". Auditoria: detect×action 100% consistentes (não era esse o bug);
  `WriteDWord`/`WriteString` criam a chave se não existir (não era isso). **Era privilégio mesmo.**

---

## [4.0.0] — Plano de energia agora aplica o Desempenho Máximo (Ultimate) no desktop

- A regra de energia virou **"Plano de energia máximo"**: no **desktop** ativa o **Desempenho Máximo
  (Ultimate Performance)**; no **notebook** mantém **Alto Desempenho** (Ultimate fritaria a bateria —
  alerta do manual). Antes só ativava Alto Desempenho.
- Novo action `power-max` no motor (desktop→`UltimatePerformance()`, notebook→High), com **snapshot do
  plano anterior** (undo restaura). `UltimatePerformance()` reusa uma cópia visível do Ultimate se já
  existe, senão cria (o template é oculto e não ativável direto; a cópia ganha GUID próprio).
- **Detecção por NOME** (`detectPowercfg` + `checkPowerPlan`): como a cópia do Ultimate tem GUID
  aleatório, casamos pelo nome do plano ("performance"/"desempenho") além do GUID — funciona em qualquer
  idioma do Windows e reconhece tanto Alto Desempenho quanto Ultimate.
- Validado: regra detecta "aplicado", diagnóstico mostra o plano por nome, e o fluxo do Ultimate
  (duplicatescheme→setactive) ativa "Ultimate Performance" de verdade (testado + restaurado).
- *Nota honesta: para FPS a diferença Alto×Máximo é pequena (os ganhos reais — CPU 100%, sem power
  throttling — já estão em regras separadas); é mais completude/perfeccionismo.*

---

## [4.0.0] — Reorganização da navegação (sub-abas)

Reestruturação "agrupado por intenção" (escolha do usuário) para acabar com as páginas roláveis gigantes
e enxugar o HUD.

- **HUD: 10 → 6 nós** — Núcleo · Diagnóstico · Otimizações · Jogos · **Manutenção** · **Medição**.
- **Manutenção** virou página com **sub-abas**: Limpeza · Reparo · Debloat · Ferramentas · Inicialização
  (a Inicialização saiu de nó próprio e entrou aqui). Cada sub-aba renderiza sob demanda — sem mais o
  rolo único com 5 features empilhadas.
- **Medição** (novo nó) agrupa **Desempenho · FPS · Benchmark** em sub-abas.
- **Histórico** virou ícone no topo (junto de Backup/Relatório).
- **CHKDSK** ganhou checkbox na sub-aba Reparo (o backend já tinha a etapa).
- Roteador novo (`showPage`/`showSub`/`navTo`/`stopLiveJobs`): sub-abas exclusivas, deep-links de
  diagnóstico/limpeza remapeados, e os polls (métricas/FPS/reparo) param ao sair da sub-aba dona.
  IDs internos preservados → toda a lógica de render seguiu funcionando. Validado: 6 nós, sub-abas
  exclusivas, navTo, histórico no topo, métricas ao vivo. Build 8,30 MB.

---

## [4.0.0] — Ferramentas do "Manual Mestre" (lacunas preenchidas)

Absorvido o `manual-mestre-otimizacao-pc.md` (guia sênior). Validou fortemente a filosofia do app (medir/
reverter/sem snake-oil) e os "mitos perigosos" que o app já evita. Implementadas as lacunas seguras:

- **Ferramentas rápidas** (`winutil/maintenance.go`, seção na página Manutenção): **Faxina de rede**
  (flushdns + reset Winsock/TCP-IP, **sem** release/renew p/ não derrubar TeamViewer; §16), **TRIM nos
  SSDs** (`defrag /C /L`; §9), **Plano Desempenho Máximo** (desbloqueia/ativa o Ultimate Performance
  oculto; §14.2), **Varredura rápida do Defender** (em segundo plano; §18), **Liberar hibernação**
  (apaga `hiberfil.sys`, libera GBs, com aviso de Fast Startup; §19.2).
- **CHKDSK no reparo** — 5ª etapa do Reparo do Windows (`chkdsk /scan`, online, sem reinício; §20).
- **Guia de ajustes NVIDIA/AMD** — modal no card de Driver (Modo Baixa Latência Ultra, energia máx,
  cache de shader, V-Sync off, G-SYNC/FreeSync, SAM, Anti-Lag…; §12/§13). Honesto: orienta (o painel do
  fabricante não tem API limpa); aponta o que o app já cobre (HAGS, shader cache, por-jogo, térmica).

Tudo nativo, reversível e sem tocar dados pessoais. Build 8,30 MB.

---

## [4.0.0] — Auditoria geral: segurança, concorrência e robustez

Checkup completo (3 revisores em paralelo + verificação manual). Corrigido:

**Segurança**
- **Guard CSRF/origem** (`server.go`): a API local rejeita chamadas cross-site (valida `Sec-Fetch-Site`
  e `Origin`, que o JS de um site malicioso não consegue forjar). Como o app roda elevado, fecha o vetor
  de um site/anúncio descobrir a porta e disparar operações. Validado: cross-site → 403, legítimo → 200.
- **Deleção à prova de junction** (`deepclean.go`): `deletePath` nunca segue symlinks/reparse points
  (apaga só o vínculo, não o destino) — fecha a via de apagar fora do alvo.
- **Windows.old com guarda + sem cmd** (`deepclean.go`): só roda em `<SystemDrive>\Windows.old`
  (validação interna) e usa `os.RemoveAll` em vez de `cmd /c rd` (sem parser de shell).
- **Caminhos sempre absolutos** (`deepclean.go`): se o perfil do usuário não resolver, os caminhos de
  cache ficam vazios (ignorados) em vez de virarem relativos ao diretório atual.
- **Debloat com dupla validação** (`debloat.go`): além do regex anti-injeção, a família do pacote tem
  de estar no catálogo curado — impede remover qualquer pacote arbitrário via API.
- **Cache de miniaturas por prefixo** (`deepclean.go`): só apaga `thumbcache_*`/`iconcache_*`.

**Concorrência / robustez**
- **Timeout de guarda na captura de FPS** (`fps.go`): `exec.CommandContext` mata o PresentMon se travar
  — evita prender o motor em "capturando" para sempre.
- **Locks nas operações destrutivas** (`server.go`): `opMu` serializa debloat/limpeza/driver; `s.mu` no
  tweak de jogo (escreve registro) — sem dupla execução concorrente.
- **Vazamento de handle PDH** fechado em falha parcial (`gpu_usage.go`).

**Frontend**
- **Timers órfãos**: `showPage` agora para `fpsPoll`/`repairPoll` ao trocar de página; `runThermal` e
  `runBench` limpam o poll de métricas em `finally` (não vazam se o estresse falhar).
- **Contexto WebGL** liberado em todos os caminhos de `gpuStress` (não acumula contextos perdidos).
- **Null-guards** em `renderInicio`/`renderAll`; guarda anti-duplo-clique no toggle de jogo; `escHtml` no
  toast; ícone correto para toast de aviso; remoção de linha morta.

*Deferido (baixo risco, anotado): tweaks de jogo (IFEO/Defender) ainda não entram no "Desfazer tudo"
central — são reversíveis pelo próprio toggle; reset do Windows Update é best-effort supervisionado.*

---

## [4.0.0] — Relatório com prova (Objetivo B #10 — fecha o Objetivo B)

- **Relatório turbinado** — o relatório do cliente (overlay imprimível/PDF) agora carrega a **prova de
  ganho**: **FPS antes × depois** (médio, 1% low, 0.1% low e o **% de ganho** — ex.: 71 → 119, +66%),
  **Desempenho medido** (Índice ThazzDraco antes/depois, GPU em pts + temperatura de pico, CPU, disco) e
  o resultado do **teste térmico** (temp sob carga, throttling). Tudo puxado do estado real das medições
  (FPS/benchmark/térmica).
- **Branding do cliente** — botão "Logo do cliente" (upload de imagem → data URL) exibe a marca do
  cliente no cabeçalho, ao lado da ThazzDraco, junto com o nome. Sai no PDF.
- Validado: relatório renderiza FPS 71,4→118,6 (+66%), Desempenho medido e logo do cliente.
  **Objetivo B (6–10) concluído.**

---

## [4.0.0] — Saúde térmica & undervolt (Objetivo B #9)

- **Teste de estresse térmico** (página Desempenho) — estressa a GPU (WebGL) por ~25s e mede a
  **temperatura de pico** + detecta **throttling térmico real** via NVML
  (`nvmlDeviceGetCurrentClocksThrottleReasons`: distingue throttle **térmico** de **limite de potência**).
  Veredito honesto: refrigeração saudável / esquentando / **throttling térmico** (custa FPS) com conselho
  (poeira, fans, pasta térmica, undervolt). Campo `throttle` adicionado ao GPUInfo (`/api/metricas`).
- **Guia de undervolt** — modal com passo a passo: GPU (MSI Afterburner, curva de voltagem) + CPU AMD
  (Curve Optimizer/PBO) + CPU Intel (XTU/offset), com aviso de estabilidade. Não aplica automático
  (é no Afterburner/BIOS) — orienta.
- **Honestidade:** temperatura real só NVIDIA (NVML); **AMD/Intel = uso medido, temp N/A** (sensor do
  fabricante); **CPU não é medida** (sem driver de sensor — coerente com o princípio firmado). Validado:
  RTX 5070 sob carga 95%, pico 60°C, sem throttling → veredito "saudável".

---

## [4.0.0] — Limpeza profunda (Objetivo B #8)

- **Limpeza profunda** — `winutil/deepclean.go` + `/api/limpeza-profunda/scan` e `/limpar`. Vai além
  dos temporários, por categoria com tamanho real: **cache de shaders** (NVIDIA DXCache/GLCache, D3DSCache,
  AMD — libera espaço e corrige stutter de shaders), **despejos de erro** (Minidump/MEMORY.DMP/CrashDumps/
  WER), **cache do Windows Update**, **Delivery Optimization**, **cache de miniaturas**, **Lixeira** e
  **Windows.old** (versão anterior do Windows — dezenas de GB). Cada categoria mostra MB, recomendados
  pré-marcados; **Lixeira e Windows.old vêm desmarcados e com aviso**. Tudo regenerável — nunca dados
  pessoais. Windows.old usa **icacls (SID dos Admins, independe de idioma) + rd** para vencer o
  TrustedInstaller; WinSxS fica para o DISM Cleanup do #6 (não duplicado). Página Manutenção, abaixo da
  limpeza de temporários. Validado: 941 MB limpáveis na máquina do usuário (**832 MB só de shader cache**).

---

## [4.0.0] — Debloat profissional (Objetivo B #7)

- **Remover bloatware** — `winutil/debloat.go` + `debloat_catalog.json` (embutido) + `/api/debloat/lista`
  e `/remover`. Lista apps UWP **inúteis** (Notícias/Clima MSN, Groove, Clipchamp, jogos promocionais,
  3D Viewer, Get Help, Power Automate, Spotify stub…) por **nome EXATO de pacote** casado num catálogo
  curado — **nunca** componentes de sistema (a sondagem mostrou que substring pegava `PeopleExperienceHost`
  e `XboxGameCallableUI`; por isso exato). Remoção por usuário via `Remove-AppxPackage`, **reversível**
  (reinstala pela Store). Página Manutenção: lista agrupada por categoria, recomendados pré-marcados,
  **apps do Xbox desmarcados por padrão** (gamer-aware, com aviso ⚠), botão com contagem dinâmica.
  Validado: 18 bloats achados em 148 pacotes, **zero componentes de sistema** na lista.

---

## [4.0.0] — Reparo do Windows (Objetivo B #6) + correção de tradução

- **Reparo do Windows "nível formatação"** — `winutil/repair.go` + `/api/reparo/iniciar` + `/status`.
  Orquestra **SFC /scannow**, **DISM RestoreHealth**, **DISM StartComponentCleanup** e **reset do
  Windows Update** (para serviços → renomeia caches SoftwareDistribution/catroot2 com backup `.bak` →
  reinicia). Saída **ao vivo** (stream linha a linha tratando `\r` das barras de progresso) + barra de
  progresso por etapa, num console na marca. Tudo são reparos do próprio Windows — **não apagam dados**;
  exigem admin. Página **Limpeza renomeada para "Manutenção"** (hospeda limpeza + reparo). Validado: o
  console capturou a saída real do DISM (erro 740 sem admin = plumbing OK).
- **Correção do prompt de tradução do Edge** — a janela-app não oferece mais "traduzir para português":
  `<html lang="pt-BR" translate="no">` + `<meta name="google" content="notranslate">` + flags
  `--disable-translate` / `--disable-features=Translate,TranslateUI,msEdgeTranslate`.

---

## [4.0.0] — Otimização por jogo turbinada (Objetivo A #5 — fecha o Objetivo A)

- **Perfis curados por título** — `winutil/gameprofiles.go` + `gameprofiles.json` (embutido): banco de
  ~12 jogos populares (VALORANT, CS2, Fortnite, COD/Warzone, Apex, PUBG, LoL, Overwatch 2, R6, Battlefield,
  GTA V, Rocket League) com **dicas de configuração in-game testadas** (Vsync/motion blur off, texturas,
  NVIDIA Reflex/Anti-Lag, limite de FPS) + quais tweaks de sistema mais ajudam. `MatchProfile()` casa
  pelo nome/exe do jogo detectado. A página Jogos ganha um selo do tipo + botão **"🎯 Perfil"** que abre
  o guia do título, com botão "Aplicar tweaks seguros agora" (FSO/GPU/prioridade/antivírus).
- **Decisão de segurança:** as dicas são aplicadas pelo técnico no **menu do próprio jogo** — **não
  editamos arquivos de config** de jogos com anti-cheat (Vanguard/Ricochet/EAC), evitando risco de ban.
  Os perfis com anti-cheat trazem esse aviso em destaque. O usuário (especialista de FPS) pode refinar o
  JSON com seus próprios settings testados.
- Validado na máquina do usuário: Battlefield 6, Call of Duty e PUBG casaram com perfis; modal mostra as
  dicas + aviso de anti-cheat. **Objetivo A (1–5) concluído.**

---

## [4.0.0] — Plano de latência extrema + Perfis na UI (Objetivo A #4)

- **Tela de Perfis (presets) ligada** — pendência antiga da Fase 5 fechada. Cards de **Equilibrado /
  Competitivo / Streaming** no topo da página Otimizações, com "Aplicar" num clique (confirma e mostra
  o que faz; tudo reversível pelo Histórico). `/api/aplicar-preset` já existia; faltava a UI.
- **Plano de latência extrema** = o preset **Competitivo** (21 ajustes): plano de energia + processador
  100% + power throttling off + USB suspend off + SystemResponsiveness + Network Throttling off + Timer
  Resolution + Nagle off + TCP + prioridade de tarefas de jogos + o novo **Win32PrioritySeparation**.
- **2 regras novas (43 no total):** `cpu.priority-separation` (Win32PrioritySeparation=38 — favorece o
  jogo em foco, menos input lag) e `gpu.driver-keep` (SearchOrderConfig=0 — **impede o Windows de trocar
  o driver de vídeo** sozinho; complemento do #3). Ambas amarelas, reversíveis. Validadas (detect OK).

---

## [4.0.0] — Driver de GPU (Objetivo A #3)

- **Gestão de driver de GPU** — `winutil/driver.go` + `/api/driver`: lê **versão e data reais** do
  driver de cada GPU física (registro da classe de display; **versão marketing NVIDIA via NVML**, ex.:
  596.36) e calcula a idade. Filtra adaptadores virtuais (Parsec/IDD/Basic). Mostrado num card na
  página **Diagnóstico** — princípio firmado: **informa, não alarma** ("desatualizado" foi proibido).
- **Guia de instalação limpa (estilo DDU)** — modal com passo a passo (baixar driver certo → DDU →
  Modo de Segurança → instalação limpa → comprovar no Benchmark/FPS) e links por marca.
  **Decisão de segurança:** o app **não** remove o driver automaticamente — DDU exige Modo de Segurança
  e supervisão; pior, num **TeamViewer** isso derrubaria a tela/o acesso remoto. O guia avisa disso em
  destaque. Limpeza automática só dos **resíduos do instalador** (`C:\NVIDIA`/`C:\AMD`, extração temp —
  nunca o DriverStore ativo) via `/api/driver/limpar`.
- Validado na máquina do usuário: RTX 5070 driver 596.36 (~1 mês), Intel UHD 630 (integrada),
  Parsec virtual filtrado.

---

## [4.0.0] — Portabilidade: GPU universal + mais launchers

- **Uso de GPU universal (qualquer marca)** — `winutil/gpu_usage.go`: lê o uso do motor 3D via
  **contadores PDH do Windows** (`\GPU Engine(*engtype_3D)\Utilization Percentage`, a mesma fonte do
  Gerenciador de Tarefas). Agora **AMD e Intel** mostram uso real (antes só o nome). **Validado** contra
  o NVML numa RTX: em repouso 0/0, sob estresse ~94%/~90% (PDH×NVML acompanham). Temperatura segue
  específica do fabricante (NVML p/ NVIDIA; AMD/Intel = **N/A honesto**, sem inventar). A UI já trata
  temperatura ausente (não mostra "0°C").
- **Detecção de jogos além de Steam/Epic** — `launcherGames()` em `winutil/games.go`: varre o registro
  de desinstalação por **publisher de jogo** (Blizzard/Activision → Battle.net/**COD**, Electronic Arts
  → **Battlefield**, Ubisoft, Rockstar, Riot, etc.) + chaves específicas de **Ubisoft Connect** e
  **GOG**, com filtro anti-launcher/anticheat. Beneficia o diagnóstico (jogo no HDD) e a página Jogos.
  *Não regride Steam/Epic (8+3 mantidos); a detecção dos novos launchers precisa de teste num PC que
  tenha esses jogos instalados — esta máquina não tinha.*

---

## [4.0.0] — Diagnóstico de gargalos + estresse de GPU + ícone

- **Diagnóstico de gargalos (Objetivo A #2)** — `winutil/diagnose.go` + `/api/diagnostico` + página
  "Diagnóstico" (2º nó do HUD). Cruza hardware/config e aponta o que custa FPS, com dado real e
  correção: **XMP/EXPO desligado** (compara `configured_mhz` vs `rated_mhz` do SMBIOS), **jogo num HD
  mecânico** (mapeia a pasta do jogo → PhysicalDrive → SSD/HDD), **monitor abaixo da taxa máxima**
  (EnumDisplaySettings), **HAGS** (HwSchMode), **plano de energia** (ActiveScheme), espaço em disco,
  RAM, apps na inicialização, GPU dedicada em notebook. Cada achado tem severidade (crítico/atenção/
  OK), e ação: **"Corrigir agora"** (aplica a regra — ex.: `power.high-performance`,
  `win.gpu-scheduling-hags`) ou link pra página, ou passo manual (BIOS/Windows). Validado na máquina
  do usuário (achou: tela 120/144 Hz, 8 jogos no HDD, plano de energia, 11 apps no startup).
- **Benchmark agora estressa a GPU de verdade** — antes só CPU/mem/disco (parecia "fake"). Novo
  **estresse de GPU via WebGL** na janela do Edge: renderiza um fractal pesado por pixel em lotes com
  `gl.finish()` (sem cap de vsync) e mede o throughput → **score de GPU**. Mostra **uso/temperatura ao
  vivo** (NVML) enquanto martela — validado: GPU a **90% e +20°C** sob carga. Entra no **Índice** e no
  fluxo **Antes × Depois**. Página renomeada "Estresse de CPU + GPU".
- **Ícone e logo** — ícone do app refeito: isolado o **maior componente conectado** do brasão para
  remover os respingos/grunge soltos dos cantos, recortado justo e regerado `icon.ico` (16–256, LANCZOS).
  Logo do portal (home) **aumentado** (286→344 px).

---

## [4.0.0] — Medidor de FPS real in-game (PresentMon/ETW)

Recurso-âncora do "Objetivo A" (provar ganho de FPS, ex.: 70 → 120). Novo pacote `internal/fps`:
- **Captura via PresentMon** (Intel, **MIT**) embutido no `.exe` (`go:embed`, extraído em runtime pro
  `%LOCALAPPDATA%` com a licença junto) — usa **ETW da Microsoft**, **sem hook/injeção** (não toma ban).
- **Métricas profissionais** medidas de quadros reais: **FPS médio**, **1% low** e **0.1% low** (média
  dos piores 1%/0.1% — definição dos reviewers), pior/melhor quadro, **% de engasgo**, quadros e
  descartados. **Gráfico de frametime** (SVG, com linha de média). Parsing do CSV **por nome de
  coluna** (resiliente a versão); `msBetweenDisplayChange` → fallback `msBetweenPresents`.
- **Captura assíncrona** com contagem regressiva (`/api/fps/iniciar` + poll `/api/fps/status`); lista
  de jogos por janela visível (`/api/fps/jogos`, via EnumWindows, filtra shell/Edge/o próprio app).
- **Página "FPS"** (9º nó do HUD): seletor de jogo + entrada manual, duração 15/30/60s, e o fluxo
  **Antes × Depois** com ganho % por métrica. Testes unitários cobrindo percentis/1% low/parsing.
- Exige admin (ETW) — o app já roda elevado; mensagem amigável se faltar privilégio.

---

## [4.0.0] — Benchmark integrado & validação de escrita real

- **Benchmark integrado** (`winutil/benchmark.go` + `/api/benchmark` + página "Benchmark"):
  mede **CPU** single-thread e multi-thread (Mops/s, kernel misto inteiro+float, todos os núcleos
  lógicos), **banda de memória** (GB/s, copy+leitura sequencial) e **escrita de disco** (MB/s,
  sincronizada com `Sync()`/FlushFileBuffers para medir o dispositivo, não o cache). **Índice
  ThazzDraco** composto (transparente, normalizado por PC mediano). Fluxo **Antes × Depois**: marca
  uma referência, otimiza/fecha apps, roda de novo e mostra o **ganho % por métrica** (▲/▼). Tudo em
  ~1s; não precisa de admin. **Honestidade:** a leitura de disco foi deliberadamente removida porque
  ler logo após escrever bate no cache de página (RAM), não no disco — seria número enganoso
  (princípio "só dado real").
- **Validação de escrita real** do motor: ciclo **snapshot → escrita → undo** confirmado em registro
  de verdade (regra `win.transparency-off`, `HKCU`: `1 → 0` ao aplicar, `→ 1` ao desfazer, valor
  original restaurado exatamente). O caminho `HKLM`/serviços usa o mesmo código (falta só rodar
  elevado).

---

## [4.0.0] — Funcionalidades & polimento (continuação)

Lote de features e ajustes sobre a base v4.0 (resumo; detalhes nas seções anteriores):

- **Mais regras (41 no total):** +8 seguras/reversíveis (startup delay, menu delay, dicas/ads off,
  busca web off, apps em segundo plano, RemoteRegistry, NVIDIA telemetry, WMP Network).
- **Inicialização:** lista Run + pastas Startup; liga/desliga via `StartupApproved` (reversível).
- **Desempenho ao vivo:** CPU% (GetSystemTimes), RAM, top processos (psapi), uptime.
- **Saúde do PC:** espaço + uso por disco, **S.M.A.R.T.** (IOCTL predict-failure), bateria, uptime.
- **GPU:** NVIDIA real via **NVML** (temp/uso/VRAM — validado RTX 5070 40°C); nome universal via
  EnumDisplayDevices; AMD via ADL "a validar". **Breakdown de memória** (GetPerformanceInfo:
  confirmada/cache/pool paginado/**pool não-paginado**/processos) — diagnostica RAM alta vs vazamento
  de driver.
- **Relatório do cliente:** resumo imprimível/PDF (nome do cliente, antes/depois).
- **Backup & Restaurar:** `engine/backup.go` fotografa registro/serviços/powercfg/inicialização em
  `%ProgramData%\ThazzDraco\backups`; restaura tudo num clique. Botão no HUD.
- **Otimização por jogo:** detecta Steam/Epic, acha o exe principal; por jogo: **FSO off**, **GPU
  máx**, **CPU alta** (IFEO) e **excluir do antivírus** (Defender via PowerShell — única exceção
  nativa) + botão **⚡ Tudo**. Tudo reversível.
- **Cerimônia de scan:** modal cinematográfico (radar + 15 etapas + barra + revelação "X melhorias
  encontradas · N discos/apps/jogos"). A varredura real (escanear + saúde + inicialização + jogos)
  roda em paralelo — teatral **e** verdadeira.
- **Sem auto-scan:** o app abre em "Pronto para escanear" e o usuário clica (botão na home).
- **Limpeza no perfil do usuário real** (não do admin).
- **Janela maior (1380×940)** e **home responsiva por altura** (não corta o botão em escala 125/150%).
- **Atalho na área de trabalho** (`ThazzDraco Optimizer.lnk`, ícone do dragão).
- Princípio firmado: **só mostrar dado real/verificável** (cliente acompanha ao vivo) — sem
  temperatura de CPU chutada nem alarme de "driver desatualizado".

---

## [4.0.0] — Reescrita nativa em Go (executável único)

Reescrita completa do app como **um único `.exe` Go nativo, portátil e sem dependências**
(`app/dist/ThazzDraco.exe`, ~7,4 MB) — pensado para rodar em **qualquer PC** via pendrive ou
TeamViewer. Aposenta a UI WPF desenhada em PowerShell (considerada "mal feita").

### Arquitetura nova (`app/`)
- **Motor nativo** (sem WMI/PowerShell): `internal/winutil` fala direto com a API do Windows —
  registro (snapshot/escrita/undo), serviços (svc/mgr), powercfg, ponto de restauração via
  `SRSetRestorePoint`, e **perfil de hardware** por SMBIOS (RAM speed/chassi), GetSystemPowerStatus
  (bateria), EnumDisplaySettings (refresh), IOCTL seek-penalty (SSD/HDD, com o disco do **sistema**
  em 1º para o gate de SSD acertar) e GetAdaptersAddresses (DNS). Elevação via manifesto
  `requireAdministrator` (escudo UAC).
- **Motor de regras** (`internal/engine`): porta fiel do declarativo — carrega `rules.json`/
  `presets.json` **embutidos** (`go:embed`), detecta (gates, registry/foreach/powercfg/service/
  cleanup/profile-compara/dns), aplica com snapshot e desfaz por regra/lote/tudo. Histórico
  persistido em `%ProgramData%\ThazzDraco\historico.json`. Invariantes de segurança preservados.
- **Servidor** (`internal/server`): HTTP local em porta efêmera + API JSON (`/api/escanear`,
  `/api/aplicar`, `/api/aplicar-verdes`, `/api/aplicar-preset`, `/api/desfazer`, `/api/historico`).
  Watchdog encerra o processo quando a janela para de pingar (fechou).
- **UI web profissional** (`web/`): design system "Tactical Command Console" na marca (ciano +
  gunmetal + cromado + HUD), **fontes nativas do Windows** (Bahnschrift/Consolas → 100% offline).
  Gauge "Boost Score" animado, perfil do PC, cards de regra com tier/flags, presets, histórico,
  consentimento, toasts. Embutida no binário.
- **Janela-app**: abre no **Edge `--app`** (sem abas/barra, perfil isolado) com fallback ao
  navegador padrão. Sem bundlar runtime.
- **Build**: toolchain Go portátil em `E:\CLAUDE AI\_toolchain`; `app/build.ps1`. Ícone do dragão +
  manifesto embutidos via `resgen.py` (gera `resource.syso` COFF self-contained, sem windres/rsrc).

### UX multipágina (Cockpit + Seções) — escolha do usuário
Reestruturada a UI de "sidebar de ações" para **app de múltiplas páginas** (nav no topo), após
estudo de 3 protótipos com o usuário:
- **Início (Cockpit)**: gauge Boost Score, botão único **"Otimizar agora"** (aplica os seguros),
  cards **"Recomendado pra você"** (consultivas + top seguros + limpeza) e perfil do PC. Varredura
  **automática** ao abrir.
- **Otimizações (Seções)**: subnav por categoria (Energia, Jogos & Windows, GPU & Tela, Memória,
  Rede, Serviços, Sistema) com contadores; cada regra é um **toggle on/off** (ligar=aplicar,
  desligar=desfazer) + **selo de segurança** (seguro/médio/avançado). O antigo filtro
  verde/amarelo/vermelho saiu; tier virou selo.
- **Limpeza**: anel com tamanho dos temporários + "Limpar agora" + colunas "o que é apagado / o que
  NUNCA é tocado".
- **Histórico**: lotes reversíveis (desfazer lote / desfazer tudo). Botão **"Reiniciar agora"**
  (endpoint `/api/reiniciar`). Validado por screenshots das 4 páginas.

### Reestilo "Sidebar / NVIDIA + Gaming angular" (escolha do usuário)
Após estudo com 10 referências de estilo, o usuário escolheu **layout sidebar (estilo painel
NVIDIA)** com estética **Gaming RGB** (cantos cortados angulares, títulos itálico-maiúsculo,
botões/toggles chanfrados, glow), **nas cores da marca** (ciano/gunmetal — sem vermelho). Removida
a barra superior; nav vira **sidebar à esquerda** (Início/Otimizações/Limpeza/Histórico + Reescanear
+ badge admin). Conteúdo: hero angular com gauge, recomendados, categorias como **chips angulares**,
regras com **toggle chanfrado**, limpeza/histórico no mesmo idioma visual. Só `index.html` + `app.css`
mudaram; motor/servidor/`app.js` intactos. Validado por screenshots das 4 páginas com dados reais.

### Reestilo "DRACO // PORTAL" (escolha do usuário, após várias rodadas)
O usuário rejeitou sidebar/topbar e pediu algo "que impressione". Apresentadas direções
cinematográficas; escolheu o **Portal**: a tela inicial é o **dragão da marca nítido dentro de um
portal rúnico** (anel girando + score como carga + raios + partículas + chão em perspectiva), com a
**navegação num HUD de comando no rodapé** (Núcleo/Otimizações/Limpeza/Histórico — sem sidebar) e
**sem botão "otimizar tudo"** (a home é diagnóstico; o trabalho acontece no "console"). As páginas
funcionais (Otimizações = console com toggles, Limpeza, Histórico) usam o mesmo idioma cinematográfico.
Só `index.html`/`app.css` reescritos + `buildGauge/setGauge` adaptados (portal); motor/servidor/lógica
intactos. Validado por screenshots (Início + console) com dados reais; navegação confirmada via eval
(o clique do preview falha por bug da ferramenta, não do app).

### Lote de melhorias (escolha do usuário: 10,8,7,6,5,4,3,2,1)
- **#1** Raios crepitando ao redor do núcleo + **energia muda de cor com o score** (var CSS `--energy`
  tinge dragão/raios/halo: vermelho<50, ciano<70, etc.).
- **#2** **Pulso de energia** na tela ao ativar um módulo (feedback de aplicação).
- **#3** **Limpeza no perfil do usuário real** (via `ProfileImagePath` do SID) em vez do TEMP do admin.
- **#4** **Detalhes técnicos por regra**: campo `tecnico` no scan + botão ⓘ → modal "o que altera no
  sistema" (chave/valor/serviço/powercfg).
- **#5** **Modo performance** (botão no HUD) que reduz animações; auto-ativa em PC fraco
  (`hardwareConcurrency<=2`) ou `prefers-reduced-motion`; persiste em localStorage.
- **#6** **Aviso "sem administrador"** (banner) + erros mais claros.
- **#7** **Log de operações** (`%ProgramData%\ThazzDraco\operacoes.log`) com antes→depois de cada
  aplicar/desfazer e MB liberados na limpeza.
- **#8** **Layout responsivo** (media queries: portal/telemetria/recomendados se adaptam a janelas menores).
- **#10** **Score anima do valor anterior** (não do zero) + reescaneia após aplicar. (#9 teclado ficou para depois.)
- Validado por screenshots + eval (modal técnico, raios, energia, banner) sem erros de console.

### Features (lote 1: as 4 prioritárias)
- **Feature 3 — Mais regras:** +8 regras seguras e reversíveis (33 → **41**): startup-delay, menu-show-delay,
  desativar dicas/ads, busca web no Iniciar, apps em segundo plano, RemoteRegistry, telemetria NVIDIA,
  WMP Network. Canônico `rules/rules.json` sincronizado com o embutido.
- **Feature 1 — Gerenciador de inicialização:** `winutil/startup.go` lista Run (HKCU/HKLM/Wow64) +
  pastas Startup e habilita/desabilita via **StartupApproved** (igual ao Gerenciador de Tarefas, sem
  apagar nada → reversível). Página "Inicialização" com toggles + contadores. Endpoints
  `/api/inicializacao` e `/api/inicializacao/set`.
- **Feature 4 — Desempenho ao vivo:** `winutil/metrics.go` (CPU% via GetSystemTimes amostrado, RAM via
  GlobalMemoryStatusEx, top processos por working set via Toolhelp+psapi, uptime). Página "Desempenho"
  com anéis de CPU/RAM e lista de processos, atualizando a cada 2s. Endpoint `/api/metricas`.
- **Feature 2 — Relatório do cliente:** overlay imprimível (tema claro) com logo, **antes×depois** do
  score (rastreia o score inicial da sessão), perfil do PC, lista de otimizações aplicadas (do
  histórico), recomendações pendentes, nome do cliente e branding. Botão "Salvar PDF / Imprimir"
  (`window.print()` + CSS `@media print`). Barra de comando passou a 6 áreas.
- Validado por screenshots + eval das 4 telas com dados reais; sem erros de console.

### Validado
- Perfil de hardware, varredura (score 48, 33 regras), API por HTTP e UI (screenshots) conferidos
  com dados reais da máquina. Ícone extraído do `.exe` confirma o dragão. Aplicação real de escrita
  pende teste do usuário em runtime elevado.

---

## [Não lançado]

### Documentação
- Criada a pasta `docs/` com a base de documentação do projeto: `README`, `ARCHITECTURE`,
  `ROADMAP`, `SCAN-ENGINE`, `OPTIMIZATIONS`, `CONVENTIONS`, `CHANGELOG`.
- Registrada a visão de evolução para **plataforma de otimização** (scan completo + recomendação
  consciente do hardware + aplicação reversível).
- Criado o **guia mestre** [`GUIA-OTIMIZACAO-GAMING.md`](GUIA-OTIMIZACAO-GAMING.md): catálogo
  completo (20 partes) de tudo o que é possível otimizar para jogos, com selo de segurança
  (🟢/🟡/🔴) por item e seções de "o que nunca é tocado" e "limpeza segura".

### Motor de regras (início da Fase 2)
- Definido o contrato das regras declarativas em [`RULES-SCHEMA.md`](RULES-SCHEMA.md): campos,
  3 modos (acionável/consultivo/limpeza), tiers → comportamento, `hardware_gate`, tipos de
  operação e o **mecanismo de snapshot + undo** que torna a reversibilidade estrutural.
- Criado `rules/rules.json` (lote 1): **14 regras 🟢** derivadas do guia — 11 acionáveis
  (energia, registro, GPU, rede, input), 1 limpeza com consentimento e 2 consultivas
  (XMP/EXPO e taxa de atualização do monitor). Cada regra rastreia a `parte` do guia de origem.
- Criado **`motor.ps1`** — núcleo novo (abordagem strangler): reusa a detecção de SID do usuário
  real (de `coletar.ps1`) e as queries CIM/WMI (de `metricas.ps1`), monta um mini-perfil do PC
  (contexto + RAM + display) e roda o passo **`detect`** de todas as regras → gera `estado.json`
  com estado por regra e **score de fonte única**. Somente leitura.
- Criado **`Escanear.bat`** — gatilho elevado (UAC) que roda o `motor.ps1`.
- Validado contra hardware real: detect de registro/powercfg/cleanup/perfil funcionando; corrigido
  bug de contagem (PowerShell retorna escalar quando `Where-Object` casa 1 item → `.Count` dava o
  nº de chaves do dicionário; resolvido com `@(...)`).
- Extraídas as funções compartilhadas para **`lib.ps1`** (detecção de SID, registro, powercfg,
  gates, `Detect-Rule`, perfil) e `motor.ps1` refatorado para usá-la — sem duplicação.
- Criado **`executor.ps1`** — passo **APPLY/UNDO** com rede de segurança: **ponto de restauração**
  + **snapshot** do estado anterior antes de qualquer escrita; `undo` reaplica o snapshot. Suporta
  `-Todas`, `-Ids`, `-Desfazer`, `-DryRun`, `-Confirmar`, `-PularRestorePoint`. Invariantes
  aplicados: consultivas não são aplicadas, limpeza só na allowlist e com consentimento, regra que
  toque dado pessoal é bloqueada. Validado em `-DryRun` contra o sistema real (snapshot e diff
  antes→depois corretos; já-aplicadas puladas).
- Criados **`Aplicar.bat`** (aplica as verdes, com confirmação + restore point) e **`Desfazer.bat`**
  (reverte o último lote). Backups por lote em `backup/<timestamp>/`.

### Catálogo expandido + Dashboard do motor
- Catálogo ampliado para **28 regras** (15 🟢, 11 🟡, 2 🔴) cobrindo energia, registro, memória,
  serviços, GPU e exibição. Adicionada a operação **`service`** (detect/apply/undo) e os campos de
  perfil **`discos[].tipo`** (SSD/HDD) e **`hardware.ram.total_gb`** para gates conscientes de hardware.
- **Removida a regra de Core Parking:** o setting é oculto no powercfg e não permite snapshot
  confiável → sem undo garantido. Mantido o invariante de reversibilidade (mesma razão adia
  MSI-mode e bcdedit, que ficam para quando houver snapshot/undo confiável).
- `executor.ps1` regenera o `estado.json` após aplicar, para o dashboard refletir o novo estado.
- Criado o **dashboard novo `painel.html`** (lê `estado.json`): score em anel, perfil do PC, lista
  de regras com selo 🟢/🟡/🔴 + estado, e botões Escanear / Aplicar Verdes / Desfazer (padrão
  modal→.bat→polling). **`Iniciar.bat` agora abre o `painel.html`.** Verificado funcionalmente
  (carrega via HTTP e renderiza as 28 regras); `.claude/launch.json` para preview local.

### UI interativa, retirada do legado e mais regras
- **Modo interativo:** `servidor.ps1` ganhou o endpoint **`/api/run`** (ping/escanear/aplicar/
  aplicar-verdes/desfazer) que roda motor/executor sob demanda (o servidor já está elevado). O
  `painel.html` agora faz **Escanear / Aplicar Verdes / Desfazer com 1 clique**, tem **checkboxes**
  para selecionar regras 🟡/🔴 e **"Aplicar selecionadas"** (confirma se houver vermelhas), com
  **fallback** para modal→.bat quando o endpoint não existe. `Iniciar.bat` força o servidor
  PowerShell. Verificado: `ping` + `escanear` via HTTP retornaram o estado (33 regras).
- `executor.ps1` ganhou **`-IdsCsv`** (usado pela UI/endpoint).
- **Catálogo → 33 regras** (17 🟢, 14 🟡, 2 🔴): + parâmetros TCP, **Nagle por interface**, serviços
  inúteis (Fax/RetailDemo/WAP Push), geolocalização/mapas e **DNS rápido** (consultivo).
- Novo tipo de operação **`registry-foreach`** (apply/snapshot/undo em todas as subchaves de um
  caminho — ex.: Nagle em cada interface) e detect **`dns-check`** (recomenda DNS rápido). O perfil
  agora inclui o DNS atual.
- **Legado aposentado:** `FPS_Optimizer_Script.ps1` movido para **`legacy/`**; `2_Otimizar.bat`
  virou aviso de descontinuação apontando para o fluxo novo (seguro e reversível).

### App nativo (janela WPF) — nova entrada principal
- Criados **`ThazzDraco.ps1`** + **`ThazzDraco.bat`**: um **app nativo de janela única** (WPF em
  PowerShell, sem instalar nada). Abre **uma janela** que faz tudo **dentro dela** — Escanear,
  Aplicar Verdes, **Aplicar Selecionadas** (checkboxes 🟡/🔴) e Desfazer — **sem navegador, sem
  servidor, sem .bat**. Reaproveita `motor.ps1`/`executor.ps1` em segundo plano (DispatcherTimer
  atualiza a tela sem congelar). Console escondido, auto-elevação UAC, avisa se o ponto de
  restauração falhar. Passou no teste de construção headless (STA, `-CheckOnly`).
- `ThazzDraco.bat` é a **entrada principal**; o painel web (`Iniciar.bat`/`painel.html`) vira alternativa.
- **Design profissional (1ª passada):** janela **sem moldura** com cantos arredondados + sombra
  flutuante, **barra de título customizada** (minimizar/fechar, arrastável), **score em anel**
  desenhado (arco real), **ícones** (Segoe MDL2), **hover/press** nos botões, **hover** nas linhas,
  paleta/tipografia refinadas e **fade-in** na abertura. Construção verificada headless (STA).
- **Design 2ª passada (UX):** **busca por texto**, **tooltips** por regra, **descrição humana** no
  card (o texto técnico foi pro tooltip), **contadores nos filtros**, **marcar/limpar visíveis**,
  anel de score com brilho/contraste e o estado **"pendente" em azul** (não mais vermelho-alarme).
  `motor.ps1` passou a incluir `descricao` no `estado.json`. (Escaneamento é manual — só ao clicar.)

### App nativo — pacote completo de recursos
- **Janela redimensionável** (WindowChrome) com **ícone próprio** na barra de tarefas (gerado em memória).
- **Tema claro/escuro** (toggle, via DynamicResource).
- **Perfis/Presets** em `rules/presets.json` (**Equilibrado**, **Competitivo**, **Streaming**) —
  aplicam um conjunto curado de regras em 1 clique (reusam o executor, com ponto de restauração).
- **Histórico de lotes** — janela que lista os lotes aplicados e permite **desfazer um específico**.
- **Aplicar regra individual** direto no card (botão "Aplicar" por linha).
- **Animação** do anel de score (conta de 0 ao valor) e **banner de reinício** (com botão
  *Reiniciar agora*) quando alguma mudança aplicada exige reboot.
- Criado **`build-exe.ps1`**: empacota o app como **`ThazzDraco.exe`** com ícone (via ps2exe).
  Construção do app verificada headless (STA); runtime das telas pende validação do usuário.

### Painel web redesenhado (identidade "TELEMETRY")
- `painel.html` refeito com direção estética própria: **cluster de instrumentos de tuning**.
  **Tacômetro** em SVG para o score (agulha animada + ticks + arco), **leitura de ECU** pros dados
  do PC, **LEDs de diagnóstico** por nível de risco, fundo blueprint + grão + brilho âmbar de sódio,
  tipografia **Chakra Petch + JetBrains Mono**, painéis com cantos chanfrados (clip-path), perfis e
  "aplicar individual" integrados. Continua 100% funcional (endpoint `/api/run` + fallback modal).
  Verificado no preview: gauge correto e 33 canais renderizando.
- **Identidade "TELEMETRY" portada para o app nativo** (`ThazzDraco.ps1`): tacômetro de instrumento
  (arco 270° + ticks + agulha) para o score, paleta âmbar-de-sódio / fósforo / void, **LEDs de
  diagnóstico com glow** nas linhas, fundo blueprint (DrawingBrush) e tipografia técnica
  **Bahnschrift + Consolas** (fontes nativas, sem dependência). Toda a lógica preservada. Adicionado
  o modo `-RenderPng` (renderiza a janela para PNG headless) para validação visual; verificado.
- **Reskin para a IDENTIDADE DA MARCA** (@thazdracofpsotimizacao): paleta **ciano elétrico #2ea8e6 +
  gunmetal navy + cromado**, **raio** (lightning do dragão) no header, wordmark cromado "THAZZDRACO"
  + tagline "PC FPS BOOST", **molduras de HUD** (cantos em L) no tacômetro, grid de aço. Substitui o
  TELEMETRY âmbar. Verificado via `-RenderPng`.
- **Logo real embarcado:** fundo cinza removido (flood-fill pelas bordas + erosão/feather via
  Pillow) → `assets/logo.png` transparente. O app carrega automaticamente no **header** (no lugar
  do wordmark) e como **ícone** da janela/taskbar; fallback pro raio+wordmark se o arquivo faltar.
  Acento **laranja** (#ff7e30) adicionado ao raio do header, fiel ao logo.
- **Layout reorganizado para sidebar** (a pedido): menu lateral à esquerda (logo no topo + AÇÕES +
  PERFIS + MAIS) e conteúdo à direita (score/perfil + filtros + lista). **Fundo preto** com textos
  nas cores da marca. Corrigido o modo `-RenderPng` para levar os `Resources` junto ao desanexar a
  janela (sem isso, elementos com `DynamicResource` saíam invisíveis no PNG de validação).

### Conhecido (pendente)
- **Bug de contrato:** tabela "Top Processos" não renderiza (`processos` vs `m.top_processos`).
- **Bug de contrato:** card "Sistema" não renderiza (`sistema.uptime_txt` vs `m.uptime.texto`).
- Score duplicado entre `coletar.ps1` e `Dashboard.html`.
- Sem ponto de restauração / undo nas otimizações.

---

## [3.0] — Dashboard + Métricas (versão atual)

### Adicionado
- **Dashboard web** (`Dashboard.html`) com assistente de 4 fases (Antes / Otimizar / Validar /
  Métricas), tema "gamer", score em anel, comparativo antes/depois e estimativa de FPS.
- **Servidor local** (`servidor.ps1`) como fallback quando não há Python; launcher `Iniciar.bat`.
- **Coletor de snapshot** (`coletar.ps1`) com score de 16 checks e detecção do SID do usuário real.
- **Métricas de hardware** (`metricas.ps1`): CPU, RAM (por pente), GPU, discos (SSD/HDD), rede,
  top processos; suporte a temperatura via LibreHardwareMonitor/OpenHardwareMonitor.
- **Bloco avançado de otimização** no `FPS_Optimizer_Script.ps1`: Core Parking, C-States, Power
  Throttling, MSI Mode (GPU/NVMe), Interrupt Moderation, prioridade por jogo (IFEO), MPO, HDCP,
  GPU Preemption.

---

## [1.1] — Otimizador base

### Adicionado
- `FPS_Optimizer_Script.ps1` com 10 etapas: plano de energia, serviços, tweaks de registro, rede,
  GPU/HAGS, memória, limpeza de temporários, startup apps, mouse, flush DNS.
- Correção de encoding (scripts 100% ASCII, sem acentos).

---

## Convenção

Ao subir uma versão, mover os itens de **[Não lançado]** para uma nova seção `[x.y]` com a data, e
registrar Adicionado / Alterado / Corrigido / Removido. Ver [CONVENTIONS.md](CONVENTIONS.md#versionamento).
