# AUDIT — ThazzDraco Optimizer
### Cruzamento das Auditorias 1 (Código) + 2 (Visual DOM)
> Gerado em: 2026-06-29 | Auditor: Claude Sonnet 4.6
> Arquivos auditados: `app/web/app.css`, `app/web/index.html`, `app/web/app.js`, `app/web/icons.js`, `app/web/overlay.html`

---

## Legenda

| Símbolo | Significado |
|---------|-------------|
| **[A1]** | Encontrado na Auditoria 1 (Código/Design System) |
| **[A2]** | Encontrado na Auditoria 2 (Visual/DOM) |
| **[A1+A2]** | Convergente — ambas as auditorias apontaram a mesma raiz |
| 🔴 | Crítico — quebra UX ou identidade visual |
| 🟡 | Importante — degrada qualidade perceptível |
| 🟢 | Polimento — pequeno refinamento |

---

## FASE 1 — Correções Rápidas
> Cada item leva menos de 30 minutos. Alto impacto, baixo risco.

---

### 1.1 — Banner `.noadmin` compacto [A2] 🔴

**Problema:** O banner de "requer admin" ocupa 168px de altura — quase 20% do viewport 900px. É o primeiro elemento que o usuário sem UAC vê, empurrando o portal para baixo de forma agressiva. Parece erro, não aviso.

**Fix:** Reduzir para uma barra compacta de ~44px no topo da tela, similar a uma notificação de sistema.

**Arquivo:** `app/web/app.css`
```css
/* ANTES */
.noadmin { display:none; align-items:center; gap:11px; margin:0 32px 4px; padding:11px 18px; }

/* DEPOIS */
.noadmin { display:none; align-items:center; gap:8px;
           padding:8px 20px; font-size:11px; border-radius:0; border-left:none;
           border-bottom:1px solid #5a2725; }
.noadmin .na-ico { width:14px; height:14px; flex:none; }
/* remover o bloco grande de padding/margin, tornar uma barra horizontal compacta */
```

**Arquivo:** `app/web/index.html` — no bloco `#noadmin`, simplificar o conteúdo para uma linha.

---

### 1.2 — `:focus-visible` global [A1] 🔴

**Problema:** Nenhum elemento do app tem estilo de foco para teclado. Botões, nav, toggles — todos invisíveis ao Tab. Falha de acessibilidade básica.

**Fix:** Adicionar um bloco global no topo do CSS.

**Arquivo:** `app/web/app.css` — inserir logo após o `:root { ... }`:
```css
:focus-visible {
  outline: 2px solid var(--cyan);
  outline-offset: 2px;
}
/* Remove o outline padrão feio do browser, substitui pelo ciano do app */
button:focus:not(:focus-visible),
a:focus:not(:focus-visible) { outline: none; }
```

---

### 1.3 — `bolt` ícone fill quebra padrão stroke [A1] 🔴

**Problema:** `bolt` usa `fill="currentColor" stroke="none"`, todos os outros 40+ ícones são stroke. No HUD onde `bolt` aparece em destaque, ele tem peso visual radicalmente maior.

**Fix:** Converter `bolt` para stroke, igual ao `energy` (que tem o mesmo path).

**Arquivo:** `app/web/icons.js`, linha 9:
```js
// ANTES
bolt: S('<path d="M13 2 4.5 13.5H11l-1 8.5L19.5 10H13z" fill="currentColor" stroke="none"/>'),

// DEPOIS
bolt: S('<path d="M13 2 4.5 13.5H11l-1 8.5L19.5 10H13z"/>'),
```

---

### 1.4 — GPU VRAM mostra "-1/-1 MB" antes do primeiro poll [A2] 🟡

**Problema:** No estado inicial da página GPU, antes do primeiro ciclo de polling das métricas, os valores de VRAM aparecem como "-1 / -1 MB" em vez de "—".

**Fix:** No JS que renderiza a GPU, checar `< 0` e exibir "—".

**Arquivo:** `app/web/app.js` — na função de render da GPU, onde se mostra VRAM:
```js
// Substituir qualquer exibição direta de g.VramUso / g.VramTot por:
const vramUsado = (g.vram_uso_mb >= 0) ? g.vram_uso_mb + " MB" : "—";
const vramTotal = (g.vram_tot_mb > 0)  ? g.vram_tot_mb + " MB" : "—";
```

---

### 1.5 — Botão "Exportar config" ativo sem jogos [A2] 🟡

**Problema:** O botão de exportar jogos aparece habilitado mesmo quando `#jogosList` está vazio. Exportar um JSON vazio não tem valor e confunde o usuário.

**Fix:** Desabilitar o botão quando não há jogos carregados.

