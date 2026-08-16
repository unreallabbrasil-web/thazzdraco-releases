# Esquema de Regras (Rules Schema)

> **Status v4.0:** este contrato está **em produção**. As regras vivem em
> `app/internal/engine/assets/rules.json` (**61 regras**, embutidas no `.exe` via `go:embed`) e são
> interpretadas pelo motor Go (`internal/engine/model.go`, `detect.go`, `apply.go`). Tipos de
> detecção/ação suportados: `registry`, `registry-foreach`, `powercfg`, `powercfg-setting`,
> `service`, `cleanup`/`cleanup-size`, `profile-compara`, `dns-check`. O contrato abaixo continua
> vigente.

Contrato do **motor de recomendação**. Cada otimização do guia
([GUIA-OTIMIZACAO-GAMING.md](GUIA-OTIMIZACAO-GAMING.md)) vira uma **regra declarativa** — um dado,
não código. O scanner usa a regra para *detectar*, o motor para *recomendar* e o executor para
*aplicar/desfazer*.

- **Onde ficam:** `rules/rules.json` (JSON puro — consumível tanto pelo PowerShell quanto pelo
  dashboard JS, sem duplicar lógica).
- **Fonte:** cada regra referencia a `parte` do guia de onde saiu.

---

## Anatomia de uma regra

| Campo | Tipo | Descrição |
|---|---|---|
| `id` | string | Identificador único, ex.: `win.game-dvr-off`. |
| `parte` | int | Parte do guia de origem (rastreabilidade). |
| `categoria` | string | Energia, Windows, GPU, Rede, Memória, Input... |
| `titulo` | string | Nome curto exibido na UI. |
| `descricao` | string | O que faz e por que ajuda (linguagem do usuário). |
| `tier` | enum | `verde` 🟢 · `amarelo` 🟡 · `vermelho` 🔴 (selo de segurança do guia). |
| `modo` | enum | `acionavel` · `consultivo` · `limpeza` (ver abaixo). |
| `impacto` | enum | `alto` · `medio` · `baixo`. |
| `requer_reboot` | bool | Precisa reiniciar para valer. |
| `requer_consentimento` | bool | Exige confirmação explícita antes de executar. |
| `toca_dados_pessoais` | bool | **Sempre `false`.** Invariante de segurança (ver abaixo). |
| `hardware_gate` | objeto/null | Condição de hardware/contexto para a regra se aplicar. |
| `detect` | objeto | Como o scanner lê o estado atual e decide se está "aplicado" ou "pendente". |
| `action` | objeto/null | Como aplicar. `null` em regras consultivas. |
| `undo` | objeto/null | Como reverter. Quase sempre `{ "tipo": "restore" }` (ver Snapshot). |
| `orientacao` | string | Só em `consultivo`: instruções para o usuário (ex.: ativar XMP no BIOS). |
| `depende_de` | string[] | Opcional. Ids de regras que precisam vir **antes**. |
| `conflita_com` | string[] | Opcional. Ids de regras que fazem a mesma coisa (ou se anulam). |

## Os 3 modos

- **`acionavel`** — o programa **aplica e desfaz** automaticamente (registro, powercfg, serviço...).
- **`consultivo`** — o programa **não pode** mexer (hardware/BIOS), apenas **detecta e recomenda**.
  Ex.: XMP/EXPO, Resizable BAR, refrigeração. `action`/`undo` são `null`; usa `orientacao`.
- **`limpeza`** — apaga **somente** arquivos temporários/cache descartáveis, **sempre** com
  `requer_consentimento: true` e `toca_dados_pessoais: false`. Sem `undo` (são regeneráveis).

## Tier → comportamento na UI

| Tier | Comportamento |
|---|---|
| 🟢 `verde` | Pode vir **marcado por padrão**. Base do "Otimização Segura em 1 clique". |
| 🟡 `amarelo` | Vem **desmarcado**, com a contrapartida explicada. Opt-in consciente. |
| 🔴 `vermelho` | Fica em seção **"Avançado"**, escondido por padrão, **exige consentimento** + aviso. |

## Relações entre regras (`depende_de` / `conflita_com`)

