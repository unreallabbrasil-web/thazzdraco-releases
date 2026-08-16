# Guia Completo de Otimização para Jogos

> Catálogo mestre de **absolutamente tudo** o que é possível fazer para extrair o máximo de
> desempenho de um PC em jogos — do mais seguro ao mais avançado.
>
> Este é o documento de referência (o "universo" de otimizações). O subconjunto que o script
> atual já automatiza está em [OPTIMIZATIONS.md](OPTIMIZATIONS.md).

---

## 🔒 Promessa de segurança

Tudo neste guia respeita três regras inegociáveis:

1. **Nunca toca em documentos pessoais.** Fotos, vídeos, downloads, saves de jogos, projetos,
   área de trabalho, e-mails — nada disso é lido, movido ou apagado. Ver
   [O que NUNCA é tocado](#-o-que-nunca-é-tocado).
2. **Nunca apaga nada sem consentimento.** Qualquer limpeza é restrita a arquivos temporários e
   cache (descartáveis por natureza) e só com confirmação. Ver
   [Limpeza segura](#-limpeza-segura-o-que-pode-e-o-que-não-pode).
3. **Sempre reversível.** Antes de qualquer mudança no sistema: ponto de restauração + anotação do
   valor anterior. Ver [Parte 0](#parte-0--antes-de-começar-rede-de-segurança).

## Como ler este guia — classificação de segurança

| Selo | Significado |
|---|---|
| 🟢 **Seguro** | Sem risco prático, reversível em segundos, não afeta nada pessoal. Pode aplicar à vontade. |
| 🟡 **Cuidado** | Reversível, mas tem contrapartida, depende do hardware ou precisa de configuração correta. |
| 🔴 **Avançado** | Pode causar instabilidade, mexe em segurança/garantia ou exige conhecimento. Só com consciência do risco. |

Cada item indica também, quando relevante: **(reboot)** se exige reiniciar, e **(hardware)** se é
físico/BIOS.

## Índice

- [Parte 0 — Antes de começar (rede de segurança)](#parte-0--antes-de-começar-rede-de-segurança)
- [Parte 1 — Configurações do Windows](#parte-1--configurações-do-windows-as-mais-seguras)
- [Parte 2 — Energia e CPU](#parte-2--energia-e-cpu)
- [Parte 3 — GPU: drivers e painel](#parte-3--gpu-drivers-e-painel)
- [Parte 4 — Monitor e exibição](#parte-4--monitor-e-exibição)
- [Parte 5 — Memória / RAM](#parte-5--memória--ram)
- [Parte 6 — Armazenamento / Disco](#parte-6--armazenamento--disco)
- [Parte 7 — Serviços do Windows](#parte-7--serviços-do-windows)
- [Parte 8 — Inicialização e segundo plano](#parte-8--inicialização-e-segundo-plano)
- [Parte 9 — Rede e latência](#parte-9--rede-e-latência)
- [Parte 10 — Registro do Windows](#parte-10--registro-do-windows)
- [Parte 11 — Timers, kernel e boot](#parte-11--timers-kernel-e-boot)
- [Parte 12 — Mouse, teclado e input lag](#parte-12--mouse-teclado-e-input-lag)
- [Parte 13 — Por jogo / launcher](#parte-13--por-jogo--launcher)
- [Parte 14 — BIOS / UEFI](#parte-14--bios--uefi-hardware)
- [Parte 15 — Refrigeração e térmica](#parte-15--refrigeração-e-térmica-hardware)
- [Parte 16 — Overclock e undervolt](#parte-16--overclock-e-undervolt-avançado)
- [Parte 17 — Manutenção segura](#parte-17--manutenção-segura-do-windows)
- [Parte 18 — Monitorar e medir](#parte-18--monitorar-e-medir-o-ganho)
- [Parte 19 — Avançado / entusiasta](#parte-19--avançado--entusiasta)
- [Parte 20 — O que EVITAR](#parte-20--o-que-evitar-mitos-e-perigos)
- [O que NUNCA é tocado](#-o-que-nunca-é-tocado)
- [Limpeza segura](#-limpeza-segura-o-que-pode-e-o-que-não-pode)
- [Checklist por nível](#-checklist-rápido-por-nível-de-segurança)

---

## Parte 0 — Antes de começar (rede de segurança)

| | Item | O que faz |
|---|---|---|
| 🟢 | **Criar ponto de restauração** | `Criar um ponto de restauração` no Windows. Permite voltar atrás se algo der errado. **Faça sempre antes de tweaks.** |
| 🟢 | **Anotar valores atuais** | Salvar o estado antes de mudar (o `coletar.ps1` já faz isso para os tweaks do script). |
| 🟢 | **Exportar chaves do registro** | `regedit` → exportar a chave antes de editá-la = backup pontual. |
| 🟡 | **Imagem do sistema (opcional)** | Backup completo do disco (ferramentas como Macrium Reflect). Para quem quer rede total. |
| 🟢 | **Medir o "antes"** | Rodar um benchmark/jogo e anotar FPS, 1% low e temperaturas para comparar depois (ver [Parte 18](#parte-18--monitorar-e-medir-o-ganho)). |

---

## Parte 1 — Configurações do Windows (as mais seguras)

Tudo aqui é feito pela interface gráfica do Windows, reversível e sem risco.

| | Item | Onde / o que faz |
|---|---|---|
| 🟢 | **Game Mode** | *Configurações → Jogos → Modo Jogo: Ativado.* Prioriza recursos para o jogo. |
| 🟢 | **HAGS (GPU Scheduling)** | *Configurações → Tela → Gráficos → Configurações gráficas padrão → Agendamento de GPU acelerado por hardware.* Pode reduzir input lag. **(reboot)** |
| 🟢 | **VRR (Variable Refresh Rate)** | Mesma tela de Gráficos. Ajuda com G-SYNC/FreeSync. |
| 🟢 | **Otimizações para janelas** | Liga otimização de jogos em janela/borderless (Win11), reduz latência no modo janela. |
| 🟢 | **Game DVR / Xbox Game Bar** | *Configurações → Jogos → Capturas:* desligar gravação em segundo plano. Tira overhead. |
| 🟢 | **Preferência de GPU por app** | *Gráficos → escolher o jogo → Alto desempenho.* Útil em notebooks com 2 GPUs. |
| 🟢 | **Plano de energia** | *Alto Desempenho* (ver [Parte 2](#parte-2--energia-e-cpu)). |
| 🟢 | **Efeitos visuais → Desempenho** | *Ajustar aparência e desempenho do Windows → Melhor desempenho.* Desliga animações. |
| 🟢 | **Transparência off** | *Personalização → Cores → Efeitos de transparência: Desligado.* |
| 🟢 | **Apps em segundo plano** | *Configurações → Apps:* desligar apps que rodam em background sem necessidade. |
| 🟢 | **Assistente de Foco / Não Perturbe** | Silencia notificações durante o jogo (evita stutter de pop-ups). |
| 🟡 | **Storage Sense** | *Sistema → Armazenamento.* Útil para limpar temporários **automaticamente** — mas configure para **não** apagar Downloads/Lixeira sem querer. Ver [Limpeza segura](#-limpeza-segura-o-que-pode-e-o-que-não-pode). |
| 🟢 | **Telemetria/diagnóstico mínimo** | *Privacidade → Diagnóstico:* reduzir para o mínimo. |

---

## Parte 2 — Energia e CPU

| | Item | O que faz |
|---|---|---|
| 🟢 | **Plano Alto Desempenho** | Evita que a CPU reduza clock à toa. `powercfg /setactive ...` |
| 🟡 | **Plano Máximo Desempenho (Ultimate)** | Plano oculto, ainda mais agressivo. Mais consumo/calor. **Em notebook na bateria: evitar.** |
| 🟢 | **Min/Max do processador 100%** | Mantém a CPU em clock alto. Ótimo em desktop; em notebook esquenta mais. |
| 🟡 | **Core Parking off** | Mantém todos os núcleos ativos. Ajuda em alguns jogos, aumenta consumo. |
| 🟡 | **Power Throttling off** | Impede o Windows de limitar a frequência de apps. |
| 🟢 | **USB Selective Suspend off** | Evita que portas USB "durmam" (mouse/teclado). |
| 🟡 | **PCIe Link State Power Management off** | Mantém a GPU/dispositivos PCIe em desempenho pleno. |
| 🔴 | **C-States (registro/BIOS)** | Reduz latência mantendo a CPU "acordada". Tweak controverso; melhor feito no BIOS. |
| 🔴 | **Afinidade de CPU por jogo** | Fixar o jogo em núcleos específicos (Process Lasso). Avançado; pode piorar se mal feito. |

> 💡 O **maior ganho de CPU em jogos** costuma vir do **XMP/EXPO da RAM** (Parte 5) e de bom
> resfriamento (Parte 15), não desses tweaks de energia.

---

## Parte 3 — GPU: drivers e painel

### Drivers
| | Item | O que faz |
|---|---|---|
| 🟢 | **Driver atualizado** | A maior fonte de FPS "de graça". Baixar do site da NVIDIA/AMD/Intel. |
| 🟡 | **Instalação limpa / DDU** | Remover driver antigo com *Display Driver Uninstaller* antes de instalar. Resolve stutter/conflitos. |
| 🟢 | **Limpar cache de shaders** | Resolve stutter após troca de driver. |

### NVIDIA (Painel de Controle / app NVIDIA)
| | Item | Valor recomendado p/ jogos |
|---|---|---|
| 🟢 | **Power Management Mode** | *Prefer Maximum Performance* |
| 🟢 | **Low Latency Mode** | *On* ou *Ultra* (reduz input lag; com G-SYNC use *On*) |
| 🟢 | **Max Frame Rate** | Cap ~3 FPS abaixo do refresh com G-SYNC (ex.: 141 num monitor 144 Hz) |
| 🟢 | **Vertical Sync** | *Off* no painel se usa G-SYNC + cap (ou *On* no painel + *Off* no jogo) |
| 🟡 | **Texture Filtering – Quality** | *High Performance* (troca um pouco de qualidade por FPS) |
| 🟡 | **Threaded Optimization** | *On* (auto costuma bastar) |
| 🟢 | **Shader Cache Size** | *Ilimitado* ou 10 GB+ |
| 🟢 | **G-SYNC** | Habilitar para tela compatível (elimina tearing sem input lag de VSync) |
| 🔴 | **NVIDIA Profile Inspector** | Ajustes finos por jogo (ex.: limitar pré-renderização). Avançado. |

### AMD (Adrenalin)
| | Item | O que faz |
|---|---|---|
| 🟢 | **Radeon Anti-Lag** | Reduz input lag |
| 🟡 | **Radeon Boost** | Reduz resolução em movimento rápido por mais FPS |
| 🟡 | **Image Sharpening / RSR** | Nitidez / super resolução para ganhar FPS |
| 🟢 | **Enhanced Sync** | Alternativa ao VSync com menos lag |
| 🟡 | **Surface Format Optimization** | Pequeno ganho de desempenho |
| 🟡 | **Tessellation → AMD optimized** | Reduz carga de tesselação |
| 🟢 | **Radeon Chill** | Economiza energia/calor limitando FPS quando parado (opcional) |

### Intel Arc
| | Item | O que faz |
|---|---|---|
| 🟢 | **Intel Arc Control / Graphics Command Center** | Perfis de desempenho, atualização de driver, telemetria. |

### Recursos transversais
| | Item | O que faz |
|---|---|---|
| 🟡 | **MSI Mode para GPU** | Interrupções por mensagem = menor latência. Tweak de registro. |
| 🟢 | **Resizable BAR / Smart Access Memory** | Ganho real em vários jogos. Ativar no BIOS + driver. **(hardware/reboot)** |
| 🟡 | **Desligar MPO (Multiplane Overlay)** | Corrige stutter/flicker em alguns sistemas NVIDIA. |
| 🟡 | **Upscaling (DLSS/FSR/XeSS)** | Maior FPS por renderizar em resolução menor. Por jogo. |

---

## Parte 4 — Monitor e exibição

| | Item | O que faz |
|---|---|---|
| 🟢 | **Taxa de atualização máxima** | *Configurações → Tela → Avançado.* Erro clássico: monitor 144 Hz rodando a 60 Hz! |
| 🟢 | **G-SYNC / FreeSync / VRR** | Elimina tearing sem o lag do VSync. |
| 🟢 | **Cabo DisplayPort** | Geralmente suporta refresh/resolução maiores que HDMI antigo. **(hardware)** |
| 🟡 | **Formato de cor / profundidade** | RGB full, 8/10-bit conforme o monitor. |
| 🟡 | **HDR** | Só se o monitor for bom em HDR; senão pode piorar a imagem. |
| 🟢 | **Resolução nativa** | Rodar na resolução nativa do painel evita borrão (use upscaling no jogo, não no Windows). |
| 🔴 | **HDCP off** | Libera ciclos da GPU, **mas quebra vídeo protegido (Netflix/Prime/Disney+).** Só se não usa streaming nesse PC. |
| 🟡 | **Overclock de monitor** | Subir o refresh acima do nominal (alguns painéis aceitam). Risco baixo, teste estabilidade. |

---

## Parte 5 — Memória / RAM

| | Item | O que faz |
|---|---|---|
| 🟢 | **XMP / EXPO / DOCP no BIOS** | **O tweak de RAM mais importante.** Faz a RAM rodar na velocidade anunciada (ex.: 3200/6000 MHz em vez de 2133). Ganho enorme, principalmente em Ryzen. **(BIOS/reboot)** |
| 🟢 | **Dual channel** | Usar pentes em pares nos slots corretos dobra a banda. **(hardware)** |
| 🟡 | **Page file gerenciado** | Manter o arquivo de paginação ativo (não desligar!). Tamanho gerenciado pelo sistema costuma bastar. |
| 🟡 | **DisablePagingExecutive** | Mantém o kernel na RAM (requer RAM sobrando). |
| 🟡 | **Limpar standby list (ISLC)** | *Intelligent Standby List Cleaner* reduz micro-stutter em alguns jogos limpando a memória standby. |
| 🟡 | **Prefetch/Superfetch** | Desativar **só faz sentido em SSD**; em HDD ajuda. *Gating* por tipo de disco. |
| 🔴 | **Timings manuais da RAM** | Tuning fino de latência (CL, tRCD...). Para entusiastas, exige teste de estabilidade. |
| 🟢 | **Mais RAM** | 16 GB é o piso confortável hoje; 32 GB ideal para jogos + background. **(hardware)** |

---

## Parte 6 — Armazenamento / Disco

| | Item | O que faz |
|---|---|---|
| 🟢 | **Jogos no SSD/NVMe** | Carregamento muito mais rápido e menos stutter de streaming de texturas. |
| 🟢 | **Manter espaço livre** | Deixar ~10–15% livre no SSD preserva desempenho. |
| 🟢 | **TRIM ativo** | Mantém o SSD rápido. Normalmente já vem ligado. |
| 🟢 | **Modo AHCI** | Garantir AHCI/NVMe (não IDE) no BIOS. **(BIOS)** |
| 🟢 | **DirectStorage** | Em jogos compatíveis (Win11 + NVMe), acelera carregamento. |
| 🟡 | **Não desfragmentar SSD** | SSD **não** se desfragmenta (só TRIM). HDD pode desfragmentar. |
| 🟡 | **Write caching** | Cache de escrita ligado pode melhorar desempenho do disco. |
| 🟢 | **Índice de busca no drive de jogos** | Desligar indexação no drive só de jogos reduz I/O. |
| 🟢 | **Limpeza de temporários** | Apenas temp/cache, com consentimento — ver [Limpeza segura](#-limpeza-segura-o-que-pode-e-o-que-não-pode). |

---

## Parte 7 — Serviços do Windows

> 🟡 Desativar serviços dá ganho pequeno e tem pegadinhas. Faça com critério e de forma reversível.

| | Serviço | Observação |
|---|---|---|
| 🟢 | **DiagTrack** (telemetria) | Seguro desativar. |
| 🟡 | **SysMain** (Superfetch) | Desativar só em SSD; em HDD pode ajudar. |
| 🟡 | **WSearch** (Windows Search) | Desativar **quebra a busca do menu Iniciar**. |
| 🟡 | **Xbox** (XblAuthManager, XblGameSave, XboxNetApiSvc) | **Não** desative se usa Game Pass / Xbox app. |
| 🟡 | **Print Spooler** | Só desative se **não** usa impressora. |
| 🟢 | **Fax, RetailDemo, MapsBroker** | Inúteis para a maioria; seguros. |
| 🟡 | **lfsvc** (geolocalização), **PhoneSvc** | Seguro para a maioria; afeta apps que usam localização/celular. |

---

## Parte 8 — Inicialização e segundo plano

| | Item | O que faz |
|---|---|---|
| 🟢 | **Limpar startup** | *Gerenciador de Tarefas → Inicializar:* desativar o que não precisa abrir com o Windows. Acelera o boot e libera RAM. |
| 🟢 | **Tarefas agendadas** | Revisar atualizadores/telemetria agendados (com cuidado). |
| 🟡 | **Overlays** | Discord, GeForce Experience, Steam, MSI Afterburner — **overlays podem causar stutter**. Teste desligar. |
| 🟡 | **Software de RGB** | Apps de iluminação (iCUE, etc.) consomem CPU em background. |
| 🟢 | **Fechar navegador ao jogar** | Abas de Chrome/Edge comem RAM e CPU. |
| 🟡 | **OneDrive / sync** | Pausar sincronização durante o jogo (sem desinstalar). |
| 🟢 | **Bandeja do sistema** | Reduzir apps residentes desnecessários. |

---

## Parte 9 — Rede e latência

| | Item | O que faz |
|---|---|---|
| 🟢 | **Cabo (Ethernet) > Wi-Fi** | Menor latência e jitter. O melhor "tweak" de rede. **(hardware)** |
| 🟢 | **Driver da placa de rede atualizado** | Estabilidade e desempenho. |
| 🟡 | **Nagle's Algorithm off** | Menos latência em jogos (pode reduzir eficiência em downloads). |
| 🟢 | **DNS rápido** | Trocar para Cloudflare `1.1.1.1` ou Google `8.8.8.8`. |
| 🟡 | **Desativar throttling de rede** | `NetworkThrottlingIndex` para jogos. |
| 🟡 | **Interrupt Moderation da NIC** | Desativar pode reduzir latência (aumenta uso de CPU). |
| 🟢 | **Delivery Optimization** | Impedir o Windows de usar sua banda para distribuir updates a outros PCs. |
| 🟢 | **Pausar Windows Update ao jogar** | Evita download/instalação no meio da partida. |
| 🟡 | **QoS no roteador** | Priorizar o tráfego do PC/jogo no roteador. **(rede)** |
| 🔴 | **Desativar IPv6** | Às vezes resolve problemas específicos; pode quebrar outras coisas. Situacional. |
| 🟢 | **Diagnóstico** | `ping`, `tracert`, teste de jitter para achar a real fonte de lag. |

---

## Parte 10 — Registro do Windows

> Todos exigem cuidado e backup, mas são reversíveis. Os valores exatos estão em
> [OPTIMIZATIONS.md](OPTIMIZATIONS.md).

| | Tweak | O que faz |
|---|---|---|
| 🟢 | **Game DVR / AppCapture off** | Tira overhead de gravação. |
| 🟡 | **SystemResponsiveness = 0** | Dá mais prioridade a apps de primeiro plano sobre tarefas multimídia agendadas. |
| 🟡 | **GPU Priority / Scheduling = High** | Prioriza tarefas de jogo. |
| 🟡 | **Win32PrioritySeparation** | Ajusta como o Windows divide tempo de CPU (quantum) a favor do primeiro plano. |
| 🟢 | **Mouse acceleration off** | Input "raw" e consistente (ver [Parte 12](#parte-12--mouse-teclado-e-input-lag)). |
| 🟡 | **NetworkThrottlingIndex** | Remove o limite de throughput de rede em multimídia. |

---

## Parte 11 — Timers, kernel e boot

> 🔴 Os mais controversos. Podem ajudar **ou** causar instabilidade dependendo do sistema. Teste um
> de cada vez e tenha como reverter.

| | Tweak | O que faz |
|---|---|---|
| 🟡 | **Timer Resolution alto** | Resolução de timer de ~0,5 ms. Ajuda input/latência. |
| 🔴 | **HPET on/off** | Efeito varia por máquina — testar nos dois estados e medir. |
| 🔴 | **Dynamic Tick / Platform Tick (bcdedit)** | Reduz interrupções; pode instabilizar. |
| 🟡 | **Fast Startup** | Ligar acelera o boot; **desligar** resolve alguns bugs de driver/estado. Teste. |
| 🟡 | **MSI Mode (GPU/NVMe)** | Interrupções por mensagem = menor latência. |
| 🔴 | **Interrupt Affinity** | Mover IRQs de GPU/NIC para fora do núcleo 0. Entusiasta. |

---

## Parte 12 — Mouse, teclado e input lag

| | Item | O que faz |
|---|---|---|
| 🟢 | **Desligar "Aprimorar precisão do ponteiro"** | Remove aceleração de mouse → mira consistente. |
| 🟢 | **Polling rate 1000 Hz+** | No software do mouse. Mais atualizações por segundo. |
| 🟢 | **Raw input no jogo** | Ativar onde existir a opção. |
| 🟢 | **DPI nativo do sensor** | Evitar interpolação de DPI estranho. |
| 🟢 | **Desligar Sticky/Filter Keys** | Evita pop-ups e atrasos de tecla. |
| 🟢 | **Cabo/2.4 GHz > Bluetooth** | Menor latência de periférico. **(hardware)** |
| 🟢 | **Firmware do mouse/teclado** | Atualizar firmware pode melhorar consistência. |

---

## Parte 13 — Por jogo / launcher

| | Item | O que faz |
|---|---|---|
| 🟢 | **Fullscreen exclusivo** | Geralmente o **menor input lag** vs. borderless. |
| 🟢 | **VSync off no jogo (com G-SYNC + cap)** | Combinação ideal: G-SYNC + limite de FPS + VSync off no jogo. |
| 🟢 | **Cap de FPS** | Limitar abaixo do refresh estabiliza frametime e reduz lag com VRR. |
| 🟢 | **Baixar configs pesadas** | Sombras, reflexos, volumetria, motion blur, ambient occlusion costumam custar muito FPS. |
| 🟢 | **Launch options** | Ex.: parâmetros de inicialização na Steam por jogo. |
| 🟢 | **Arquivos de config** | Ex.: `autoexec.cfg` em CS2/Source. |
| 🟢 | **Verificar integridade** | Reparar arquivos do jogo (Steam/Epic) resolve crashes/stutter. |
| 🟡 | **Prioridade do processo** | *Alta* para o jogo (não *Tempo real*). Pode automatizar com Process Lasso. |
| 🟢 | **Atualizar o jogo** | Patches frequentemente trazem otimização. |

---

## Parte 14 — BIOS / UEFI (hardware)

> **(hardware/reboot)** — feito fora do Windows. Reversível voltando a configuração.

| | Item | O que faz |
|---|---|---|
| 🟢 | **XMP / EXPO / DOCP** | Velocidade correta da RAM. **Maior ganho de plataforma.** |
| 🟢 | **Resizable BAR / Above 4G Decoding** | Ganho real em vários jogos modernos. |
| 🟡 | **Atualizar BIOS** | Melhor compatibilidade/estabilidade (siga o manual; risco se interromper). |
| 🟡 | **PBO (Ryzen) / limites de potência** | Mais boost de CPU com bom resfriamento. |
| 🟡 | **Desativar dispositivos não usados** | Controladores/portas que você não usa. |
| 🔴 | **Ajustes de C-States/energia da CPU** | Lugar "certo" para mexer em C-States (vs. registro). Avançado. |
| 🟡 | **Secure Boot / TPM** | **Necessários para o anti-cheat de vários jogos** (Valorant/Vanguard). Não desative se joga esses. |
| 🟢 | **Perfil de fan / Smart Fan** | Curvas de ventoinha para evitar throttling térmico. |

---

## Parte 15 — Refrigeração e térmica (hardware)

> Um PC que **esquenta demais reduz o clock sozinho** (throttling). Resfriar bem costuma dar mais
> FPS estável do que dezenas de tweaks de software.

| | Item | O que faz |
|---|---|---|
| 🟢 | **Limpar poeira** | Restaura o fluxo de ar. **(hardware)** |
| 🟢 | **Curvas de ventoinha** | Mais refrigeração sob carga. |
| 🟢 | **Bom fluxo de ar no gabinete** | Entrada/saída equilibradas. **(hardware)** |
| 🟡 | **Trocar pasta térmica** | CPU/GPU antigas ganham vários graus. **(hardware)** |
| 🟢 | **Monitorar temperaturas** | Evitar throttling (ver [Parte 18](#parte-18--monitorar-e-medir-o-ganho)). |
| 🔴 | **Repaste/cooler melhor** | Upgrade de refrigeração. **(hardware)** |

---

## Parte 16 — Overclock e undervolt (avançado)

> 🔴 Sempre teste estabilidade e temperatura. Reversível, mas exige paciência e medição.

| | Item | O que faz |
|---|---|---|
| 🔴 | **Overclock de GPU** | MSI Afterburner: subir core/memória por mais FPS. |
| 🟡 | **Undervolt de GPU** | Menos calor/consumo mantendo (ou ganhando) desempenho. Ótimo custo-benefício. |
| 🔴 | **Overclock de CPU** | Mais clock; exige refrigeração e teste. |
| 🟡 | **Undervolt de CPU** | Menos calor/throttling, especialmente em notebook. |
| 🟡 | **Curva V/F** | Tuning fino de tensão/frequência. |
| 🟢 | **Teste de estabilidade** | OCCT, Prime95, FurMark, memtest após qualquer OC. |

---

## Parte 17 — Manutenção segura do Windows

| | Item | O que faz |
|---|---|---|
| 🟢 | **Windows atualizado** | Correções de desempenho/segurança. |
| 🟢 | **Drivers atualizados** | GPU, chipset, rede, áudio. |
| 🟢 | **SFC / DISM** | `sfc /scannow` e `DISM /RestoreHealth` reparam o Windows sem apagar dados. |
| 🟢 | **Anti-malware** | Malware é causa comum de lentidão. Escanear com o Defender. |
| 🟡 | **Remover bloatware** | Desinstalar apps OEM inúteis — **sempre listando e confirmando com o usuário** antes. |
| 🟢 | **Reiniciar regularmente** | Limpa estado/memória; uptime gigante degrada desempenho. |

---

## Parte 18 — Monitorar e medir (o ganho)

> Sem medir, otimização é achismo. Use estas ferramentas para comprovar o "antes e depois".

| | Ferramenta | Para que |
|---|---|---|
| 🟢 | **MSI Afterburner + RTSS** | Overlay de FPS, frametime, uso e temperatura de CPU/GPU. |
| 🟢 | **HWiNFO64** | Sensores detalhados (temperatura, clock, throttling, consumo). |
| 🟢 | **CapFrameX / PresentMon** | Captura de frametime, 1% e 0.1% low (suavidade real, não só FPS médio). |
| 🟢 | **LatencyMon** | Diagnostica latência de DPC (drivers causando stutter). |
| 🟢 | **3DMark / Unigine** | Benchmark padronizado para comparar. |
| 🟢 | **CrystalDiskMark / CrystalDiskInfo** | Velocidade e **saúde (SMART)** do disco. |
| 🟢 | **LibreHardwareMonitor** | Sensores (já integrável a este projeto via `metricas.ps1`). |

> 📊 Olhe **1% low / frametime**, não só FPS médio — é o que define se o jogo está "suave".

---

## Parte 19 — Avançado / entusiasta

> 🔴 Alto risco/complexidade. Para quem sabe o que está fazendo e mede tudo.

| | Item | Observação |
|---|---|---|
| 🟡 | **Process Lasso** | Automatiza afinidade/prioridade por processo. |
| 🔴 | **Desativar mitigações (Spectre/Meltdown, ASLR)** | Pode dar FPS em CPUs antigas, **mas reduz a segurança do sistema.** Só com plena consciência. |
| 🔴 | **Interrupt affinity manual** | Mover IRQs de GPU/NIC para núcleos dedicados. |
| 🔴 | **Windows "debloated" / ISO custom** | Remover componentes do Windows. Pode quebrar atualizações/recursos. |
| 🔴 | **Kernel/driver tweaks de baixo nível** | Risco de instabilidade; sempre reversível e medido. |

---

## Parte 20 — O que EVITAR (mitos e perigos)

> ❌ Coisas que **não** ajudam, prejudicam, ou são perigosas. Esta lista é tão importante quanto as otimizações.

| | Evitar | Por quê |
|---|---|---|
| ❌ | **"Registry cleaners"** | Ganho de desempenho ≈ zero; risco de quebrar o sistema. |
| ❌ | **"Otimizadores" milagrosos / pagos** | Maioria é placebo ou bloatware; alguns instalam malware. |
| ❌ | **"RAM cleaners" / liberadores de memória** | O Windows gerencia RAM melhor que eles; podem até piorar. |
| ❌ | **Desligar o page file totalmente** | Causa crashes em jogos que esperam tê-lo. Mantenha gerenciado. |
| ❌ | **Scripts de debloat extremos** | Removem coisas essenciais, quebram Update/Loja/anti-cheat. |
| ❌ | **Deletar "arquivos do sistema" para liberar espaço** | Nunca apague pastas do Windows/Arquivos de Programas manualmente. |
| ❌ | **Desativar o antivírus para "ganhar FPS"** | Ganho irrelevante, risco enorme. |
| ❌ | **Aplicar dezenas de tweaks de uma vez** | Se algo quebrar, você não sabe o quê. Mude e meça aos poucos. |
| ❌ | **Overclock sem teste de estabilidade** | Crashes, corrupção, instabilidade. |
| ❌ | **Mexer em segurança/anti-cheat sem necessidade** | Pode te banir de jogos online (Secure Boot/TPM são exigidos por anti-cheats). |

---

## 🔐 O que NUNCA é tocado

Para qualquer automação deste projeto, estas áreas são **intocáveis**:

- **Documentos pessoais:** `Documentos`, `Imagens`, `Vídeos`, `Música`, `Downloads`, `Área de
  Trabalho` e qualquer pasta de usuário.
- **Saves e perfis de jogos.**
- **Arquivos de programas e dados de aplicativos** (exceto cache temporário descartável, com consentimento).
- **Nuvem/sincronização** (OneDrive, Google Drive, Dropbox) — no máximo *pausar*, nunca apagar.
- **Navegador:** histórico, senhas, favoritos, sessões — **nunca** tocados.
- **Chaves de registro fora do escopo de otimização** documentado.

A automação só **lê** o sistema (scan) e só **escreve** nos pontos de otimização catalogados,
sempre com backup e undo.

## 🧹 Limpeza segura: o que pode e o que não pode

**✅ Pode ser limpo (descartável, com consentimento):**
- `%TEMP%`, `%TMP%`, `C:\Windows\Temp` — arquivos temporários.
- Cache de shaders da GPU (é recriado).
- Cache de miniaturas e ícones do Windows.
- `Windows\Prefetch` (recriado; ganho discutível — só em SSD).
- Logs antigos do Windows Update (`cleanmgr` / `Limpeza de Disco`).
- Lixeira — **somente** se o usuário confirmar explicitamente.

**❌ Nunca é apagado automaticamente:**
- Pasta **Downloads** (contém arquivos que o usuário baixou de propósito).
- Qualquer pasta pessoal.
- Saves, configs e perfis de jogos/apps.
- Pontos de restauração (são a sua rede de segurança).

> Toda limpeza deve: (1) mostrar **o que** e **quanto** será apagado, (2) **pedir confirmação**, e
> (3) restringir-se às pastas de temporários/cache acima.

---

## ✅ Checklist rápido por nível de segurança

**🟢 Comece por aqui (seguro, alto retorno):**
1. Ponto de restauração (Parte 0)
2. **XMP/EXPO no BIOS** (Parte 5) ← maior ganho isolado
3. Driver de GPU atualizado + painel NVIDIA/AMD (Parte 3)
4. Taxa de atualização correta + G-SYNC/FreeSync (Parte 4)
5. Plano Alto Desempenho + Game Mode (Partes 1–2)
6. Resizable BAR (Parte 14)
7. Mouse acceleration off + polling 1000 Hz (Parte 12)
8. Limpar startup + overlays + navegador ao jogar (Parte 8)
9. Cabo Ethernet + DNS rápido (Parte 9)
10. Limpar poeira + curvas de ventoinha (Parte 15)

**🟡 Depois, se quiser mais (com medição):**
- Tweaks de registro (Parte 10), serviços (Parte 7), rede avançada (Parte 9), undervolt (Parte 16).

**🔴 Só com consciência do risco:**
- Timers/HPET/kernel (Parte 11), mitigações de segurança, OC, debloat (Parte 19).

> Regra final: **mude uma coisa, meça, e só então mude a próxima.** É assim que se sabe o que
> realmente funcionou — e como reverter se não funcionou.