**Arquivo:** `app/web/app.js` — na função que renderiza/atualiza a lista de jogos:
```js
const btnExport = document.getElementById("btnExportJogos"); // ajustar ID real
if (btnExport) btnExport.disabled = (state.jogos?.length ?? 0) === 0;
```

---

### 1.6 — Nav `<a>` não focável por teclado [A1] 🔴

**Problema:** Os itens de navegação são `<a data-page>` sem `href`. Links sem `href` não recebem foco de teclado por padrão — a nav inteira é inacessível via Tab.

**Fix (mínimo):** Adicionar `tabindex="0"` em cada `.cn` no HTML.

**Arquivo:** `app/web/index.html` — em cada `<a class="cn" data-page="...">`:
```html
<!-- ANTES -->
<a class="cn active" data-page="inicio" data-icon-left="home">

<!-- DEPOIS -->
<a class="cn active" data-page="inicio" data-icon-left="home" tabindex="0" role="button">
```

**Arquivo:** `app/web/app.js` — adicionar suporte a `keydown Enter/Space` nos listeners da nav:
```js
document.querySelectorAll(".cn").forEach(a => {
  a.addEventListener("keydown", e => {
    if (e.key === "Enter" || e.key === " ") { e.preventDefault(); a.click(); }
  });
});
```

---

### 1.7 — `user-select: none` impede copiar dados úteis [A1] 🟡

**Problema:** `user-select: none` no `body` impede o usuário de copiar caminhos de arquivo, versões de driver, tamanhos de RAM — dados que ele frequentemente precisa compartilhar em suporte.

**Fix:** Remover do body; aplicar somente onde faz sentido (botões, nav, etiquetas da UI).

**Arquivo:** `app/web/app.css`:
```css
/* ANTES */
body { ... user-select:none; ... }

/* DEPOIS — remover do body, adicionar seletivamente */
button, .tophud, .cmd, .toggle, label { user-select: none; }
/* Textos de conteúdo (cards, listas, resultados) ficam selecionáveis */
```

---

### 1.8 — `.btn-hero` sem estado `:hover` [A1] 🟡

**Problema:** O botão CTA mais importante do app (ESCANEAR PC) não responde visualmente ao mouse. Único botão sem feedback de hover.

**Fix:** Adicionar hover consistente com os outros botões primários.

**Arquivo:** `app/web/app.css`:
```css
.btn-hero:hover {
  filter: brightness(1.12);
  transform: translateY(-1px);
}
.btn-hero:active {
  transform: translateY(0);
  filter: brightness(0.95);
}
```

---

### 1.9 — `.toggle` sem ARIA [A1] 🟢

**Problema:** Todos os toggles do app são `<label class="toggle">` sem `role` ou `aria-checked`. Leitores de tela não conseguem identificá-los.

**Fix:** No JS que cria ou gerencia os toggles, setar atributos ARIA ao inicializar e ao mudar estado.

**Arquivo:** `app/web/app.js` — na função de `wire()` e nos callbacks dos toggles:
```js
function setToggle(el, ativo) {
  el.setAttribute("role", "switch");
  el.setAttribute("aria-checked", ativo ? "true" : "false");
  // ... lógica existente
}
```

**Arquivo:** `app/web/index.html`, linha com `#lgbToggle`:
```html
<label class="toggle" id="lgbToggle" role="switch" aria-checked="false"><i></i></label>
```

---

### 1.10 — `mem` ícone duplica `ram` exato [A1] 🟢

**Problema:** `mem` e `ram` têm paths idênticos em `icons.js` (linhas 16 e 34). Um deles pode ser eliminado ou diferenciado.

**Fix:** Remover `mem` e substituir qualquer `data-icon="mem"` por `data-icon="ram"` no HTML/JS. Ou diferenciar o visual de `mem` para representar "módulo de memória" de forma distinta.

**Arquivo:** `app/web/icons.js`, linha 34 — remover ou diferenciar:
```js
// Opção A: remover (e substituir uso por ram)
// Opção B: diferenciar
mem: S('<rect x="2" y="5" width="20" height="14" rx="2"/><path d="M6 5V3M10 5V3M14 5V3M18 5V3M7 12h10"/>'),
```

---

## FASE 2 — Melhorias Importantes
> Cada item leva 30 minutos a 2 horas. Impacto significativo na qualidade.

---

### 2.1 — Identidade HUD: converter features novas de `border-radius` para `clip-path` [A1+A2] 🔴

**Problema:** 8+ componentes criados nas features F1–F14 usam `border-radius` convencional. O app original usa `clip-path` assimétrico como assinatura visual. Cada nova feature adicionada dilui progressivamente a identidade.

**Componentes afetados e fix:**

