# ThazzDraco Optimizer

**Otimizador de PC para jogos — um único `.exe` nativo, portátil e sem dependências.**
Lê tudo que dá pra otimizar no Windows, recomenda só o que cabe naquele hardware, aplica com
backup/undo e mostra um diagnóstico ao vivo. Pensado para rodar em **qualquer PC** (pendrive ou
TeamViewer) e otimizar máquinas de clientes com segurança.

> Marca **ThazZDraco FPS Otimização** · [@thazdracofpsotimizacao](https://www.instagram.com/thazdracofpsotimizacao/)

---

## ✨ O que ele faz

- 🐉 **Núcleo / Diagnóstico (Portal)** — varredura cinematográfica com **Boost Score** (0–100).
- ⚡ **Otimizações** — 61 regras declarativas (registro, serviços, powercfg) com **toggle on/off**
  por módulo, organizadas por categoria, com **selo de segurança** (seguro/médio/avançado) e
  **detalhes técnicos** (a chave/valor exata que cada uma altera).
- 🎮 **Otimização por jogo** — detecta jogos do Steam/Epic e aplica por jogo: **FSO off**,
  **GPU máx**, **CPU alta** (prioridade) e **excluir do antivírus** (Defender). Tudo reversível.
- 🚀 **Inicialização** — liga/desliga programas que sobem com o Windows (igual ao Gerenciador de
  Tarefas, via `StartupApproved`, reversível).
- 🧹 **Limpeza** — apaga **só** temporários/cache (allowlist), nunca documentos pessoais.
- 📊 **Desempenho & Saúde** — ao vivo: CPU, RAM, **GPU (temperatura/uso/VRAM real via NVML)**,
  top processos, **detalhamento de memória** (pool não-paginado etc.), espaço + **S.M.A.R.T.**
  dos discos, bateria, uptime.
- 🎯 **FPS real in-game** — mede **FPS médio, 1% low e 0.1% low** de qualquer jogo via **PresentMon
  (ETW da Microsoft, sem hook/injeção)** e compara **antes × depois** para provar o ganho (ex.: 70 →
  120 FPS), com gráfico de frametime.
- 🩺 **Diagnóstico de gargalos** — cruza o hardware e aponta **o que segura seu FPS** (XMP/EXPO
  desligado, jogo num HD, monitor abaixo da taxa máxima, HAGS, plano de energia…) com a correção de
  cada um (1 clique ou passo na BIOS/Windows).
- 🏁 **Benchmark / estresse de CPU + GPU** — martela **CPU e GPU** com números **reais** (uso e
  temperatura da GPU ao vivo) e compara **antes × depois** para mostrar o ganho.
- 🛟 **Backup & Restaurar** — fotografa o estado de tudo antes da sessão e restaura num clique.
- 🧾 **Relatório do cliente** — resumo imprimível/PDF do que foi feito.
- 📝 **Histórico** — lotes reversíveis (desfazer lote / desfazer tudo) + log de operações.

### Segurança (inegociável)
Ponto de restauração + **snapshot por valor** antes de qualquer escrita · **undo** exato (regrava o
valor anterior) · limpeza só em pastas temporárias · **nunca toca em documentos pessoais** · regras
de risco pedem **consentimento** · princípio "**só mostrar dado real**" (nada de temperatura de CPU
chutada nem alarmes inventados — o cliente acompanha ao vivo).

---

## 🚀 Como usar

1. Leve **`app/dist/ThazzDraco.exe`** para o PC alvo — **pendrive** ou **TeamViewer** (File Transfer);
   é um arquivo só, tanto faz.
2. Clique direito → **Executar como administrador** → aceite o **UAC** (precisa de admin para mexer em
   registro/serviços/powercfg).
3. Abre uma janela (Microsoft Edge em modo app; se faltar, cai no navegador padrão).
4. Clique **Escanear PC** → aplique o que quiser (toggles, ou por categoria) → **reversível**.

Não instala nada. É **um arquivo só**, sem dependências. (Há também um atalho na área de trabalho:
`ThazzDraco Optimizer`.)

> **Via TeamViewer**, lembre: (1) o **UAC aparece na tela remota** — o TeamViewer precisa estar com a
> exibição de UAC ligada para você aceitar; (2) a **instalação limpa de driver (DDU)** derruba a tela e
> pode **cortar o acesso remoto** — faça só com **monitor físico**; (3) a **Faxina de rede** foi feita
> **sem** release/renew justamente para **não derrubar** a conexão (o reset vale após o reboot).

---

## 🛠️ Como compilar

Requer o toolchain **Go portátil** em `E:\CLAUDE AI\_toolchain` (não polui o sistema).

```powershell
cd "E:\CLAUDE AI\OPTM\app"
.\build.ps1        # gera dist\ThazzDraco.exe (ícone do dragão + manifesto admin + sem console)
```

O `build.ps1` roda `resgen.py` (gera `resource.syso` com ícone + manifesto, sem ferramenta externa)
e `go build -ldflags="-H windowsgui -s -w"`. Saída: **`dist/ThazzDraco.exe`** (~8,8 MB).

Modos de linha de comando (debug): `tz.exe -profile` (perfil de hardware), `tz.exe -scan`
(varredura em JSON), `tz.exe -headless -port N` (sobe o servidor sem janela/admin, para testes).
Importante: o build com manifesto (`resource.syso` presente) exige admin para rodar; para testes
headless, compile **sem** o `resource.syso`.

---

## 🧱 Arquitetura (resumo)

Tudo em **`app/`** (módulo Go `thazzdraco`). Um binário que **embute** a UI web e as regras
(`go:embed`), sobe um servidor HTTP local em porta efêmera e abre a UI numa janela do Edge `--app`.

```
app/
  main.go                 # elevação UAC, servidor, janela, watchdog
  internal/winutil/       # API Win32 pura (registro, serviços, powercfg, restore,
                          #   perfil/SMBIOS, métricas, saúde/SMART, GPU/NVML, inicialização, jogos)
  internal/engine/        # motor declarativo: rules.json embutido, detect/apply/undo, backup
  internal/server/        # servidor HTTP + API JSON
  web/                    # UI "DRACO // PORTAL" (HTML/CSS/JS, fontes nativas, offline)
  dist/ThazzDraco.exe     # >>> o entregável <<<
```

Detalhes em [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Histórico completo em
[docs/CHANGELOG.md](docs/CHANGELOG.md).

---

## 📚 Documentação

| Doc | Conteúdo |
|---|---|
| [ARCHITECTURE](docs/ARCHITECTURE.md) | Arquitetura Go v4.0 (winutil / engine / server / web) |
| [ROADMAP](docs/ROADMAP.md) | Status das fases (o que está pronto / pendente) |
| [OPTIMIZATIONS](docs/OPTIMIZATIONS.md) | Catálogo das 61 regras + tweaks por jogo |
| [RULES-SCHEMA](docs/RULES-SCHEMA.md) | Contrato das regras declarativas |
| [SCAN-ENGINE](docs/SCAN-ENGINE.md) | Como a varredura/detecção funciona |
| [GUIA-OTIMIZACAO-GAMING](docs/GUIA-OTIMIZACAO-GAMING.md) | Guia mestre (catálogo completo + selos de segurança) |
| [CONVENTIONS](docs/CONVENTIONS.md) | Convenções do projeto |
| [DISTRIBUICAO-CONFIANCA](docs/DISTRIBUICAO-CONFIANCA.md) | Por que o Windows alerta (SmartScreen/Defender) e como resolver |
| [CHANGELOG](docs/CHANGELOG.md) | Histórico de versões |

---

## 📌 Status (v4.4.3 — jul/2026)

**Funcional e validado** com dados reais (perfil, varredura, API, UI, GPU NVML, detecção de jogos,
benchmark, **medidor de FPS**). **Objetivo A** (ganho de FPS comprovável) e **Objetivo B** (limpar/
reparar nível pro) **100% concluídos**. Escrita real do motor **validada** (snapshot→escrita→undo em
registro; bug do powercfg corrigido — detecção lê o índice AC certo). **Uso de GPU é universal**
(NVIDIA/AMD/Intel via contadores PDH); **temperatura** é por sensor do fabricante (NVIDIA/NVML real;
AMD/Intel = N/A honesto). Detecção de jogos cobre Steam/Epic + Battle.net/EA/Ubisoft/Rockstar/Riot/GOG
(via registro). Navegação reorganizada em **6 nós** com sub-abas (Manutenção/Medição). **Tela de
Perfis ligada.** Passada de **copy sênior** + auditoria de **robustez** (restauração honesta, trava na
limpeza irreversível, escrita atômica) e **watchdog robusto** (acabou o "Failed to fetch" em operações
longas). Pendente: validação dos **novos launchers** num PC que tenha esses jogos. A versão legada
PowerShell/WPF (v3.0) foi **aposentada** (`legacy/`).

---

## 🙏 Créditos / Terceiros

- **[PresentMon](https://github.com/GameTechDev/PresentMon)** © Intel Corporation — licença **MIT**.
  Usado para medir o FPS real in-game via ETW. O binário é embutido e extraído em runtime junto com
  o texto da licença (`%LOCALAPPDATA%\ThazzDraco\PresentMon-LICENSE.txt`).