Campos **opcionais** — ausência significa "sem restrição", então toda regra antiga continua válida.
Quem os interpreta é o plano da Sessão ([SESSAO](SESSAO.md) §5.4); fora dele, aplicar por
Otimizações continua funcionando como sempre.

- **`depende_de`** — a regra só faz sentido depois de outra. Isso é **ordem de execução**, não
  estética: `power.processor-max-performance` grava valores em `SCHEME_CURRENT`, e se o esquema ativo
  mudar depois, os valores ficaram no esquema velho. Por isso ela depende de
  `power.high-performance`. No plano, o dependente é ordenado depois e fica **bloqueado** enquanto a
  dependência não estiver marcada (ou já aplicada na máquina).
- **`conflita_com`** — fazer as duas é redundante ou contraditório. Ex.: `mem.prefetch-superfetch-off`
  (registro) e `svc.sysmain-off` (serviço) desligam o mesmo Superfetch por caminhos diferentes. O
  plano mantém **uma**: vence maior impacto → menor risco → a que **não** pede reinício; a outra fica
  bloqueada com o motivo. A relação é **simétrica** mesmo declarada de um lado só.

**Duas regras de ouro ao declarar uma relação:**

1. **Todo bloqueio precisa de saída.** O técnico tem que conseguir destravar o item (marcando a
   dependência, ou desmarcando o vencedor do conflito). Por isso o conflito com uma regra **já
   aplicada** na máquina *não* bloqueia nada — não haveria como desfazer o bloqueio.
2. **Nada de ciclo.** `a` depender de `b` que depende de `a` trava os dois sem recurso. Há um teste
   (`TestCatalogoSemCicloDeDependencia`) que quebra o build se isso for parar no catálogo — assim
   como há um que confere se as relações apontam para regras que existem.

## `hardware_gate`

Condição avaliada contra o `system-profile.json` (ver [SCAN-ENGINE.md](SCAN-ENGINE.md)). Se a
condição for falsa, a regra **não é recomendada** naquele PC.

```jsonc
// condição simples
"hardware_gate": { "campo": "contexto.na_bateria", "op": "==", "valor": false }

// combinação
"hardware_gate": { "todos": [
  { "campo": "discos[0].tipo", "op": "==", "valor": "SSD" },
  { "campo": "hardware.gpu.fabricante", "op": "==", "valor": "NVIDIA" }
] }
```

Campos típicos: `contexto.tipo` (desktop/notebook), `contexto.na_bateria`, `discos[].tipo`
(SSD/HDD), `hardware.gpu.fabricante`, `hardware.ram.total_gb`. Operadores: `==`, `!=`, `<`, `>`,
`contem`. Agrupadores: `todos` (E), `qualquer` (OU).

## Operações (`detect` / `action` / `undo`)

Tipos de operação que o executor/scanner sabem interpretar:

| `tipo` | Usado em | Campos |
|---|---|---|
| `registry` | detect, action | `hive` (HKCU/HKLM), `hkcu_real_user` (bool), `path`, `valores[]` `{name, value, value_type, esperado}` |
| `powercfg` | action | `comandos[]` (lista de listas de argumentos) |
| `powercfg` / `powercfg-setting` | detect | `verifica`/`esperado` (esquema ativo) ou `sub`/`setting`/`esperado` |
| `service` | detect, action | `servicos[]` (lista de nomes); apply = parar + `Disabled`; undo restaura StartType/estado |
| `netsh` | action | `comandos[]` |
| `bcdedit` | action | `comandos[]` |
| `cleanup` | action | `alvos[]` (lista **fixa** de pastas temp/cache permitidas) |
| `cleanup-size` | detect | `alvos[]` (só calcula tamanho, não altera) |
| `profile-compara` | detect | `atual`, `nominal`, `quando` (consultivo: ex. RAM abaixo do nominal) |
| `registry-foreach` | detect, action | `hive`, `base` (caminho cujas **subchaves** são iteradas), `valores[]` — aplica/snapshot/undo em cada subchave (ex.: Nagle por interface de rede) |
| `dns-check` | detect | (sem campos) — `recomendado` se o DNS atual não for um conhecido rápido (1.1.1.1/8.8.8.8/9.9.9.9/...), senão `ok` |
| `restore` | undo | (sem campos — usa o snapshot, ver abaixo) |