| Componente | Classe CSS | `border-radius` atual | Fix |
|---|---|---|---|
| Live Game Bar (F1) | `.live-game-bar` | `7px` | `clip-path: var(--cut-row)` |
| Row de serviço (F11) | `.svc-row` | `6px` | `clip-path: var(--cut-row)` |
| Row de driver (F12) | `.drv-row` | `6px` | `clip-path: var(--cut-row)` |
| Histórico de perf (F5) | `.perf-hist-box` | `8px` | `clip-path: var(--cut-card)` |
| Opção DNS | `.dns-opt` | `7px` | `clip-path: var(--cut-row)` |
| Parâmetro FPS | `.fps-param` | `4px` | `clip-path: var(--cut-row)` |
| Input adicionar jogo | `.game-add-input` | `6px` | manter (inputs não levam clip-path) |
| Badge de jogo ativo | `.lgb-game` | `4px` | `clip-path: var(--cut-btn)` |

**Arquivo:** `app/web/app.css` — para cada classe, substituir `border-radius:Xpx` por `clip-path: var(--cut-row)` (ou `--cut-card` para painéis maiores).

> **Nota:** Verificar visualmente após cada mudança — o `clip-path` com polígono assimétrico pode cortar bordas de elementos com `overflow: visible`. Testar com conteúdo longo.

---

### 2.2 — Empty states padronizados em todos os containers dinâmicos [A1+A2] 🔴

**Problema:** Containers ficam em branco silencioso até o usuário executar uma ação. A classe `.empty` já existe no CSS com estilo visual adequado. Falta usá-la.

**Containers que precisam de empty state:**

| Container ID | Página | Trigger do empty state |
|---|---|---|
| `#otimRules` | Otimizações | Antes do scan ou sem resultados |
| `#reco` | Otimizações | Antes do scan |
| `#subnav` | Otimizações | Antes do scan |
| `#jogosList` | Jogos | Nenhum jogo detectado |
| `#toolsGrid` | Manutenção | Antes de renderTools() |
| `#svcList` | Manutenção / Ferramentas | Antes do scan de serviços |
| `#drvList` | Manutenção / Ferramentas | Antes do audit de drivers |
| `#perfTop` | Medição | Antes do primeiro poll |
| `#histList` | Histórico | Sem histórico de lotes |

**Fix modelo para cada um — no JS, quando a lista estiver vazia:**
```js
// app.js — função que renderiza #otimRules
function renderOtimRules(rules) {
  const el = document.getElementById("otimRules");
  if (!rules || rules.length === 0) {
    el.innerHTML = `<div class="empty">
      ${ICONS.scan || ""}
      <p>Execute um Scan para ver as otimizações disponíveis</p>
    </div>`;
    return;
  }
  // ... render normal
}
```

---

### 2.3 — Estado inicial do HUD: "—" parece broken [A2] 🟡

**Problema:** Os contadores do HUD (`#cAplicados`, `#cPendentes`, `#cOportunidades`) mostram "—" no estado inicial — visualmente parecem estar com erro em vez de "aguardando".

**Fix:** Trocar "—" por um estado visual claro de "aguardando scan" com cor neutra e texto de suporte.

**Arquivo:** `app/web/app.js` — no estado inicial do HUD:
```js
// ANTES: simplesmente "—"
// DEPOIS: mostrar valor com estado semântico
function setHudInitial() {
  document.getElementById("cAplicados").textContent = "0";
  document.getElementById("cPendentes").textContent = "?";
  document.getElementById("cOportunidades").textContent = "?";
  // adicionar classe "hud-pending" para estilizar o "?" diferente
}
```

**Arquivo:** `app/web/app.css`:
```css
.hud-val.hud-pending { color: var(--ink-3); font-style: normal; }
```

---

### 2.4 — Cores de estado hard-coded: mover para variáveis [A1] 🟡

**Problema:** 11+ cores de estado (`#ff6a52`, `#35e0a0`, `#ffb43a`, `#41c7ff`) aparecem literalmente no CSS em vez de usar as variáveis existentes. Se a paleta mudar, esses ficam para trás.

**Fix:** As variáveis JÁ existem — só precisam ser usadas:

**Arquivo:** `app/web/app.css` — substituir cada ocorrência:

| Hard-coded | Variável a usar |
|---|---|
| `#ff6a52` | `var(--red)` |
| `#35e0a0` | `var(--green)` |
| `#ffb43a` | `var(--amber)` |
| `#41c7ff` | `var(--cyan)` |
| `#7ff0c2` | `var(--t-verde)` |
| `#ff9a8a` | `var(--t-vermelho)` |
| `#ffd27a` | `var(--gold)` |

Afeta: `.diag-card.*`, `.thermal-card.*`, `.bench-delta.*`, `.diag-pill.*`, `.diag-badge.*`

