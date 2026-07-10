# Roadmap

Status de evolução do tweaker para a **plataforma de otimização** (v4.0, Go nativo).

---

## Visão

> Conectar a ferramenta em qualquer computador Windows, rodar uma varredura completa e receber uma
> lista priorizada e segura de tudo o que pode ser otimizado — aplicando apenas o que o usuário
> escolher, sempre com possibilidade de desfazer.

## Princípios

1. **Ler antes de agir** — varredura read-only antes de qualquer mudança.
2. **Consciente do hardware** — gates por SSD/HDD, notebook/desktop, RAM, bateria.
3. **Reversível por padrão** — ponto de restauração + snapshot por valor + undo + backup/restore.
4. **Transparente** — cada regra mostra o que faz, o impacto, o risco e os detalhes técnicos.
5. **Sem destruir nada** — risco alto fica desmarcado e pede consentimento.
6. **Portátil e sem dependências** — um `.exe`, roda de pendrive.
7. **Nada de "snake oil"** — só dado real/verificável (cliente acompanha ao vivo).

---

## Fases

### Fase 0 — Base v3.0 (✅ histórica, aposentada)
Assistente PowerShell de 4 passos, tweaks fixos, WPF. Em `legacy/`.

### Fase 1 — Motor de varredura (✅ concluída)
Perfil de hardware nativo (SMBIOS, energia, display, disco SSD/HDD, DNS), detecção de contexto
(notebook/desktop, SSD do sistema), varredura de **todas** as regras → `ScanResult`.

### Fase 2 — Motor de recomendação (✅ concluída)
Regras **declarativas** (`rules.json`, 43 regras), gating por hardware/uso, score calculado por uma
fonte única (Go). Detalhes técnicos por regra.

### Fase 3 — Aplicação segura + restauração (✅ concluída)
Ponto de restauração + **snapshot por valor** antes de escrever, aplicar só a seleção, **undo** por
regra/lote/tudo, **log** de operações (antes→depois), **Backup & Restaurar** completo, DryRun (CLI).

### Fase 4 — Plataforma / UX (✅ concluída)
Reescrita nativa em **Go** (1 `.exe` portátil). UI "DRACO // PORTAL" cinematográfica. **Presets**
no backend (`presets.json`). **Relatório** exportável (PDF/imprimir). Portabilidade total.
Features extras entregues: **Inicialização**, **Desempenho/Saúde ao vivo**, **GPU (NVML)**,
**breakdown de memória**, **Otimização por jogo**, **cerimônia de scan**.

### Fase 4.1 — Polimento & robustez (✅ concluída)
Navegação reorganizada em **6 nós** com sub-abas (Manutenção/Medição). **Tela de Perfis ligada.**
Ferramentas do "Manual Mestre" (faxina de rede, TRIM, plano máximo, Defender, hibernação, CHKDSK).
Passada de **copy sênior** (voz da marca, você-imperativo, sem hype). Auditoria de **robustez**:
restauração de backup honesta (conta só o que voltou), trava de servidor na limpeza irreversível
(Lixeira/Windows.old exigem `confirmar`), escrita atômica de histórico/backups, e **watchdog robusto**
(`inFlight` + 90s + qualquer request = vida) que acabou com o "Failed to fetch" em operações longas.

### Objetivo B — Limpar/reparar nível profissional ("sem formatar")
- [x] **#6 Reparo do Windows** — SFC + DISM (RestoreHealth/Cleanup) + reset do Windows Update, com saída
  ao vivo. Página "Manutenção". Não apaga dados.
- [x] **#7 Debloat** profissional — apps UWP inúteis por catálogo curado (nome exato, gamer-aware,
  reversível pela Store). OEM Win32 fica para depois (uninstaller varia, mais risco).
- [x] **#8 Limpeza profunda** — shader cache, dumps, cache do Update, Delivery Opt, miniaturas, Lixeira,
  Windows.old (icacls+rd). WinSxS via DISM Cleanup do #6.
- [x] **#9 Saúde térmica & undervolt** — teste de estresse térmico + detecção de throttling (NVML) +
  guia de undervolt. Temp real só NVIDIA; CPU/AMD/Intel = N/A honesto.
