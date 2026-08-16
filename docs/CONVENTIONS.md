# Convenções do projeto

> **Status v4.0 (Go nativo):** o backend é **Go** em `app/` (módulo `thazzdraco`). Convenções atuais:
> camada `internal/winutil` = API Win32 pura (sem WMI/PowerShell, salvo a exceção do Defender);
> `internal/engine` = motor declarativo; regras em `assets/rules.json`; UI em `web/` (vanilla, fontes
> nativas, offline). Build via toolchain Go portátil + `build.ps1`. **Para testes headless, compile
> sem o `resource.syso`** (o manifesto exige admin). Não rodar `./tz.exe` recém-compilado com
> manifesto via bash (dá EACCES — é o UAC, não bug). As convenções de contrato JSON abaixo (nomes de
> chaves consistentes UI↔backend) seguem valendo. A divergência PowerShell↔dashboard citada abaixo é
> **histórica** (v3.0 aposentada).

Padrões de código e contratos que mantêm o projeto consistente.

---

## PowerShell

- **Somente ASCII.** Sem acentos e sem caracteres especiais nos scripts `.ps1` (evita problemas de
  encoding ao serem servidos/lidos). Use `Write-OK`, `Write-WARN`, etc.
- **Tolerância a falha.** Todo coletor/tweak isola erros com `try/catch` e `-ErrorAction
  SilentlyContinue`/`Stop` conforme o caso. Uma falha pontual nunca derruba o script inteiro.
- **Idempotência.** Rodar duas vezes produz o mesmo resultado. Crie chaves com
  `if (!(Test-Path $p)) { New-Item -Path $p -Force }` antes de gravar.
- **Caminhos relativos ao script.** Use `Split-Path -Parent $MyInvocation.MyCommand.Path` para
  achar a pasta; nada de caminhos absolutos chumbados.
- **Saída JSON.** Sempre `ConvertTo-Json -Depth 6 | Out-File -Encoding UTF8 -Force`.

## Padrão de elevação (UAC) nos `.bat`

Todo `.bat` que chama PowerShell começa com o mesmo bloco de auto-elevação:

```bat
@echo off
net session >nul 2>&1
if %errorlevel% neq 0 (
    PowerShell -Command "Start-Process '%~f0' -Verb RunAs"
    exit /b
)
cd /d "%~dp0"
```

E chama o script com: `PowerShell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0script.ps1"`.

## Detecção do usuário real

Como os scripts rodam **como administrador**, `HKCU` aponta para o perfil do admin, não do usuário
logado. Para gravar/ler no perfil certo, detecta-se o SID do dono do `explorer.exe`:

```powershell
$exp = Get-WmiObject Win32_Process -Filter "Name='explorer.exe'" | Select-Object -First 1
$owner = $exp.GetOwner()   # -> User/Domain -> NTAccount -> Translate p/ SID
# grava em HKCU: e também em Registry::HKEY_USERS\<SID>\...
```

Mantenha esse padrão em qualquer novo tweak que escreva em `HKCU` (ver `Set-HKCUValue` em
`FPS_Optimizer_Script.ps1` e `gvU` em `coletar.ps1`).

## Contrato de dados (PS → Dashboard)

O `Dashboard.html` consome os JSON por **nome de chave**. Se o PowerShell gravar uma chave com nome
diferente do que o JS lê, **a UI não renderiza e não dá erro** — falha silenciosa. Antes de
renomear qualquer chave, atualize os dois lados.

### Chaves esperadas

`coletar.ps1` → consumido por `calcChecks`, `renderAntes`, `renderRelatorio`:
`fase, timestamp, pc, usuario, hw{os,cpu,ramGB,gpu,uptime}, energia{plano,otimizado,hiber},
servicos{total,rodando,desativos}, registro{...16 chaves...}, score, checks_ok`.

`metricas.ps1` → consumido por `renderMetricas`:
`timestamp, pc, cpu, ram, gpu, discos[], rede[], processos[], sistema, sensor_fonte`.

### ⚠️ Bugs de contrato conhecidos (corrigir na Fase 1)

| `metricas.ps1` grava | `Dashboard.html` lê | Efeito |
|---|---|---|
| `processos` | `m.top_processos` (linha ~1167) | Tabela "Top Processos" fica **vazia** |
| `sistema` (com `uptime_txt`) | `m.uptime.texto` (linha ~1139) | Card "Sistema" **não renderiza** |

**Correção recomendada:** alinhar o JS ao que o PS já grava (`m.processos`, `m.sistema.uptime_txt`)
— ou vice-versa, mas em um só lugar. Depois disso, formalizar o contrato neste documento.

> Lição: na plataforma, o contrato JSON deve ser **versionado** (`schema_version`) e validado, para
> que divergências falhem de forma visível, não silenciosa.

## Score: fonte única (dívida técnica)

Hoje o cálculo dos 16 checks existe **duplicado**: em `coletar.ps1` (PowerShell) e em
`Dashboard.html` (`calcChecks`). Qualquer mudança precisa ser feita nos dois. **Alvo:** o motor de
regras calcula o score uma vez; a UI só exibe.

## Versionamento

- Versão visível no rodapé do dashboard e no cabeçalho dos scripts (`v3.0`).
- Registrar toda mudança relevante em [CHANGELOG.md](CHANGELOG.md).
- Esquema sugerido: `MAJOR.MINOR` enquanto for projeto pessoal; subir MAJOR quando a arquitetura
  mudar (ex.: introdução do motor de recomendação = v4.0).

## Nomes de arquivos

- `.bat` numerados pela ordem do fluxo (`1_`, `2_`, ...).
- `.ps1` com nome da função (`coletar`, `metricas`, `servidor`, `FPS_Optimizer_Script`).
- JSON de saída com nome da fase (`antes.json`, `depois.json`, `metricas.json`).
- Documentação em `docs/` com nome em MAIÚSCULAS (`ARCHITECTURE.md`).