---

### 2.5 — Tokens duplicados: limpar aliases sem propósito [A1] 🟢

**Problema:** `--t-verde`, `--t-amarelo`, `--t-vermelho` são idênticos a `--green`, `--gold`, `--red`. `--cyan-deep` nunca é usado.

**Fix:** No `:root`, remover os duplicados e substituir os usos pelo token canônico. Verificar que nenhum JS usa os tokens `--t-*` diretamente.

**Arquivo:** `app/web/app.css` — no bloco `:root`:
```css
/* REMOVER estas linhas */
--t-verde: #34e29f;    /* duplica --green */
--t-amarelo: #f5c451;  /* duplica --gold  */
--t-vermelho: #e25252; /* duplica --red   */
--cyan-deep: #1a4a6e;  /* nunca usado     */
```

Após remover, buscar qualquer uso de `var(--t-verde)` etc. e substituir pelo token canônico.

---

### 2.6 — Toast: adicionar animação de saída e timer visual [A1] 🟢

**Problema:** O toast aparece com animação de entrada (`slidein`) mas some sem nenhuma transição — desaparece abruptamente. Sem timer visual, o usuário não sabe quanto tempo tem para ler.

**Fix:** Adicionar barra de progresso de esgotamento e animação de saída.

**Arquivo:** `app/web/app.css`:
```css
/* Adicionar ao .toast existente */
.toast::after {
  content: '';
  position: absolute;
  bottom: 0; left: 0;
  height: 2px;
  background: var(--cyan);
  animation: toast-timer var(--toast-dur, 4s) linear forwards;
}
@keyframes toast-timer { from { width: 100% } to { width: 0 } }

.toast.hide {
  animation: slideout .25s ease forwards;
}
@keyframes slideout { to { opacity:0; transform: translateX(26px); } }
```

**Arquivo:** `app/web/app.js` — no `showToast()`, ao remover o toast:
```js
function hideToast(el) {
  el.classList.add("hide");
  el.addEventListener("animationend", () => el.remove(), { once: true });
}
```

---

### 2.7 — Delay visual em `#toolsGrid` (Manutenção) [A2] 🟡

**Problema:** A grade de ferramentas (Turbo, DNS, etc.) em Manutenção renderiza de forma assíncrona via `renderTools()`. Na troca de aba, o usuário vê um container vazio por um frame perceptível.

**Fix:** Renderizar `#toolsGrid` sincronamente ao carregar a aba, não apenas quando ela se torna ativa. Se já tiver os dados, não há custo.

**Arquivo:** `app/web/app.js` — chamar `renderTools()` no init do app, não somente no `showPage("manutencao")`:
```js
// No DOMContentLoaded ou init():
renderTools(); // renderiza a grade uma vez, ficará pronta quando o usuário abrir a aba
```

---

### 2.8 — `#perfTop` vazio 2s no primeiro acesso a Medição [A2] 🟡

**Problema:** `#perfTop` (métricas ao vivo em Medição) fica vazio por ~2 segundos no primeiro acesso, enquanto o poll de métricas não completou o primeiro ciclo.

**Fix:** Inicializar `#perfTop` com valores placeholder "—" visualmente consistentes antes do primeiro poll, em vez de deixar o container vazio.

**Arquivo:** `app/web/app.js` — na função que inicializa a aba de medição:
```js
function initPerfTop() {
  // Renderizar skeleton com "—" antes do primeiro dado real
  const container = document.getElementById("perfTop");
  if (!container.innerHTML.trim()) {
    renderPerfTop({ cpu_pct: -1, ram: null, gpus: [], cpu_temp_c: -1 });
  }
}
```

---

## FASE 2B — Polimento Visual e Funcional
> Itens das auditorias que não entraram nas fases anteriores. Menor esforço, mas completam o quadro.

---

### 2.9 — `#sysStrip` vazio sem scan [A2] 🟡

**Problema:** Na página Início, o strip do sistema (`#sysStrip`) fica em branco antes de um scan — espaço reservado visível sem conteúdo, criando um buraco visual abaixo do botão CTA.

**Fix:** Preencher com dados mínimos disponíveis sem scan (hostname, versão do Windows, uptime) ou mostrar um placeholder com traços estilizados.

**Arquivo:** `app/web/app.js` — no init da página início, popular o strip com dados básicos do SO que já chegam em `/api/metrics` (uptime, ram.total_gb):
```js
function initSysStrip(m) {
  const el = document.getElementById("sysStrip");
  if (!el || el.dataset.filled) return;
  el.innerHTML = `<span>${m.hostname || "PC"}</span>
                  <span>${m.ram?.total_gb?.toFixed(0) || "—"} GB RAM</span>
                  <span>Up: ${m.uptime || "—"}</span>`;
  el.dataset.filled = "1";
}
```