- [x] **#10 Relatório com prova** — FPS antes×depois (+ganho %), desempenho medido (índice/GPU/temp),
  logo+nome do cliente. **Objetivo B 100%.**

### Fase 5 — Avançado (em aberto)
- [x] **Medidor de FPS real in-game (Objetivo A #1)** — PresentMon/ETW embutido; FPS médio + 1% low +
  0.1% low + frametime; captura por jogo; Antes × Depois para provar o ganho (70 → 120).
- [x] **Diagnóstico de gargalos (Objetivo A #2)** — XMP-off, jogo no HDD, refresh abaixo do máx, HAGS,
  plano de energia, espaço, RAM, startup; cada um com correção (1 clique / página / manual).
- [x] **Estresse de GPU no benchmark** — WebGL, uso/temperatura ao vivo (NVML), Antes × Depois.
- [x] **Uso de GPU universal (NVIDIA/AMD/Intel)** via contadores PDH — validado contra NVML.
- [x] **Mais launchers na detecção de jogos** — Battle.net/EA/Ubisoft/Rockstar/Riot/GOG (via registro);
  validar em PC com esses jogos.

- [x] **Driver de GPU (Objetivo A #3)** — versão/data reais (NVML p/ NVIDIA), guia de instalação limpa
  (DDU) com aviso de TeamViewer, limpeza segura de resíduos do instalador. Sem nuke automático (risco).
- [x] **Plano de latência extrema + Perfis na UI (Objetivo A #4)** — preset Competitivo (21 ajustes)
  aplicável em 1 clique; +2 regras (Win32PrioritySeparation, impedir Windows de trocar o driver).
- [x] **Otimização por jogo turbinada (Objetivo A #5)** — perfis curados por título (~12 jogos) com
  dicas in-game testadas + tweaks seguros; sem editar config de jogo com anti-cheat. **Objetivo A 100%.**
- [x] **Tela de Perfis na UI** (Equilibrado/Competitivo/Streaming) — cards com aplicação em 1 clique.

**✅ Objetivo A (ganho de FPS comprovável) e Objetivo B (limpeza/reparo pro) — 100% concluídos.**
- [x] **Benchmark integrado para medir ganho real** — CPU (single/multi), memória (GB/s) e disco
  (MB/s sincronizado); Índice composto; comparação Antes × Depois por métrica.
- [x] **Uso de GPU AMD/Intel** — resolvido via contadores PDH (universal). Temperatura AMD/Intel fica
  N/A honesto (sem API limpa sem SDK do fabricante).
- [x] **Branding por cliente no relatório** — logo + nome do cliente no PDF (overlay imprimível).
- [ ] Temperatura de CPU (exigiria embarcar driver de sensor tipo LibreHardwareMonitor).
- [ ] Internacionalização (PT/EN/ES).

---

## Backlog de ideias (não priorizado)

- Desinstalar bloatware OEM (com consentimento).
- Auditoria de drivers (sinalizar drivers antigos — só informativo, sem alarme).
- Análise de boot lento (tempo por serviço/app).
- Encerrar processos pesados direto da tela de Desempenho.
- Manutenção rápida (1 botão: limpeza + verdes + checagens).
- Teste de rede/latência (ping de servidores de jogo).
- Ferramentas de reparo do Windows (SFC/DISM/cache do Update).
- Histórico de clientes atendidos (mini-CRM).

---

## Decisões já tomadas

| Tema | Decisão |
|---|---|
| Stack do backend | **Go nativo** (Win32 puro), substituiu o PowerShell |
| Formato das regras | **Dados declarativos** (`rules.json` embutido via `go:embed`) |
| Empacotamento | **1 `.exe`** com ícone+manifesto via `resgen.py` (sem ferramenta externa) |
| Invólucro/UI | UI web embutida + janela **Edge `--app`** (portátil, sem bundlar runtime) |
| Tratamento de risco alto | Mostrar desmarcado, com **selo de segurança** e consentimento |
| Temperatura | Só sensor **real** (GPU via NVML/ADL); CPU = N/A honesto |