> **Invocação interativa:** além dos `.bat`, o `servidor.ps1` expõe `GET /api/run?acao=<ping|escanear|
> aplicar|aplicar-verdes|desfazer>&ids=<csv>`, que roda `motor.ps1`/`executor.ps1` (via `-IdsCsv`) e
> devolve o `estado.json` atualizado. É o que dá o "1 clique" ao `painel.html`.

> ⚠️ O tipo `cleanup` **só** aceita pastas da allowlist de temporários/cache. Qualquer caminho fora
> dessa lista é rejeitado pelo executor — é a tradução do invariante "nunca apaga dado pessoal".

## Mecanismo de Snapshot + Undo (a rede de segurança)

A reversibilidade é **estrutural**, não opcional:

1. Antes de qualquer `action`, o executor **captura o estado anterior** do alvo (valor de registro,
   esquema de energia, tipo de inicialização do serviço...) e o grava em `backup/<timestamp>/`.
2. Também cria um **ponto de restauração** do Windows no início de um lote.
3. `undo: { "tipo": "restore" }` simplesmente **reaplica o snapshot**. Se o valor não existia antes,
   o undo o remove.

Por isso quase toda regra usa `undo: { "tipo": "restore" }` — não é preciso escrever o inverso à
mão, o que elimina uma classe inteira de erros. Regras `limpeza` não têm undo (alvos são
regeneráveis) e por isso exigem consentimento.

## Invariantes (validados pelo executor)

O executor **recusa** rodar uma regra que viole qualquer um destes:

1. `toca_dados_pessoais` deve ser `false`. Nenhuma regra acessa pastas de usuário, saves ou navegador.
2. `modo: acionavel` e `limpeza` **exigem** `undo` válido **ou** alvos exclusivamente regeneráveis.
3. `cleanup.alvos` deve estar 100% dentro da allowlist de temporários/cache.
4. `requer_consentimento: true` bloqueia a execução até confirmação explícita do usuário.
5. Antes de um lote acionável: ponto de restauração criado com sucesso (ou o usuário abre mão dele explicitamente).

## Ciclo de vida

```
   scanner ─ detect ─►  estado: aplicado | pendente | nao-aplicavel(gate)
                              │
   motor ── recomenda ──► lista priorizada (tier + impacto + gate)
                              │
   usuário ── seleciona ─► (verde já vem marcado; vermelho exige consentimento)
                              │
   executor ─ snapshot ─► action ─► log (valor antigo → novo)
                              │
   a qualquer momento ── undo (restore do snapshot)
```

## Exemplo completo (regra acionável verde)

```jsonc
{
  "id": "win.game-dvr-off",
  "parte": 1,
  "categoria": "Windows",
  "titulo": "Desativar Game DVR",
  "descricao": "Desliga a gravação em segundo plano do Windows, removendo overhead em jogos.",
  "tier": "verde",
  "modo": "acionavel",
  "impacto": "alto",
  "requer_reboot": false,
  "requer_consentimento": false,
  "toca_dados_pessoais": false,
  "hardware_gate": null,
  "detect": {
    "tipo": "registry", "hive": "HKCU", "hkcu_real_user": true,
    "path": "System\\GameConfigStore",
    "valores": [ { "name": "GameDVR_Enabled", "esperado": 0 } ]
  },
  "action": {
    "tipo": "registry", "hive": "HKCU", "hkcu_real_user": true,
    "path": "System\\GameConfigStore",
    "valores": [ { "name": "GameDVR_Enabled", "value": 0, "value_type": "DWord" } ]
  },
  "undo": { "tipo": "restore" }
}
```

## Versionamento

- `schema_version` no topo do `rules.json`. Subir quando a estrutura de uma regra mudar.
  - **1.0** → catálogo inicial. **1.1** → campos opcionais `depende_de` / `conflita_com`
    (compatível: uma regra sem eles se comporta exatamente como antes).
- Regras só são adicionadas/alteradas com referência à `parte` do guia — o guia é a fonte da verdade.
- Score (antigo cálculo de 16 checks) passa a ser **derivado das regras** `acionavel` aplicadas vs.
  aplicáveis — fonte única, sem a duplicação PS/JS atual (ver [CONVENTIONS.md](CONVENTIONS.md#score-fonte-única-dívida-técnica)).