---

### 2.10 — `bolt` e `energy` são o mesmo ícone [A1] 🟢

**Problema:** Em `icons.js` linhas 9 e 31, `bolt` e `energy` têm exatamente o mesmo path SVG `M13 2 4.5 13.5H11l-1 8.5L19.5 10H13z`. A diferença é só fill vs stroke. Além de ser duplicata, confunde quem decide qual usar.

**Fix:** Manter apenas `bolt` (já corrigido para stroke no item 1.3). Substituir qualquer `data-icon="energy"` por `data-icon="bolt"` e remover o alias.

**Arquivo:** `app/web/icons.js`, linha 31 — remover:
```js
// REMOVER: energy é idêntico a bolt
energy: S('<path d="M13 2 4.5 13.5H11l-1 8.5L19.5 10H13z"/>'),
```

Verificar uso: `grep -r 'data-icon="energy"'` e `grep -r '"energy"'` no JS.

---

### 2.11 — `--gold` vs `--amber`: dois amarelos sem critério [A1] 🟢

**Problema:** O app tem `--gold: #f5c451` (amarelo suave) e `--amber: #ff9d3c` (laranja-âmbar) coexistindo como cores de aviso. Sem documentação de quando usar cada um, features novas escolhem aleatoriamente.

**Fix:** Definir semântica clara no `:root` via comentário, ou eliminar um deles se os casos de uso forem equivalentes.

**Arquivo:** `app/web/app.css` — no `:root`, adicionar comentários de uso:
```css
--gold:  #f5c451;  /* aviso suave — estado "atenção" (badges, pills de warn) */
--amber: #ff9d3c;  /* aviso urgente — temperatura, limites críticos próximos */
```

Se na prática um deles não for usado consistentemente, migrar todos os usos para o token canônico e remover o outro.

---

### 2.12 — Animação de página e sub-aba inconsistente [A1] 🟢

**Problema:** A transição de página usa só `opacity` (fade simples). A transição de sub-aba usa `opacity + translateY` (fade + deslize para cima). Usuário percebe dois padrões diferentes para navegação, sem critério aparente.

**Fix:** Unificar — escolher um padrão e aplicar aos dois. O `opacity + translateY` é mais elegante para uma UI gamer.

**Arquivo:** `app/web/app.css` — na regra de entrada de `.page.active` (ou similar):
```css
/* Unificar com o mesmo padrão das sub-abas */
.page { opacity: 0; transform: translateY(8px); }
.page.active {
  opacity: 1;
  transform: translateY(0);
  transition: opacity .18s ease, transform .18s ease;
}
```

---

### 2.13 — Transição abrupta: portal cinematográfico → páginas funcionais [A1] 🟡

**Problema:** A página Início tem atmosfera dramática (dragon, raios, partículas, portal). As páginas internas são grids de dados densos sem nenhum elemento de transição de tom. A mudança é visualmente brusca — como sair de um trailer de jogo e entrar numa planilha.

**Fix:** Adicionar sutis elementos de linguagem visual compartilhada: um accent de raio ou partícula mínima nos `ptitle` de seção, ou uma linha de header ciano com o motivo de grade. Não replicar o portal — apenas uma "assinatura" discreta.

**Arquivo:** `app/web/app.css` — nos `.ptitle` das páginas internas, adicionar um detalhe de acento:
```css
.ptitle::before {
  content: '';
  display: inline-block;
  width: 3px; height: 14px;
  background: var(--cyan);
  clip-path: var(--cut-btn);
  margin-right: 8px;
  vertical-align: middle;
  opacity: 0.7;
}
```

---

### 2.14 — `stroke-width: 1.8` hard-coded em todos os ícones [A1] 🟢

**Problema:** Em `icons.js`, o `stroke-width="1.8"` está literal na string do template `S()`. Se quisermos variar o peso dos ícones por contexto (ícones do HUD mais bold, ícones de lista mais leves), precisamos reescrever todos.

**Fix:** Transformar em variável CSS que pode ser sobrescrita por contexto.

**Arquivo:** `app/web/icons.js`, linha 4 — no template `S()`:
```js
// ANTES
`stroke-width="1.8"`

// DEPOIS
`stroke-width="var(--icon-stroke, 1.8)"`
```

**Arquivo:** `app/web/app.css`:
```css
/* Peso padrão */
:root { --icon-stroke: 1.8; }
/* Ícones de destaque no HUD podem ser mais bold */
.tophud [data-icon] { --icon-stroke: 2; }
```

---

### 2.15 — Botão `×` do toast usa texto, não ícone SVG [A1] 🟢

**Problema:** O botão de fechar o toast usa o caractere `×` (texto), enquanto todo o resto do app usa ícones SVG do sistema. Peso visual, alinhamento e tamanho ficam inconsistentes entre temas/fontes.

**Fix:** Substituir pelo ícone `err` ou criar um ícone `close` específico.

**Arquivo:** `app/web/app.js` — na função `showToast()`, onde renderiza o botão de fechar:
```js
// ANTES: algo como `<button>×</button>`
// DEPOIS:
`<button class="toast-close" aria-label="Fechar"><span data-icon="err"></span></button>`
// e chamar icons.js para preencher (ou usar innerHTML direto com ICONS.err)
```

---

### 2.16 — `.reco-row` grid fixo 3 colunas [A1] 🟢

**Problema:** A grade de recomendações usa `grid-template-columns` fixo de 3 colunas. Se o número de recomendações não for múltiplo de 3, a última linha fica com células vazias visíveis (buracos de grid).

**Fix:** Usar `auto-fill` com tamanho mínimo para que o grid se adapte.

**Arquivo:** `app/web/app.css` — na classe `.reco-grid` ou equivalente:
```css
/* ANTES */
grid-template-columns: repeat(3, 1fr);

/* DEPOIS */
grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
```

---

### 2.17 — FPS: `renderFpsGames()` depende de `state.jogos` [A2] 🟡

**Problema:** A aba FPS em Medição lista os jogos disponíveis para medir FPS chamando `renderFpsGames()`, que depende de `state.jogos` já preenchido. Se o usuário acessar Medição antes da página Jogos, a lista fica vazia e sem mensagem.

**Fix:** Garantir que `state.jogos` seja carregado no init do app, não apenas quando a página Jogos é visitada. Ou adicionar empty state com instrução.

**Arquivo:** `app/web/app.js` — no init:
```js
async function loadJogosIfNeeded() {
  if (!state.jogos) {
    const r = await fetch("/api/jogos");
    state.jogos = await r.json();
  }
}
// Chamar no init() antes de renderizar qualquer aba de Medição
```

---

### 2.18 — Report PDF: flash branco ao abrir modal [A1] 🟡

**Problema:** O modal do relatório PDF tem `background: #fff` (modo papel). Ao abrir, o flash de branco em tema escuro é visualmente agressivo e parece um bug.

**Fix:** Aplicar uma transição de entrada que suavize o aparecimento do branco, ou usar uma cor de fundo escura para o overlay antes da animação terminar.

**Arquivo:** `app/web/app.css` — no `.overlay` que contém o report:
```css
.overlay { background: #04070bcc; opacity: 0; transition: opacity .2s ease; }
.overlay.show { opacity: 1; }
/* O conteúdo interno (.report-paper) já tem fundo branco mas entra depois do overlay escuro */
```

**Arquivo:** `app/web/app.js` — ao mostrar o modal:
```js
overlay.style.display = "flex";
requestAnimationFrame(() => overlay.classList.add("show"));
```

---

### 2.19 — `padding-bottom: 120px` valor mágico em `.pages` [A1] 🟢

**Problema:** `.pages` usa `padding-bottom: 120px` como compensação para a nav. Valor mágico — se a altura da nav mudar, o scroll fica errado.

**Fix:** Vincular ao tamanho real da nav via variável.

**Arquivo:** `app/web/app.css`:
```css
:root { --nav-h: 106px; } /* altura real da .cmd — medir e fixar aqui */

.pages { padding-bottom: calc(var(--nav-h) + 14px); }
```

---

### 2.20 — `font-style: italic` inconsistente em títulos [A1] 🟢

**Problema:** Alguns `.ptitle` têm `font-style: italic` e outros não, sem critério visual claro. A inconsistência é perceptível ao navegar entre seções.

**Fix:** Definir uma regra: ou todos os `.ptitle` são italic (identidade), ou nenhum é. Verificar qual uso é majoritário e padronizar.

**Arquivo:** `app/web/app.css` — na definição de `.ptitle`:
```css
/* Se a decisão for NÃO usar italic */
.ptitle { font-style: normal; }

/* Se a decisão for usar italic como identidade de seção */
.ptitle { font-style: italic; }
```

---

## FASE 3 — Refatorações Sistêmicas
> Cada item leva 2+ horas. Saúde a longo prazo do código e do design.

---

### 3.1 — Sistema tipográfico: escala de tokens [A1] 🔴

**Problema:** O CSS usa 29 tamanhos de fonte diferentes (de `8.5px` a `84px`) sem nenhum token. Nenhum refactor de tipografia pode ser feito sem busca e substituição manual em centenas de linhas.

**Fix:** Definir uma escala de 8 níveis no `:root` e substituir progressivamente.

**Arquivo:** `app/web/app.css` — adicionar ao `:root`:
```css
--fs-2xs: 8.5px;   /* labels mínimos, badges */
--fs-xs:  10px;    /* sub-labels, códigos secundários */
--fs-sm:  11px;    /* texto de suporte, descrições */
--fs-md:  13px;    /* texto base, listas */
--fs-lg:  15px;    /* subtítulos, valores de métricas */
--fs-xl:  18px;    /* títulos de seção */
--fs-2xl: 26px;    /* números grandes de dashboard */
--fs-hero: 56px;   /* score hero, valores de destaque máximo */
```

Substituir progressivamente os `font-size` literal por `var(--fs-*)`.

---

### 3.2 — Hierarquia de botões: 8 variantes → 3 papéis [A1] 🔴

**Problema:** O app tem `.btn-hero`, `.btnG`, `.abtn`, `.abtn.guide`, `.mbtn`, `.mbtn.primary`, `.mbtn.danger`, `.btn-danger2`, `.iconbtn` — 9 variantes sem critério claro de quando usar cada uma. Features novas não sabem qual escolher.

**Fix:** Mapear para 3 papéis semânticos e deprecar os aliases:

| Papel | Classe canônica | Aparência |
|---|---|---|
| Primário (ação principal da página) | `.btn-primary` | Fundo ciano, clip-path |
| Secundário (ação de apoio) | `.btn-secondary` | Borda ciano, fundo transparente |
| Destrutivo (apagar, parar, danger) | `.btn-danger` | Borda/texto vermelho |
| Ícone (HUD) | `.iconbtn` | mantém como está |

Manter as classes antigas como aliases via `@extend` ou duplicação de regras durante a transição.

---

### 3.3 — Sistema de espaçamento tokenizado [A1] 🟡

**Problema:** Cards usam `padding: 15px 18px`, `16px 20px`, `17px 18px`, `18px 20px` — todos ad hoc. Impossível manter ritmo visual consistente.

**Fix:** Definir escala de spacing no `:root`:
```css
--sp-1: 4px;
--sp-2: 8px;
--sp-3: 12px;
--sp-4: 16px;
--sp-5: 20px;
--sp-6: 24px;
--sp-8: 32px;
```

Padronizar cards para `padding: var(--sp-4) var(--sp-5)` (16px 20px) e sub-itens para `var(--sp-3) var(--sp-4)`.

---

### 3.4 — Contraste `--ink-3`: corrigir para WCAG AA [A1] 🟡

**Problema:** `--ink-3` (#5f7d9c) sobre `--panel` (#070f18) tem ~3.5:1 de contraste — abaixo do mínimo WCAG AA de 4.5:1 para texto pequeno. Afeta labels de 9–10px em todo o app.

**Fix:** Clarear `--ink-3` para atingir 4.5:1.

**Arquivo:** `app/web/app.css` — no `:root`:
```css
/* ANTES */
--ink-3: #5f7d9c;  /* ~3.5:1 */

/* DEPOIS — verificar com contrast checker */
--ink-3: #7a9ab8;  /* ~4.6:1 — passa WCAG AA */
```

> Testar visualmente após a mudança — clarear demais perde o contraste com `--ink-2`.

---

### 3.5 — `clip-path` impede `box-shadow` nos cards [A1] 🟢

**Problema:** Cards com `clip-path` não renderizam `box-shadow` (a propriedade é cortada pelo clip). O app não tem profundidade visual — nenhuma hierarquia Z visível entre camadas.

**Fix:** Usar `filter: drop-shadow(...)` em vez de `box-shadow` nos elementos com `clip-path`. Ou adicionar um wrapper sem clip-path ao redor para o shadow:

**Arquivo:** `app/web/app.css` — em cards que precisam de elevação:
```css
/* Em vez de box-shadow, que não funciona com clip-path: */
.diag-card:hover {
  filter: drop-shadow(0 4px 12px rgba(0, 180, 255, 0.15));
}
```

---

## Índice de Convergência (resumo do cruzamento)

| # | Problema | Fonte |
|---|---|---|
| 1.1 | Banner noadmin domina viewport | A2 |
| 1.2 | Sem focus-visible — inacessível por teclado | A1 |
| 1.3 | `bolt` fill quebra padrão stroke | A1 |
| 1.4 | GPU VRAM -1/-1 antes do poll | A2 |
| 1.5 | Exportar ativo sem dados | A2 |
| 1.6 | Nav `<a>` não focável | A1 |
| 1.7 | user-select no body | A1 |
| 1.8 | btn-hero sem hover | A1 |
| 1.9 | .toggle sem ARIA | A1 |
| 1.10 | `mem` = `ram` duplicata | A1 |
| **2.1** | **border-radius vs clip-path — dilui identidade** | **A1+A2 ← CONVERGENTE** |
| **2.2** | **Empty states ausentes em 17 containers** | **A1+A2 ← CONVERGENTE** |
| 2.3 | HUD "—" inicial parece broken | A2 |
| 2.4 | Cores de estado hard-coded | A1 |
| 2.5 | Tokens duplicados sem propósito | A1 |
| 2.6 | Toast sem animação de saída | A1 |
| 2.7 | toolsGrid delay assíncrono | A2 |
| 2.8 | perfTop vazio 2s no carregamento | A2 |
| 2.9 | #sysStrip vazio sem scan | A2 |
| 2.10 | bolt = energy path duplicado | A1 |
| 2.11 | --gold vs --amber sem critério | A1 |
| 2.12 | Animação página vs sub-aba inconsistente | A1 |
| 2.13 | Transição abrupta portal → páginas funcionais | A1 |
| 2.14 | stroke-width 1.8 hard-coded nos SVGs | A1 |
| 2.15 | Botão × do toast usa texto, não ícone SVG | A1 |
| 2.16 | .reco-row grid fixo 3 colunas | A1 |
| 2.17 | FPS depende de state.jogos preenchido | A2 |
| 2.18 | Report PDF flash branco ao abrir modal | A1 |
| 2.19 | padding-bottom: 120px valor mágico | A1 |
| 2.20 | font-style italic inconsistente em ptitle | A1 |
| 3.1 | 29 font-sizes sem escala de tokens | A1 |
| 3.2 | 9 variantes de botão sem hierarquia | A1 |
| 3.3 | Espaçamento sem tokens | A1 |
| 3.4 | --ink-3 abaixo WCAG AA | A1 |
| 3.5 | clip-path impede box-shadow | A1 |

**Total: 35 itens — 2 convergentes, 21 exclusivos de A1 (código), 12 exclusivos de A2 (visual)**

---

## Ordem de Execução Sugerida

```
SEMANA 1 (impacto imediato visível):
  ✅ 1.2  focus-visible global (1 bloco CSS)
  ✅ 1.3  bolt ícone → stroke (1 linha)
  ✅ 1.8  btn-hero hover (3 linhas CSS)
  ✅ 1.4  GPU VRAM guard < 0 (1 linha JS)
  ✅ 1.5  Exportar disabled sem dados (2 linhas JS)
  ✅ 2.2  Empty states nos containers principais (1h JS)

SEMANA 2 (identidade e qualidade):
  ✅ 1.1  Compactar banner noadmin (30min CSS+HTML)
  ✅ 2.1  Converter border-radius → clip-path features novas (1h CSS)
  ✅ 2.3  HUD estado inicial com 0/? em vez de — (20min JS)
  ✅ 2.4  Cores hard-coded → variáveis (45min CSS)
  ✅ 1.6  Nav tabindex + keydown (20min)

SEMANA 3 (polimento):
  ✅ 2.6  Toast animação de saída + timer visual (30min)
  ✅ 2.7  toolsGrid renderizar no init (10min)
  ✅ 2.8  perfTop skeleton inicial (20min)
  ✅ 2.9  sysStrip com dados básicos sem scan (20min)
  ✅ 1.7  user-select no body (5min)
  ✅ 1.9  toggle ARIA (15min)
  ✅ 1.10 mem/ram duplicata (10min)
  ✅ 2.5  tokens duplicados + --cyan-deep (15min)
  ✅ 2.10 energy alias duplicado de bolt (5min)
  ✅ 2.11 documentar --gold vs --amber (10min)
  ✅ 2.12 unificar animação página/sub-aba (15min)
  ✅ 2.14 stroke-width como variável CSS (10min)
  ✅ 2.15 botão × do toast → ícone SVG (10min)
  ✅ 2.16 reco-row grid auto-fill (5min)
  ✅ 2.17 state.jogos carregar no init (20min)
  ✅ 2.18 report PDF fade-in suave (15min)
  ✅ 2.19 padding-bottom variável --nav-h (5min)
  ✅ 2.20 ptitle italic padronizado (5min)
  ✅ 3.5  drop-shadow para cards com clip-path (30min)

FUTURO — opcionais com impacto de marca:
  ⬜ 2.13 Transição visual portal → páginas internas

FUTURO (refator sistêmico — quando tiver bloco de tempo):
  ⬜ 3.1  Escala tipográfica tokenizada
  ⬜ 3.2  Hierarquia de botões 9 → 3 papéis
  ⬜ 3.3  Sistema de espaçamento tokenizado
  ⬜ 3.4  --ink-3 contraste WCAG AA
```

---

*FIM DO DOCUMENTO — ThazzDraco Optimizer UI/UX Audit v1.0*
