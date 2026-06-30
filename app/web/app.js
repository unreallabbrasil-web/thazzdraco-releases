/* ============================================================================
   ThazzDraco Optimizer — app.js  (UI multipagina; vanilla, sem frameworks)
   Paginas: Inicio (Cockpit) · Otimizacoes (Secoes) · Limpeza · Historico
   ========================================================================== */
"use strict";

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const IC = (n) => (window.ICONS && window.ICONS[n]) || "";
function debounce(fn, ms) { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; }

const CLEANUP_ID = "cleanup.temp-files";

// Mapa categoria-da-regra -> secao da pagina Otimizacoes.
const SECTIONS = [
  { key: "energia",  label: "Energia",          icon: "bolt",    cats: ["Energia"] },
  { key: "jogos",    label: "Jogos & Windows",  icon: "game",    cats: ["Windows", "Input"] },
  { key: "gpu",      label: "GPU & Tela",       icon: "gpu",     cats: ["GPU", "Exibicao"] },
  { key: "memoria",  label: "Memória",          icon: "mem",     cats: ["Memoria"] },
  { key: "rede",     label: "Rede",             icon: "netcat",  cats: ["Rede"] },
  { key: "servicos", label: "Serviços",         icon: "svc",     cats: ["Servicos"] },
  { key: "sistema",  label: "Sistema",          icon: "sys",     cats: ["Registro"] },
];
function sectionOf(cat) {
  const s = SECTIONS.find((x) => x.cats.includes(cat));
  return s ? s.key : null; // null = limpeza (vai pra pagina propria)
}

const state = { scan: null, rules: [], byId: {}, page: "inicio", cat: "todas", query: "", scoreInicial: null, startup: [], bench: null, benchBase: null, fps: null, fpsBase: null, fpsDur: 30, fpsPoll: null, fpsRefreshing: false, diag: null, driver: null, repairPoll: null, bloat: null, deep: null, thermal: null, clientLogo: null, admin: true, gpuPanel: null, customJogos: [], sub: { manutencao: "limpeza", medicao: "desempenho" } };
function escHtml(s) { return (s || "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c])); }

/* ---- API ----------------------------------------------------------------- */
async function api(path, body) {
  const opt = body
    ? { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }
    : {};
  const r = await fetch(path, opt);
  if (!r.ok) throw new Error("HTTP " + r.status);
  return r.json();
}

/* ---- Busy / Toast -------------------------------------------------------- */
function busy(on, msg) { $("#busyMsg").textContent = msg || "Processando…"; $("#busy").classList.toggle("show", !!on); }
function toast(kind, title, sub) {
  const el = document.createElement("div");
  el.className = "toast " + kind;
  const icName = kind === "ok" ? "ok" : kind === "err" ? "err" : kind === "warn" ? "warn" : "info";
  el.innerHTML = `<span class="ti">${IC(icName)}</span>
    <div><b>${escHtml(title)}</b>${sub ? `<small>${escHtml(sub)}</small>` : ""}</div>
    <button class="toast-close" aria-label="Fechar">${IC("err")}</button>`;
  el.querySelector(".toast-close").onclick = () => dismissToast(el);
  $("#toasts").appendChild(el);
  const t1 = setTimeout(() => dismissToast(el), 3400);
  el._dismissTimer = t1;
}
function dismissToast(el) {
  if (!el.isConnected) return;
  clearTimeout(el._dismissTimer);
  el.style.cssText = "opacity:0;transform:translateX(20px);transition:.3s";
  setTimeout(() => el.remove(), 320);
}

/* ---- Portal (núcleo) ----------------------------------------------------- */
const R = 200, CX = 230, CY = 230, C = 2 * Math.PI * R, ARC = 0.75 * C, START = 135;
function scoreColor(s) { return s >= 90 ? "var(--green)" : s >= 70 ? "var(--cyan-br)" : s >= 50 ? "var(--amber)" : "var(--red)"; }
function buildGauge() {
  let runes = "";
  for (let i = 0; i < 60; i++) {
    const a = (i * 6) * Math.PI / 180, long = i % 5 === 0, r1 = R + 16, r2 = R + (long ? 30 : 24);
    runes += `<line x1="${CX + r1 * Math.cos(a)}" y1="${CY + r1 * Math.sin(a)}" x2="${CX + r2 * Math.cos(a)}" y2="${CY + r2 * Math.sin(a)}" stroke="${long ? "#5fd2ffaa" : "#2a577a"}" stroke-width="${long ? 2 : 1}"/>`;
  }
  $("#gauge").innerHTML = `
    <svg viewBox="0 0 460 460">
      <defs><filter id="gg" x="-60%" y="-60%" width="220%" height="220%"><feGaussianBlur stdDeviation="5" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter></defs>
      <g class="spincw"><circle cx="${CX}" cy="${CY}" r="${R + 42}" fill="none" stroke="#2ea8e6" stroke-width="1" stroke-dasharray="2 16" opacity=".45"/></g>
      <g class="spinccw">${runes}</g>
      <g class="spincw"><circle cx="${CX}" cy="${CY}" r="${R - 20}" fill="none" stroke="#1e6f9e" stroke-width="1.5" stroke-dasharray="40 18" opacity=".5"/></g>
      <circle cx="${CX}" cy="${CY}" r="${R}" fill="none" stroke="#0e2233" stroke-width="6" stroke-linecap="round" stroke-dasharray="${ARC} ${C}" transform="rotate(${START} ${CX} ${CY})"/>
      <circle id="gArc" cx="${CX}" cy="${CY}" r="${R}" fill="none" stroke="var(--cyan)" stroke-width="6" stroke-linecap="round" stroke-dasharray="0 ${C}" filter="url(#gg)" transform="rotate(${START} ${CX} ${CY})"/>
    </svg>`;
}
let __score = 0;
function setGauge(score) {
  const arc = $("#gArc"), num = $("#gNum");
  const col = scoreColor(score);
  document.documentElement.style.setProperty("--energy", col);
  if (!arc) { __score = score; return; }
  arc.style.stroke = col; if (num) num.style.color = col;
  // B1: CSS transition no arco (robusto em abas em segundo plano, sem rAF)
  if (!arc._tin) { arc.style.transition = "stroke-dasharray 1.1s cubic-bezier(0.25,1,0.5,1), stroke 0.4s"; arc._tin = true; }
  arc.style.strokeDasharray = `${(ARC * score) / 100} ${C}`;
  // Contador de número ainda usa rAF (conteúdo de texto não é animável via CSS)
  const from = __score, t0 = performance.now(), dur = 1100;
  (function step(t) {
    const k = Math.min(1, (t - t0) / dur), e = 1 - Math.pow(1 - k, 3), v = from + (score - from) * e;
    if (num) num.textContent = Math.round(v);
    if (k < 1) requestAnimationFrame(step);
  })(performance.now());
  __score = score;
  setTimeout(() => { if (num) num.textContent = Math.round(score); }, dur + 60);
}
function pulseEnergy() { // #2 pulso ao ativar
  const el = $("#energyPulse"); if (!el) return;
  el.classList.remove("on"); void el.offsetWidth; el.classList.add("on");
  setTimeout(() => el.classList.remove("on"), 760);
}
function setPerf(on) { // #5 modo performance
  document.body.classList.toggle("perf", on);
  const b = $("#btnPerf"); if (b) b.classList.toggle("on", on);
  try { localStorage.setItem("tz_perf", on ? "1" : "0"); } catch (e) {}
}

/* ---- Estresse de GPU (WebGL real) ---------------------------------------- */
// Renderiza um fractal pesado por pixel num framebuffer offscreen, em lotes com
// gl.finish() para forçar o trabalho da GPU (sem cap de vsync). Mede quantas
// passagens a GPU completa por segundo = throughput real. onSample(elapsed,total)
// roda a cada frame para a UI mostrar progresso/telemetria ao vivo.
const GPU_FS = `precision highp float;uniform vec2 uRes;uniform float uT;
void main(){vec2 uv=(gl_FragCoord.xy/uRes-0.5);uv.x*=uRes.x/uRes.y;
float zoom=2.6+sin(uT*0.7)*1.4;vec2 c=uv*zoom+vec2(-0.6+0.12*sin(uT*0.3),0.12*cos(uT*0.23));
vec2 z=vec2(0.0);float n=0.0;
for(int i=0;i<400;i++){z=vec2(z.x*z.x-z.y*z.y,2.0*z.x*z.y)+c;if(dot(z,z)>4.0)break;n+=1.0;}
float v=n/400.0;gl_FragColor=vec4(0.12+0.88*v,0.4*v,0.7*v+0.2,1.0);}`;
const GPU_VS = `attribute vec2 p;void main(){gl_Position=vec4(p,0.0,1.0);}`;

async function gpuStress(seconds, onSample) {
  const cv = document.createElement("canvas");
  cv.width = 1024; cv.height = 576;
  const gl = cv.getContext("webgl") || cv.getContext("experimental-webgl");
  if (!gl) return { ok: false, motivo: "Sem WebGL (GPU não acessível pelo navegador)." };
  const lose = () => { const ext = gl.getExtension("WEBGL_lose_context"); if (ext) ext.loseContext(); }; // libera o contexto em todo caminho
  const sh = (t, s) => { const o = gl.createShader(t); gl.shaderSource(o, s); gl.compileShader(o); return o; };
  const prog = gl.createProgram();
  gl.attachShader(prog, sh(gl.VERTEX_SHADER, GPU_VS));
  gl.attachShader(prog, sh(gl.FRAGMENT_SHADER, GPU_FS));
  gl.linkProgram(prog);
  if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) { lose(); return { ok: false, motivo: "Falha ao compilar shader." }; }
  gl.useProgram(prog);
  const buf = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buf);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  const loc = gl.getAttribLocation(prog, "p");
  gl.enableVertexAttribArray(loc); gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0);
  const uRes = gl.getUniformLocation(prog, "uRes"), uT = gl.getUniformLocation(prog, "uT");
  gl.uniform2f(uRes, cv.width, cv.height);
  gl.viewport(0, 0, cv.width, cv.height);

  const t0 = performance.now(), end = t0 + seconds * 1000;
  let passes = 0, batch = 1;
  const pix = new Uint8Array(4);
  return await new Promise((resolve) => {
    const frame = () => {
      const fs = performance.now();
      for (let i = 0; i < batch; i++) {
        gl.uniform1f(uT, (performance.now() - t0) / 1000 + i * 0.01);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
      }
      gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, pix); // força a GPU a terminar
      passes += batch;
      const dt = performance.now() - fs;
      // lotes grandes mantêm a GPU saturada (menos sincronização por passagem)
      if (dt < 42 && batch < 256) batch += Math.max(1, batch >> 2); else if (dt > 70 && batch > 1) batch--;
      const now = performance.now();
      if (onSample) onSample((now - t0) / 1000, passes);
      if (now < end) requestAnimationFrame(frame);
      else {
        const secs = (now - t0) / 1000;
        const pps = passes / secs;
        // score normalizado: passagens/s * (pixels/1e6) * (iters/100)
        const score = Math.round(pps * (cv.width * cv.height / 1e6) * 4);
        lose();
        resolve({ ok: true, score, pps: Math.round(pps), passes, segundos: +secs.toFixed(1) });
      }
    };
    requestAnimationFrame(frame);
  });
}
function spawnParticles() {
  const box = $("#particles"); if (!box) return;
  for (let i = 0; i < 18; i++) {
    const p = document.createElement("div"); p.className = "pt";
    p.style.left = (Math.random() * 100) + "%";
    p.style.animationDuration = (7 + Math.random() * 9) + "s";
    p.style.animationDelay = (-Math.random() * 14) + "s";
    p.style.opacity = (0.2 + Math.random() * 0.45);
    box.appendChild(p);
  }
}

/* ---- helpers de regra ---------------------------------------------------- */
function tierSafe(tier) {
  return { verde: { cls: "s1", label: "seguro" }, amarelo: { cls: "s2", label: "médio" }, vermelho: { cls: "s3", label: "avançado" } }[tier] || { cls: "s2", label: "?" };
}
// Uma regra é "acionável" (tem toggle) se não é consultiva nem limpeza.
function isToggleable(r) { return r.modo === "acionavel" && r.id !== CLEANUP_ID; }
function impactoRank(i) { return { alto: 0, medio: 1, baixo: 2 }[i] ?? 3; }

/* ---- Render: INÍCIO ------------------------------------------------------ */
function heroText(score) {
  if (score >= 90) return ["núcleo · carga máxima", "Máquina no talo", "Quase tudo ativo. Meça o FPS e prove o ganho — antes × depois."];
  if (score >= 70) return ["núcleo · forte", "Quase no ponto", "Faltam poucos módulos no console pra fechar o pacote."];
  if (score >= 40) return ["núcleo · parcial", "Tem FPS na mesa", "Vários módulos seguros esperando. Abra o console e destrave."];
  return ["núcleo · adormecido", "Dragão ainda dormindo", "Muito ganho latente aqui. Aplique um perfil rápido ou ative no console."];
}
function renderInicio() {
  const scan = state.scan; if (!scan) return; // sem scan ainda (estado idle)
  const tot = scan.totais || {};
  $("#cAplicados").textContent = tot.aplicados || 0;
  $("#cPendentes").textContent = tot.pendentes || 0;
  $("#cOportunidades").textContent = tot.oportunidades || 0;
  const [kick, title, sub] = heroText(scan.score || 0);
  $("#kicker").textContent = kick; $("#statusTitle").textContent = title; $("#statusSub").textContent = sub;
  const hs = $("#hudStatus"); if (hs) hs.textContent = `Núcleo online · ${tot.pendentes || 0} módulos pendentes`;
  setGauge(scan.score || 0);
  renderSysStrip(scan.perfil);
  renderReco();
}
// Faixa fina de HUD com o perfil do PC (substitui as tabelas na home).
function renderSysStrip(p) {
  const el = $("#sysStrip"); if (!el) return;
  delete el.dataset.filled;
  el.innerHTML = profileCards(p).map(([ic, lbl, val]) =>
    `<span class="sx"><i>${IC(ic)}</i>${val}</span>`).join("");
}
function renderReco() {
  const recs = [];
  // consultivas recomendadas (refresh/dns/xmp)
  state.rules.forEach((r) => { if (r.modo === "consultivo" && r.estado === "recomendado") recs.push({ r, kind: "guide" }); });
  // limpeza como oportunidade
  const clean = state.byId[CLEANUP_ID];
  if (clean) { const mb = cleanupMB(clean); if (mb >= 50) recs.push({ r: clean, kind: "clean", mb }); }
  // top seguros pendentes
  state.rules.filter((r) => isToggleable(r) && r.aplicavel && r.tier === "verde")
    .sort((a, b) => impactoRank(a.impacto) - impactoRank(b.impacto))
    .forEach((r) => recs.push({ r, kind: "apply" }));

  const list = recs.slice(0, 6);
  if (!list.length) { $("#reco").innerHTML = `<div class="reco-empty">Tudo certo por aqui — nenhuma recomendação pendente. 🐉</div>`; return; }
  $("#reco").innerHTML = list.map(({ r, kind, mb }) => {
    const sf = tierSafe(r.tier);
    let btn;
    if (kind === "guide") btn = `<button class="abtn guide" data-guide="${r.id}">Como fazer</button>`;
    else if (kind === "clean") btn = `<button class="abtn" data-goclean="1">Limpar ${fmtMB(mb)}</button>`;
    else btn = `<button class="abtn" data-apply="${r.id}">Aplicar</button>`;
    const desc = kind === "clean" ? "Só arquivos temporários/cache. Nunca toca em nada pessoal." : r.descricao;
    return `<div class="reco-card"><div class="rc-ttl">${escHtml(r.titulo)}</div><div class="rc-ds">${escHtml(desc)}</div>
      <div class="rc-ft"><span class="safe ${sf.cls}">${sf.label}</span>${btn}</div></div>`;
  }).join("");
}
function profileCards(p) {
  if (!p) return [];
  const ram = (p.hardware && p.hardware.ram) || {}, disp = p.display || {}, ctx = p.contexto || {};
  const dns = (p.rede && p.rede.dns) || [], discos = p.discos || [];
  const sys = discos.find((d) => d.sistema) || discos[0]; const out = [];
  out.push(["cpu", "Contexto", ctx.tipo === "notebook" ? "Notebook" : "Desktop"]);
  if (ram.total_gb) out.push(["ram", "Memória", `${ram.total_gb} GB${ram.configured_mhz ? " · " + ram.configured_mhz + " MHz" : ""}`]);
  if (sys) out.push(["disk", "Disco do sistema", sys.tipo]);
  if (disp.refresh_atual_hz) out.push(["display", "Tela", `${disp.refresh_atual_hz}${disp.refresh_max_hz && disp.refresh_max_hz !== disp.refresh_atual_hz ? " / " + disp.refresh_max_hz : ""} Hz`]);
  if (dns.length) out.push(["network", "DNS", dns[0]]);
  out.push([ctx.na_bateria ? "battery" : "plug", "Energia", ctx.na_bateria ? "Na bateria" : "Na tomada"]);
  return out;
}
function renderProfile(p) {
  $("#profile").innerHTML = profileCards(p).map(([ic, lbl, val]) =>
    `<div class="pcard"><div class="pic">${IC(ic)}</div><div class="pmeta"><span>${lbl}</span><b title="${val}">${val}</b></div></div>`).join("");
}

/* ---- Render: OTIMIZAÇÕES ------------------------------------------------- */
function sectionRules(key) {
  return state.rules.filter((r) => r.id !== CLEANUP_ID && (key === "todas" ? sectionOf(r.categoria) : sectionOf(r.categoria) === key));
}
function pendCount(rules) { return rules.filter((r) => r.estado === "pendente" || r.estado === "recomendado").length; }
function renderSubnav() {
  const items = [{ key: "todas", label: "Todas", icon: "bolt" }, ...SECTIONS.filter((s) => sectionRules(s.key).length)];
  $("#subnav").innerHTML = items.map((s) => {
    const rs = sectionRules(s.key), pc = pendCount(rs);
    return `<a class="${state.cat === s.key ? "on" : ""}" data-cat="${s.key}"><span class="sic">${IC(s.icon)}</span>${s.label}${pc ? `<span class="cnt">${pc}</span>` : ""}</a>`;
  }).join("");
}
function renderOtimRules() {
  const sec = SECTIONS.find((s) => s.key === state.cat);
  let rules = sectionRules(state.cat);
  const pc = pendCount(rules);
  const box = $("#otimRules");

  if (state.cat === "todas" && !state.query) {
    // Vista "Todas": grid de cards por categoria
    $("#otimTitle").textContent = "Otimizações";
    const totalPend = pendCount(sectionRules("todas"));
    $("#otimSub").textContent = totalPend ? `${totalPend} ajuste(s) disponíveis — escolha uma categoria.` : "Tudo aplicado. Seu PC está otimizado.";
    $("#btnOtimSecao").style.display = "none";
    box.innerHTML = `<div class="sec-overview">${SECTIONS.map(s => {
      const sr = sectionRules(s.key); if (!sr.length) return "";
      const p = pendCount(sr), na = sr.filter(r => r.estado === "nao-aplicavel").length, tot = sr.length - na;
      return `<div class="sec-card" data-cat="${s.key}" tabindex="0" role="button">
        <div class="sco-ico">${IC(s.icon)}</div>
        <div class="sco-body">
          <div class="sco-name">${escHtml(s.label)}</div>
          ${p ? `<div class="sco-pend">${p} pendente${p>1?"s":""}</div>` : `<div class="sco-ok">✓ tudo ok</div>`}
          <div class="sco-meta">${tot} regra${tot!==1?"s":""}</div>
        </div>
        <span class="sco-arr">›</span>
      </div>`;
    }).join("")}</div>`;
    $$("#otimRules .sec-card").forEach(el => {
      const go = () => { state.cat = el.dataset.cat; renderSubnav(); renderOtimRules(); };
      el.addEventListener("click", go);
      el.addEventListener("keydown", e => { if (e.key === "Enter" || e.key === " ") go(); });
    });
    return;
  }

  // Vista por categoria
  $("#otimTitle").textContent = sec ? sec.label : "Otimizações";
  $("#otimSub").textContent = pc ? `${pc} pendente(s) nesta seção.` : "Tudo aplicado por aqui.";
  $("#btnOtimSecao").style.display = rules.some((r) => isToggleable(r) && r.aplicavel) ? "" : "none";
  if (state.query) { const q = state.query.toLowerCase(); rules = rules.filter((r) => (r.titulo + " " + r.descricao).toLowerCase().includes(q)); }
  if (!rules.length) { box.innerHTML = `<div class="empty">${IC("empty")}<div>Nada nesta busca.</div></div>`; return; }

  // Pendentes primeiro (full card), aplicados/N-A colapsáveis e compactos
  const pending = rules.filter(r => r.estado !== "aplicado" && r.estado !== "nao-aplicavel");
  const done    = rules.filter(r => r.estado === "aplicado" || r.estado === "nao-aplicavel");

  let html = pending.map(ruleRow).join("");
  if (done.length) {
    html += `<details class="done-section"><summary>${done.length} já aplicado${done.length>1?"s":""} / não se aplic${done.length>1?"am":"a"} neste PC</summary>
      <div class="done-list">${done.map(ruleRowCompact).join("")}</div></details>`;
  }
  box.innerHTML = html;
}

function ruleRowCompact(r) {
  const on = r.estado === "aplicado";
  const tierCol = { verde: "var(--t-verde)", amarelo: "var(--t-amarelo)", vermelho: "var(--t-vermelho)" }[r.tier] || "var(--line)";
  let ctrl = "";
  if (r.estado === "nao-aplicavel") ctrl = `<div class="toggle disabled"><i></i></div>`;
  else if (isToggleable(r)) ctrl = `<div class="toggle on" data-toggle="${r.id}"><i></i></div>`;
  return `<div class="rule rule-compact ${r.estado === "nao-aplicavel" ? "na" : ""}" style="--tc:${tierCol}">
    <div class="body"><span class="title">${escHtml(r.titulo)}</span></div>
    <div class="act"><button class="infobtn" data-tec="${r.id}" title="Detalhes">${IC("info")}</button>${ctrl}</div>
  </div>`;
}
function ruleRow(r) {
  const sf = tierSafe(r.tier);
  const tierCol = { verde: "var(--t-verde)", amarelo: "var(--t-amarelo)", vermelho: "var(--t-vermelho)" }[r.tier];
  const flags = [];
  if (r.requer_reboot) flags.push(`<span class="pill flag">reinício</span>`);
  // controle à direita
  let ctrl, stCls, stTxt;
  if (isToggleable(r)) {
    if (r.estado === "nao-aplicavel") { ctrl = `<div class="toggle disabled"><i></i></div>`; stCls = "na"; stTxt = "não se aplica neste PC"; }
    else { const on = r.estado === "aplicado"; ctrl = `<div class="toggle ${on ? "on" : ""}" data-toggle="${r.id}"><i></i></div>`; stCls = on ? "aplicado" : "pendente"; stTxt = on ? "aplicado" : "pendente"; }
  } else { ctrl = `<button class="abtn guide" data-guide="${r.id}">Como fazer</button>`; stCls = "aviso"; stTxt = r.estado === "recomendado" ? "recomendado" : "consultivo"; }
  return `<div class="rule ${r.estado === "nao-aplicavel" ? "na" : ""}" style="--tc:${tierCol}">
    <div class="body">
      <div class="top"><span class="title">${escHtml(r.titulo)}</span><span class="safe ${sf.cls}">${sf.label}</span>${flags.join("")}</div>
      <div class="desc">${escHtml(r.descricao)}</div>
      <div class="st ${stCls}"><span class="sd"></span>${stTxt}${r.detalhe ? ` · <span style="color:var(--ink-4)">${escHtml(r.detalhe)}</span>` : ""}</div>
    </div>
    <div class="act"><button class="infobtn" data-tec="${r.id}" title="Detalhes técnicos">${IC("info")}</button>${ctrl}</div>
  </div>`;
}
function tecModal(r) {
  const flags = [];
  if (r.requer_reboot) flags.push("pede reinício");
  if (r.requer_consentimento) flags.push("pede consentimento");
  infoModal(escHtml(r.titulo), `<p>${escHtml(r.descricao)}</p>
    <div class="tec-box"><span class="k">O que altera no sistema</span>${escHtml(r.tecnico || "—")}</div>
    ${r.orientacao ? `<div class="tec-box" style="color:#bfe0f5"><span class="k">Como fazer</span>${escHtml(r.orientacao)}</div>` : ""}
    <div style="margin-top:12px;font-size:12px;color:var(--ink-3)">Nível: <b style="color:var(--ink-2)">${escHtml(r.tier)}</b>${flags.length ? " · " + flags.join(" · ") : ""}</div>`);
}

/* ---- Render: LIMPEZA ----------------------------------------------------- */
function cleanupMB(r) { const m = /([\d.]+)\s*MB/.exec(r.detalhe || ""); return m ? parseFloat(m[1]) : 0; }
function fmtMB(mb) { return mb >= 1024 ? (mb / 1024).toFixed(1) + " GB" : Math.round(mb) + " MB"; }
function renderLimpeza() {
  const r = state.byId[CLEANUP_ID];
  if (!r) { $("#cleanCard").innerHTML = ""; return; }
  const mb = cleanupMB(r);
  $("#cleanCard").innerHTML = `
    <div class="cc-ring">
      <svg viewBox="0 0 128 128" width="128" height="128"><circle cx="64" cy="64" r="54" fill="none" stroke="#16273a" stroke-width="9"/><circle cx="64" cy="64" r="54" fill="none" stroke="var(--cyan)" stroke-width="9" stroke-linecap="round" stroke-dasharray="${Math.min(1, mb / 4000) * 339} 339" transform="rotate(-90 64 64)" style="filter:drop-shadow(0 0 5px #2ea8e6)"/></svg>
      <div class="ccv"><b>${fmtMB(mb).split(" ")[0]}</b><span>${fmtMB(mb).split(" ")[1] || "MB"}</span></div>
    </div>
    <div class="cc-body">
      <h2>Limpeza de temporários</h2>
      <p>Encontramos <b>${fmtMB(mb)}</b> em arquivos temporários e cache. Apagar libera espaço e I/O — e é totalmente regenerável.</p>
      <div class="cc-act"><button class="btn-hero" data-apply="${CLEANUP_ID}"><span data-icon="broom"></span> Limpar agora</button>
      <span class="safe s1">seguro · só temp</span></div>
    </div>`;
  $$("#cleanCard [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
}

/* ---- Render: HISTÓRICO --------------------------------------------------- */
async function renderHistorico() {
  // F5: renderiza o chart de performance
  renderPerfChart();
  let h = [];
  try { h = (await api("/api/historico")) || []; } catch (e) {}
  if (!h.length) { $("#histList").innerHTML = `<div class="empty">${IC("history")}<div>Nenhum lote aplicado ainda.</div></div>`; $("#btnUndoAll").style.display = "none"; return; }
  $("#btnUndoAll").style.display = "";
  $("#histList").innerHTML = [...h].reverse().map((b) =>
    `<div class="hist-item"><div class="hnum">${b.rules.length}</div>
      <div class="hi"><b>${b.rules.length} otimização(ões)</b><span>${b.quando} · ${origemLabel(b.origem)}</span></div>
      <button class="abtn guide hu" data-batch="${b.id}">Desfazer lote</button></div>`).join("");
}
function origemLabel(o) { if (!o) return "manual"; if (o.startsWith("preset:")) return "perfil " + o.slice(7); return { selecao: "seleção", verdes: "verdes", regra: "individual" }[o] || o; }

/* ---- Render geral -------------------------------------------------------- */
function renderAll() {
  if (!state.scan) return;
  $("#navOtim").textContent = (state.scan.totais || {}).pendentes || "";
  renderInicio(); renderSubnav(); renderOtimRules(); renderLimpeza();
  if (state.page === "historico") renderHistorico();
}
function ingest(scan) {
  state.scan = scan; state.rules = scan.regras || []; state.byId = {};
  state.rules.forEach((r) => (state.byId[r.id] = r));
  if (state.scoreInicial == null) state.scoreInicial = scan.score || 0; // p/ relatório antes×depois
  renderAll();
}

/* ---- Router (páginas + sub-abas) ----------------------------------------- */
// Páginas que têm sub-abas: prefixo dos panes (pane-<prefixo>-<sub>).
const SUBPAGES = {
  manutencao: { nav: "subManut", prefix: "mn", tabs: ["limpeza", "reparo", "debloat", "ferramentas", "inicializacao", "browsers"] },
  medicao:    { nav: "subMed",   prefix: "md", tabs: ["desempenho", "fps", "benchmark"] },
};

function showPage(name) {
  state.page = name;
  $$(".mainnav a").forEach((a) => a.classList.toggle("active", a.dataset.page === name));
  $$(".page").forEach((p) => p.classList.toggle("active", p.id === "page-" + name));
  $(".pages").scrollTop = 0;
  if (SUBPAGES[name]) { showSub(name, state.sub[name]); return; } // delega à sub-aba ativa
  stopLiveJobs();             // saiu de toda página com job ao vivo
  if (name === "historico") renderHistorico();
  if (name === "jogos") { renderJogos(); initLiveGameBar(); }
  if (name === "gpu") renderGpu();
  if (name === "diagnostico") { if (state.diag) { renderDiag(); renderDriver(); } else runDiag(); }
}

// showSub ativa uma sub-aba (e a página dona, se ainda não estiver).
function showSub(page, sub) {
  const cfg = SUBPAGES[page]; if (!cfg) return showPage(page);
  if (!cfg.tabs.includes(sub)) sub = cfg.tabs[0];
  state.sub[page] = sub;
  if (state.page !== page) return showPage(page); // showPage chama showSub de volta
  $$("#" + cfg.nav + " button").forEach((b) => b.classList.toggle("active", b.dataset.sub === sub));
  cfg.tabs.forEach((t) => { const pn = $("#pane-" + cfg.prefix + "-" + t); if (pn) pn.classList.toggle("active", t === sub); });
  $(".pages").scrollTop = 0;
  stopLiveJobs(page + ":" + sub); // para o que não é desta sub-aba
  // dispara o render da sub-aba ativa
  if (page === "manutencao") {
    if (sub === "limpeza" && !state.deep) renderDeepClean();
    else if (sub === "debloat" && !state.bloat) renderBloat();
    else if (sub === "ferramentas") renderTools();
    else if (sub === "inicializacao") renderStartup();
    else if (sub === "browsers") renderBrowserClean();
    else if (sub === "reparo" && !state.repairPoll) api("/api/reparo/status").then((st) => { if (st.estado === "rodando" || st.log?.length) pollRepair(); }).catch(() => {});
  } else if (page === "medicao") {
    if (sub === "desempenho") { startMetrics(); renderHealth(); }
    else if (sub === "fps") { renderFpsGames(); renderFps(); }
    else if (sub === "benchmark") renderBench();
  }
}

const LIVE = Object.freeze({ METRICAS: "medicao:desempenho", FPS: "medicao:fps", REPARO: "manutencao:reparo" });

function stopLiveJobs(dest) {
  if (dest !== LIVE.METRICAS) stopMetrics();
  if (dest !== LIVE.FPS && state.fpsPoll) { clearInterval(state.fpsPoll); state.fpsPoll = null; }
  if (dest !== LIVE.REPARO && state.repairPoll) { clearInterval(state.repairPoll); state.repairPoll = null; }
  if (dest !== "jogos") stopLiveGamePoll(); // F1: para o poll de Game ao Vivo ao sair da aba
}

// navTo: navegação por nome lógico (deep-links de diagnóstico/limpeza).
function navTo(target) {
  const map = { limpeza: ["manutencao", "limpeza"], inicializacao: ["manutencao", "inicializacao"], desempenho: ["medicao", "desempenho"], fps: ["medicao", "fps"], benchmark: ["medicao", "benchmark"] };
  if (map[target]) showSub(map[target][0], map[target][1]); else showPage(target);
}

/* ---- Inicialização ------------------------------------------------------- */
async function renderStartup() {
  const box = $("#startupList");
  box.innerHTML = `<div class="empty">${IC("ring")}<div>Lendo inicialização…</div></div>`;
  try { renderStartupList(await api("/api/inicializacao")); }
  catch (e) { box.innerHTML = `<div class="empty">${IC("err")}<div>Falha ao ler.</div></div>`; }
}
function renderStartupList(list) {
  state.startup = list || [];
  const on = state.startup.filter((e) => e.enabled).length, total = state.startup.length;
  $("#startupHead").innerHTML = `<span class="pill2"><b>${total}</b> programas</span><span class="pill2"><b>${on}</b> ativos</span><span class="pill2"><b>${total - on}</b> desligados</span>`;
  const box = $("#startupList");
  if (!total) { box.innerHTML = `<div class="empty">${IC("empty")}<div>Nenhuma entrada de inicialização.</div></div>`; return; }
  box.innerHTML = state.startup.map((e, i) => `<div class="rule" style="--tc:${e.enabled ? "var(--green)" : "var(--ink-4)"}">
    <div class="body"><div class="top"><span class="title">${escHtml(e.name)}</span><span class="loc-pill">${escHtml(e.location)}</span></div>
      <div class="cmd-path">${escHtml(e.command)}</div></div>
    <div class="act"><div class="toggle ${e.enabled ? "on" : ""}" data-startup="${i}"><i></i></div></div>
  </div>`).join("");
}
async function startupToggle(i, el) {
  const e = state.startup[i]; if (!e) return;
  el.classList.add("busy");
  try {
    const d = await api("/api/inicializacao/set", { kind: e.kind, key: e.key, enabled: !e.enabled });
    if (d.lista) renderStartupList(d.lista);
    toast(!e.enabled ? "ok" : "info", !e.enabled ? "Ativado na inicialização" : "Desligado da inicialização", e.name);
  } catch (err) { toast("err", "Falha", err.message); el.classList.remove("busy"); }
}

/* ---- Desempenho (métricas ao vivo) --------------------------------------- */
let metricsTimer = null;
function startMetrics() { if (metricsTimer) return; pollMetrics(); metricsTimer = setInterval(pollMetrics, 2000); }
function stopMetrics() { if (metricsTimer) { clearInterval(metricsTimer); metricsTimer = null; } }
async function pollMetrics() {
  try {
    const m = await api("/api/metricas");
    renderMetrics(m);
    perfHistoryPush(m);
    // 2.9: preenche sysStrip com dados básicos se scan ainda não rodou
    if (!state.scan) initSysStripBasic(m);
  } catch (e) {}
}
function initSysStripBasic(m) {
  const el = $("#sysStrip"); if (!el || el.dataset.filled) return;
  const cpu = m.cpu_pct || 0, ram = m.ram || {}, up = m.uptime || "—";
  el.innerHTML =
    `<span class="sx"><i>${IC("cpu")}</i>${cpu}% CPU</span>` +
    `<span class="sx"><i>${IC("ram")}</i>${(ram.usado_gb||0).toFixed(1)} / ${(ram.total_gb||0).toFixed(1)} GB</span>` +
    `<span class="sx"><i>${IC("history")}</i>${up}</span>`;
  el.dataset.filled = "1";
}
function miniRing(pct, color) {
  const r = 40, arc = 2 * Math.PI * r, off = arc * (1 - Math.max(0, Math.min(100, pct)) / 100);
  return `<svg viewBox="0 0 96 96"><circle cx="48" cy="48" r="${r}" fill="none" stroke="#16273a" stroke-width="7"/>
    <circle cx="48" cy="48" r="${r}" fill="none" stroke="${color}" stroke-width="7" stroke-linecap="round" stroke-dasharray="${arc}" stroke-dashoffset="${off}" transform="rotate(-90 48 48)" style="transition:stroke-dashoffset .5s ease;filter:drop-shadow(0 0 5px ${color})"/></svg>`;
}
function renderMetrics(m) {
  const cpu = m.cpu_pct || 0, ram = m.ram || {}, rp = ram.pct || 0;
  const cpuCol = cpu >= 85 ? "var(--red)" : cpu >= 60 ? "var(--amber)" : "var(--cyan)";
  const ramCol = rp >= 85 ? "var(--red)" : rp >= 70 ? "var(--amber)" : "var(--green)";
  const livre = Math.max(0, (ram.total_gb || 0) - (ram.usado_gb || 0)).toFixed(1);
  let gpuHtml = "";
  (m.gpus || []).forEach((g) => {
    if (g.temp_c >= 0 || g.uso_pct >= 0) { // sensor real (NVML/ADL)
      const u = g.uso_pct >= 0 ? g.uso_pct : 0;
      const gcol = (g.temp_c >= 83 || u >= 90) ? "var(--red)" : (g.temp_c >= 70 || u >= 65) ? "var(--amber)" : "var(--cyan)";
      const big = g.temp_c >= 0 ? g.temp_c + "°" : u, lbl = g.temp_c >= 0 ? "°C GPU" : "% GPU";
      const vram = g.vram_tot_mb > 0 ? `VRAM ${(g.vram_uso_mb / 1024).toFixed(1)} / ${(g.vram_tot_mb / 1024).toFixed(1)} GB` : "";
      const sub = [g.uso_pct >= 0 ? "Uso " + g.uso_pct + "%" : "", g.temp_c >= 0 ? g.temp_c + "°C" : ""].filter(Boolean).join(" · ");
      gpuHtml += `<div class="perf-card"><div class="perf-ring">${miniRing(g.temp_c >= 0 ? Math.min(100, g.temp_c) : u, gcol)}<div class="pv"><b>${big}</b><span>${lbl}</span></div></div>
        <div class="perf-meta"><b>${escHtml(g.nome)}</b><span>${vram || "Placa de vídeo"}</span><span class="up">${sub}</span></div></div>`;
    } else if (g.vendor === "AMD" || g.vendor === "NVIDIA") { // nome sem sensor
      gpuHtml += `<div class="perf-card"><div class="perf-ring" style="opacity:.45">${miniRing(0, "var(--ink-4)")}<div class="pv"><b>—</b><span>GPU</span></div></div>
        <div class="perf-meta"><b>${escHtml(g.nome)}</b><span>${escHtml(g.nota || "Sensor a validar")}</span></div></div>`;
    }
  });
  // F4: temperatura da CPU via PDH thermal zone
  const cpuTemp = (m.cpu_temp_c != null && m.cpu_temp_c >= 0) ? m.cpu_temp_c : null;
  const tempStr = cpuTemp !== null ? ` · ${cpuTemp}°C` : "";
  const tempCol = cpuTemp !== null ? (cpuTemp >= 90 ? "var(--red)" : cpuTemp >= 75 ? "var(--amber)" : "") : "";
  const cpuSub = cpuTemp !== null
    ? `<span class="up${tempCol ? " cpu-temp-hot" : ""}" style="${tempCol ? "color:" + tempCol : ""}">Ligado há ${m.uptime || "—"}${tempStr}</span>`
    : `<span class="up">Ligado há ${m.uptime || "—"}</span>`;
  $("#perfTop").innerHTML =
    `<div class="perf-card"><div class="perf-ring">${miniRing(cpu, cpuCol)}<div class="pv"><b>${cpu}</b><span>% CPU</span></div></div>
      <div class="perf-meta"><b>Processador</b><span>Uso atual da CPU</span>${cpuSub}</div></div>
    <div class="perf-card"><div class="perf-ring">${miniRing(rp, ramCol)}<div class="pv"><b>${rp}</b><span>% RAM</span></div></div>
      <div class="perf-meta"><b>Memória</b><span>${ram.usado_gb || 0} de ${ram.total_gb || 0} GB em uso</span><span class="up">${livre} GB livres</span></div></div>${gpuHtml}`;
  // detalhamento de memória (diagnóstico do RAM alto)
  const md = m.mem_detail || {}, np = md.pool_naopag_mb || 0, npWarn = np > 2500;
  const mEl = $("#memDetail");
  if (mEl) mEl.innerHTML = md.confirmada_mb ? `
    <span class="mdp"><i>Confirmada</i>${(md.confirmada_mb / 1024).toFixed(1)} GB</span>
    <span class="mdp"><i>Em cache</i>${(md.cache_mb / 1024).toFixed(1)} GB</span>
    <span class="mdp"><i>Pool paginado</i>${md.pool_paginado_mb} MB</span>
    <span class="mdp ${npWarn ? "warn" : ""}"><i>Pool não-paginado</i>${np} MB${npWarn ? " ⚠ alto (driver?)" : ""}</span>
    <span class="mdp"><i>Processos</i>${md.processos}</span>` : "";
  const procs = m.processos || [], max = Math.max(1, ...procs.map((p) => p.mem_mb));
  $("#procList").innerHTML = procs.map((p) =>
    `<div class="proc"><div class="prow"><span class="pn">${escHtml(p.nome)}</span><span class="pmem">${p.mem_mb >= 1024 ? (p.mem_mb / 1024).toFixed(1) + " GB" : p.mem_mb + " MB"}</span></div><div class="pbar"><i style="width:${Math.round(p.mem_mb / max * 100)}%"></i></div></div>`).join("");
}

/* ---- Saúde do PC --------------------------------------------------------- */
async function renderHealth() {
  const box = $("#healthBox"); if (!box) return;
  box.innerHTML = `<div class="empty" style="padding:24px">${IC("ring")}<div>Lendo saúde…</div></div>`;
  let h; try { h = await api("/api/saude"); } catch (e) { box.innerHTML = `<div class="empty">${IC("err")}<div>Falha ao ler.</div></div>`; return; }
  const smartByNum = {}; (h.smart || []).forEach((s) => (smartByNum[s.numero] = s));
  const cards = [];
  // discos (espaço + S.M.A.R.T.)
  (h.discos || []).forEach((d) => {
    const col = d.usado_pct >= 90 ? "var(--red)" : d.usado_pct >= 75 ? "var(--amber)" : "var(--green)";
    cards.push(`<div class="hcard"><div class="hc-ic">${IC("disk")}</div>
      <div class="hc-body"><div class="hc-top"><b>Disco ${d.letra}</b><span class="hc-tag">${d.livre_gb} GB livres de ${d.total_gb} GB</span></div>
      <div class="hbar"><i style="width:${d.usado_pct}%;background:${col};box-shadow:0 0 10px ${col}"></i></div>
      <span class="hc-sub">${d.usado_pct}% usado</span></div></div>`);
  });
  // S.M.A.R.T. por disco físico
  (h.smart || []).forEach((s) => {
    const ok = s.saude === "Saudável", warn = s.saude === "Atenção";
    const col = ok ? "var(--green)" : warn ? "var(--red)" : "var(--ink-3)";
    cards.push(`<div class="hcard"><div class="hc-ic" style="color:${col}">${IC("shield")}</div>
      <div class="hc-body"><div class="hc-top"><b>${s.tipo} #${s.numero}</b><span class="hc-tag" style="color:${col}">${s.saude}</span></div>
      <span class="hc-sub">Saúde S.M.A.R.T. (previsão de falha)</span></div></div>`);
  });
  // RAM
  const ram = h.ram || {};
  if (ram.total_gb) {
    const col = ram.pct >= 85 ? "var(--red)" : ram.pct >= 70 ? "var(--amber)" : "var(--green)";
    cards.push(`<div class="hcard"><div class="hc-ic">${IC("ram")}</div>
      <div class="hc-body"><div class="hc-top"><b>Memória</b><span class="hc-tag">${ram.usado_gb} de ${ram.total_gb} GB</span></div>
      <div class="hbar"><i style="width:${ram.pct}%;background:${col};box-shadow:0 0 10px ${col}"></i></div>
      <span class="hc-sub">${ram.pct}% em uso</span></div></div>`);
  }
  // Bateria / energia
  if (h.bateria) {
    const b = h.bateria;
    if (b.presente) {
      const col = b.pct >= 40 ? "var(--green)" : b.pct >= 20 ? "var(--amber)" : "var(--red)";
      const tempo = b.minutos_restantes > 0 ? ` · ~${Math.floor(b.minutos_restantes / 60)}h${b.minutos_restantes % 60}m` : "";
      cards.push(`<div class="hcard"><div class="hc-ic">${IC("battery")}</div>
        <div class="hc-body"><div class="hc-top"><b>Bateria</b><span class="hc-tag">${b.na_tomada ? "Carregando" : "Na bateria"}${tempo}</span></div>
        <div class="hbar"><i style="width:${b.pct < 0 ? 0 : b.pct}%;background:${col};box-shadow:0 0 10px ${col}"></i></div>
        <span class="hc-sub">${b.pct < 0 ? "—" : b.pct + "%"}</span></div></div>`);
    } else {
      cards.push(`<div class="hcard"><div class="hc-ic">${IC("plug")}</div>
        <div class="hc-body"><div class="hc-top"><b>Energia</b><span class="hc-tag">${b.na_tomada ? "Na tomada" : "—"}</span></div>
        <span class="hc-sub">Desktop (sem bateria)</span></div></div>`);
    }
  }
  box.innerHTML = cards.join("");
  const note = $("#healthNote"); if (!note) box.insertAdjacentHTML("afterend", `<div class="health-note" id="healthNote">🌡️ ${escHtml(h.temp_nota || "")}</div>`);
  else note.textContent = "🌡️ " + (h.temp_nota || "");
}

/* ---- Relatório do cliente ------------------------------------------------ */
function scoreHex(s) { return s >= 90 ? "#1a9e6b" : s >= 70 ? "#1f7fb8" : s >= 50 ? "#d98a1f" : "#d23f3f"; }
/* ---- Otimização por jogo ------------------------------------------------- */
async function renderJogos() {
  const box = $("#jogosList");
  box.innerHTML = `<div class="empty">${IC("ring")}<div>Procurando jogos…</div></div>`;
  let list; try { list = await api("/api/jogos"); } catch (e) { box.innerHTML = `<div class="empty">${IC("err")}<div>Falha ao ler.</div></div>`; return; }
  state.jogos = [...(list || []), ...state.customJogos];
  if (!state.jogos.length) { box.innerHTML = `<div class="empty">${IC("gamepad")}<div>Nenhum jogo detectado. Adicione um abaixo.</div></div>`; return; }
  box.innerHTML = `<div class="game-grid">${state.jogos.map((g, i) => gameCard(g, i)).join("")}</div>`;
}

function bestCoverUrl(g) {
  if (g.cover_url) return g.cover_url;
  if (g.loja === "Steam" && g.app_id) return `https://cdn.steamstatic.com/steam/apps/${g.app_id}/library_600x900.jpg`;
  return null;
}

function gameCard(g, i) {
  const coverUrl = bestCoverUrl(g);
  const active = [g.fso, g.gpu, g.prio, g.av].filter(Boolean).length;
  // fallback chain: library_600x900 → header.jpg → nocover
  const imgSrc = coverUrl || (g.loja === "Steam" && g.app_id ? `https://cdn.steamstatic.com/steam/apps/${g.app_id}/header.jpg` : "");
  const imgHtml = imgSrc
    ? `<img src="${escHtml(imgSrc)}" loading="lazy" alt="" data-gidx="${i}" onerror="gameCoverFallback(this,${i})">`
    : "";
  return `<div class="gcard" data-gidx="${i}" tabindex="0" role="button" aria-label="${escHtml(g.nome)}">
    <div class="gcard-cover${imgSrc ? "" : " nocover"}">
      ${imgHtml}
      <div class="gcard-cover-ph">${IC("gamepad")}</div>
      ${active > 0 ? `<div class="gcard-badge">${active} ativo${active > 1 ? "s" : ""}</div>` : ""}
    </div>
    <div class="gcard-body">
      <div class="gcard-name">${escHtml(g.nome)}</div>
      <div class="gcard-meta">
        <span class="loc-pill">${escHtml(g.loja)}</span>
        ${g.perfil ? `<span class="prof-pill" style="font-size:10px">🎯 ${escHtml(g.perfil.tipo)}</span>` : ""}
      </div>
      <div class="gcard-dots">
        <span class="gdot${g.fso ? " on" : ""}" title="FSO off"></span>
        <span class="gdot${g.gpu ? " on" : ""}" title="GPU máx"></span>
        <span class="gdot${g.prio ? " on" : ""}" title="CPU alta"></span>
        <span class="gdot${g.av ? " on" : ""}" title="Anti-vírus"></span>
      </div>
    </div>
  </div>`;
}

// Fallback encadeado: library_600x900 → header.jpg → busca Steam por nome → nocover
async function gameCoverFallback(img, idx) {
  const g = state.jogos[idx]; if (!g) { img.closest('.gcard-cover').classList.add('nocover'); return; }
  const cur = img.src;
  if (g.loja === "Steam" && g.app_id && cur.includes("library_600x900")) {
    // tenta header.jpg
    img.onerror = () => { img.closest('.gcard-cover').classList.add('nocover'); };
    img.src = `https://cdn.steamstatic.com/steam/apps/${g.app_id}/header.jpg`;
    return;
  }
  // busca via Steam Search API (backend)
  if (g._coverResolved) { img.closest('.gcard-cover').classList.add('nocover'); return; }
  g._coverResolved = true;
  try {
    const r = await api("/api/jogos/cover?name=" + encodeURIComponent(g.nome));
    if (r.url) {
      g.cover_url = r.url;
      img.onerror = () => { img.closest('.gcard-cover').classList.add('nocover'); };
      img.src = r.url;
    } else {
      img.closest('.gcard-cover').classList.add('nocover');
    }
  } catch { img.closest('.gcard-cover').classList.add('nocover'); }
}

/* ---- Painel por jogo -------------------------------------------------------- */
function openGamePanel(i) {
  const g = state.jogos[i]; if (!g) return;
  state.activeGameIdx = i;

  const coverUrl = bestCoverUrl(g);
  const coverHtml = coverUrl
    ? `<img src="${escHtml(coverUrl)}" alt="" onerror="this.closest('.gph-cover').classList.add('nocover')">`
    : "";

  $("#gpContent").innerHTML = `
    <div class="gph-cover${coverUrl ? "" : " nocover"}">
      ${coverHtml}
      <div class="gph-ph">${IC("gamepad")}</div>
      <div class="gph-overlay">
        <div class="gph-name">${escHtml(g.nome)}</div>
        <div class="gph-meta"><span class="loc-pill">${escHtml(g.loja)}</span>${g.perfil ? `<span class="prof-pill" style="font-size:10px">🎯 ${escHtml(g.perfil.tipo)}</span>` : ""}</div>
      </div>
    </div>
    <div class="gp-tabs">
      <button class="gp-tab active" data-tab="tweaks">Tweaks</button>
      <button class="gp-tab" data-tab="config">Config Gráfica</button>
      <button class="gp-tab" data-tab="nvidia">NVIDIA</button>
    </div>
    <div id="gpTab-tweaks" class="gp-tab-content active">${renderPanelTweaks(g, i)}</div>
    <div id="gpTab-config" class="gp-tab-content">
      <div class="gcfg-loading">${IC("ring")}<div>Lendo config do jogo…</div></div>
    </div>
    <div id="gpTab-nvidia" class="gp-tab-content"><div class="gcfg-loading">${IC("ring")}<div>Detectando GPU…</div></div></div>`;

  const panel = $("#gamePanel");
  panel.classList.add("open");
  $("#gamePanelBg").classList.add("show");

  // tab switching
  panel.querySelectorAll(".gp-tab").forEach(t => {
    t.onclick = () => {
      panel.querySelectorAll(".gp-tab").forEach(x => x.classList.remove("active"));
      panel.querySelectorAll(".gp-tab-content").forEach(x => x.classList.remove("active"));
      t.classList.add("active");
      const tab = $("#gpTab-" + t.dataset.tab); if (tab) tab.classList.add("active");
    };
  });

  if (g.exe) loadGameConfig(g.exe, i);
  else $("#gpTab-config").innerHTML = `<div class="gcfg-unsupported">${IC("warn")}<div>Executável não encontrado — adicione o caminho do .exe para habilitar.</div></div>`;
  loadNvidiaTab(g);
}

function closeGamePanel() {
  $("#gamePanel").classList.remove("open");
  $("#gamePanelBg").classList.remove("show");
}

async function loadNvidiaTab(g) {
  // usa GPU já carregada no state, senao busca do driver
  let gpus = (state.driver && state.driver.gpus) || [];
  if (!gpus.length) {
    try {
      const d = await api("/api/driver");
      state.driver = d;
      gpus = d.gpus || [];
    } catch {}
  }
  const tab = $("#gpTab-nvidia");
  if (!tab) return;
  const isNvidia = gpus.some(gpu => gpu.vendor === "NVIDIA");
  tab.innerHTML = renderPanelNvidia(isNvidia, gpus, g);
}

function renderPanelTweaks(g, i) {
  const tw = (kind, label, desc, on, enabled) => `
    <div class="gptw-row">
      <div class="gptw-info"><div class="gptw-label">${label}</div><div class="gptw-desc">${desc}</div></div>
      <div class="toggle ${on ? "on" : ""} ${enabled ? "" : "disabled"}" data-gtw="${i}" data-kind="${kind}"><i></i></div>
    </div>`;
  const noExe = !g.exe;
  let html = `<div class="gp-tweaks">
    ${tw("fso", "FSO off", "Reduz input lag e stutter em jogos DirectX", g.fso, !noExe)}
    ${tw("gpu", "GPU máximo desempenho", "Força alta performance no driver gráfico", g.gpu, !noExe)}
    ${tw("prio", "Prioridade de CPU alta", "Processo do jogo recebe mais fatias de CPU", g.prio, !noExe)}
    ${tw("av", "Excluir do antivírus", "Remove a pasta do scan em tempo real do Defender", g.av, !!g.pasta)}
    <button class="btn-hero gp-tudo-btn" data-gopt="${i}" ${noExe ? "disabled" : ""}>${IC("bolt")} Ativar tudo</button>
    ${noExe ? `<div class="gp-tweaks-note">${IC("warn")} Executável não localizado — abra o jogo e reescaneie.</div>` : ""}`;
  if (g.perfil) {
    const p = g.perfil;
    const dicas = (p.dicas || []).map(d => `<li>${escHtml(d)}</li>`).join("");
    html += `<div class="gp-profile-box"><div class="gp-profile-title">🎯 Perfil: ${escHtml(p.tipo)}</div><ol class="gp-pts">${dicas}</ol></div>`;
  }
  html += "</div>";
  return html;
}

function renderPanelNvidia(isNvidia, gpus, g) {
  if (!isNvidia) {
    const names = gpus.map(x => x.nome).join(", ") || "GPU não detectada";
    return `<div class="gnv-no-nvidia">${IC("warn")}<div>GPU detectada: <b>${escHtml(names)}</b>.<br>As configurações NVIDIA não estão disponíveis para esta GPU.</div></div>`;
  }
  const nvName = (gpus.find(x => x.vendor === "NVIDIA") || {}).nome || "NVIDIA";
  return `
    <div class="gnv-badge">${IC("bolt")}<div class="gnv-badge-text">GPU: <b>${escHtml(nvName)}</b></div></div>
    <div class="gnv-settings">
      <div class="gnv-row"><div class="gnv-info"><b>Modo de energia</b><span>Máximo desempenho — elimina clock adaptativo</span></div><span class="gnv-tag rec">Recomendado</span></div>
      <div class="gnv-row"><div class="gnv-info"><b>Low Latency Mode</b><span>Ultra — reduz input lag do driver</span></div><span class="gnv-tag rec">Recomendado</span></div>
      <div class="gnv-row"><div class="gnv-info"><b>Shader Cache</b><span>Ilimitado — evita gagueiros ao compilar shaders</span></div><span class="gnv-tag rec">Recomendado</span></div>
      <div class="gnv-row"><div class="gnv-info"><b>VSync</b><span>Desativado por aplicativo — controle pelo jogo</span></div><span class="gnv-tag info">Por aplicativo</span></div>
    </div>
    <button class="btn-hero gnv-open-btn" onclick="openNvidiaPanel()">Abrir NVIDIA Painel de Controle</button>
    <div class="gnv-note">Configure em "Gerenciar configurações 3D → Configurações do programa" e selecione o executável ${g.exe ? `(${escHtml(g.exe.split(/[/\\]/).pop())})` : "do jogo"}.</div>`;
}

async function loadGameConfig(exe, idx) {
  try {
    const r = await api("/api/jogos/config?exe=" + encodeURIComponent(exe));
    renderPanelConfig(r, exe, idx);
  } catch (e) {
    const tab = $("#gpTab-config");
    if (tab) tab.innerHTML = `<div class="gcfg-error">${IC("err")}<div>Erro ao ler config: ${escHtml(e.message)}</div></div>`;
  }
}

function renderPanelConfig(r, exe, idx) {
  const tab = $("#gpTab-config"); if (!tab) return;
  if (!r.supported) {
    tab.innerHTML = `<div class="gcfg-unsupported">${IC("gamepad")}
      <div><b>Config automática não disponível para este jogo</b><br><small>Jogos suportados:</small></div>
      <ul class="gcfg-supported-games">
        <li>VALORANT</li><li>Fortnite</li><li>PUBG</li><li>CS2</li>
        <li>Apex Legends</li><li>Rocket League</li><li>Rainbow Six Siege</li>
        <li>Battlefield 2042 / BF1</li><li>DayZ</li>
      </ul></div>`;
    return;
  }
  if (r.error) {
    tab.innerHTML = `<div class="gcfg-error">${IC("warn")}<div>${escHtml(r.error)}</div></div>`;
    return;
  }
  const vals = r.values || {};
  const fields = r.fields || [];
  const fileShort = r.config_file ? r.config_file.replace(/.*[/\\]([^/\\]+[/\\][^/\\]+)$/, "…/$1") : "";

  const fldHtml = fields.map(f => {
    // Tipo "res" — resolução (dois campos combinados)
    if (f.type === "res") {
      const keys = f.key.split(",");
      let curVal = "";
      if (keys.length >= 2) {
        const w = vals[keys[0]] || "", h = vals[keys[1]] || "";
        if (w && h) curVal = `${w}x${h}`;
      }
      const opts = f.options.map(o => `<option value="${escHtml(o.value)}"${o.value === curVal ? " selected" : ""}>${escHtml(o.label)}</option>`).join("");
      return `<div class="gcfg-field"><label for="gcfg-${escHtml(f.key)}">${escHtml(f.label)}</label>
        <select class="gcfg-select" id="gcfg-${escHtml(f.key)}" data-cfgkey="${escHtml(f.key)}" data-cfgtype="${f.type}">${opts}</select></div>`;
    }
    // Tipo "fps_ue" — float string (240.000000 → 240)
    if (f.type === "fps_ue") {
      const raw = vals[f.key] || "";
      const cur = raw ? String(Math.round(parseFloat(raw))) : "0";
      const opts = f.options.map(o => `<option value="${escHtml(o.value)}"${o.value === cur ? " selected" : ""}>${escHtml(o.label)}</option>`).join("");
      return `<div class="gcfg-field"><label for="gcfg-${escHtml(f.key)}">${escHtml(f.label)}</label>
        <select class="gcfg-select" id="gcfg-${escHtml(f.key)}" data-cfgkey="${escHtml(f.key)}" data-cfgtype="${f.type}">${opts}</select></div>`;
    }
    // Tipo "enum" ou "fps_int"
    const cur = vals[f.key] || "";
    const opts = f.options.map(o => `<option value="${escHtml(o.value)}"${o.value === cur ? " selected" : ""}>${escHtml(o.label)}</option>`).join("");
    return `<div class="gcfg-field"><label for="gcfg-${escHtml(f.key)}">${escHtml(f.label)}</label>
      <select class="gcfg-select" id="gcfg-${escHtml(f.key)}" data-cfgkey="${escHtml(f.key)}" data-cfgtype="${f.type}">${opts}</select></div>`;
  }).join("");

  tab.innerHTML = `
    <div class="gcfg-file" title="${escHtml(r.config_file || "")}">${escHtml(fileShort)}</div>
    <div class="gcfg-fields">${fldHtml}</div>
    <div class="gcfg-save-row">
      <button class="btn-hero" id="gcfgSaveBtn">Salvar config</button>
      <span class="gcfg-saved-msg" id="gcfgSavedMsg">Salvo!</span>
    </div>
    <div class="gcfg-warn">${IC("warn")} Feche o jogo antes de salvar — ele pode sobrescrever as mudanças ao abrir.</div>`;

  const btn = $("#gcfgSaveBtn"); if (!btn) return;
  btn.onclick = () => saveGameConfig(exe, r.config_file, fields);
}

async function saveGameConfig(exe, cfgFile, fields) {
  const btn = $("#gcfgSaveBtn"); if (btn) { btn.disabled = true; btn.textContent = "Salvando…"; }
  const values = {};
  const tab = $("#gpTab-config"); if (!tab) return;

  tab.querySelectorAll(".gcfg-select").forEach(sel => {
    const key = sel.dataset.cfgkey;
    const type = sel.dataset.cfgtype;
    const val = sel.value;
    if (!key || !val) return;

    if (type === "res") {
      const parts = val.split("x");
      if (parts.length === 2) {
        const keys = key.split(",");
        values[keys[0]] = parts[0];
        values[keys[1]] = parts[1];
        // UE LastUserConfirmed variants
        if (keys[0].startsWith("Resolution")) {
          values["LastUserConfirmedResolutionSizeX"] = parts[0];
          values["LastUserConfirmedResolutionSizeY"] = parts[1];
        } else if (keys[0] === "ResolutionSizeX") {
          values["LastUserConfirmedResolutionSizeX"] = parts[0];
          values["LastUserConfirmedResolutionSizeY"] = parts[1];
        }
      }
    } else if (type === "fps_ue") {
      const n = parseInt(val) || 0;
      values[key] = n === 0 ? "0.000000" : n.toFixed(6);
    } else {
      values[key] = val;
    }
  });

  try {
    await api("/api/jogos/config/set", { exe, values });
    const msg = $("#gcfgSavedMsg"); if (msg) { msg.classList.add("show"); setTimeout(() => msg.classList.remove("show"), 2000); }
    toast("ok", "Config salva", "Abra o jogo para aplicar as novas configurações.");
    // atualiza dots do card
    const g = state.jogos[state.activeGameIdx]; if (g) refreshGameCard(state.activeGameIdx);
  } catch (e) { toast("err", "Falha ao salvar", e.message); }
  finally { if (btn) { btn.disabled = false; btn.textContent = "Salvar config"; } }
}

function refreshGameCard(i) {
  const g = state.jogos[i]; if (!g) return;
  const card = document.querySelector(`.gcard[data-gidx="${i}"]`);
  if (!card) return;
  card.outerHTML = gameCard(g, i);
}

function openNvidiaPanel() {
  try { api("/api/jogos/tweak", { exe: "", pasta: "", fso: false, gpu: false, prio: false, av: false }).catch(() => {}); } catch {}
  // Abre o painel de controle NVIDIA via shell (best effort)
  fetch("/api/ferramentas/dns").catch(() => {}); // keep-alive no-op
  const msg = "Abra o Painel de Controle NVIDIA → Configurações 3D → Gerenciar configurações 3D → aba 'Configurações do programa'.";
  infoModal("NVIDIA Painel de Controle", `<p>${msg}</p>`);
}

function addCustomGame() {
  const exeInput = $("#customGameExe"), nameInput = $("#customGameName");
  if (!exeInput) return;
  const exe = exeInput.value.trim();
  if (!exe) return toast("warn", "Caminho vazio", "Cole o caminho do .exe do jogo.");
  if (!exe.toLowerCase().endsWith(".exe")) return toast("warn", "Arquivo inválido", "O caminho deve terminar em .exe.");
  const parts = exe.replace(/\\/g, "/").split("/");
  const filename = parts[parts.length - 1];
  const nome = (nameInput && nameInput.value.trim()) || filename.replace(/\.exe$/i, "");
  const pasta = exe.substring(0, exe.length - filename.length).replace(/[/\\]$/, "");
  if (state.customJogos.some(g => g.exe.toLowerCase() === exe.toLowerCase())) {
    return toast("info", "Jogo já adicionado", nome);
  }
  state.customJogos.push({ nome, loja: "Manual", pasta, exe, fso: false, gpu: false, prio: false, av: false });
  exeInput.value = ""; if (nameInput) nameInput.value = "";
  renderJogos();
  toast("ok", "Jogo adicionado", nome + " — configure os tweaks abaixo.");
}

function gameProfile(i) {
  const g = state.jogos[i]; if (!g || !g.perfil) return;
  const p = g.perfil;
  const dicas = (p.dicas || []).map((d) => `<li>${escHtml(d)}</li>`).join("");
  const html = `
    <p class="gp-sub">Configurações recomendadas para <b>${escHtml(p.nome)}</b> — aplique no <b>menu do próprio jogo</b>.</p>
    <ol class="gp-dicas">${dicas}</ol>
    ${p.tweaks ? `<div class="gp-box">${IC("bolt")}<div><b>Tweaks de sistema:</b> ${escHtml(p.tweaks)}</div></div>` : ""}
    ${p.notas ? `<div class="gp-box warn">${IC("warn")}<div>${escHtml(p.notas)}</div></div>` : ""}
    <div class="gp-act"><button class="btn-hero" data-gopt="${i}" ${g.exe ? "" : "disabled"}><span data-icon="bolt"></span> Aplicar tweaks seguros agora</button></div>`;
  infoModal(`Perfil · ${p.nome}`, html);
}
function gameSend(g) { return api("/api/jogos/tweak", { exe: g.exe, pasta: g.pasta, fso: g.fso, gpu: g.gpu, prio: g.prio, av: g.av }); }
async function gameToggle(i, kind, el) {
  const g = state.jogos[i]; if (!g) return;
  if (el.classList.contains("busy")) return; // ignora clique duplo enquanto aplica
  if (kind === "av" ? !g.pasta : !g.exe) return;
  const novo = !g[kind];
  el.classList.add("busy");
  try {
    const prev = g[kind]; g[kind] = novo;
    await gameSend(g);
    el.classList.toggle("on"); el.classList.remove("busy");
    const labels = { fso: ["FSO desativado", "FSO no padrão"], gpu: ["GPU alto desempenho", "GPU automática"], prio: ["Prioridade alta ativada", "Prioridade normal"], av: ["Excluído do antivírus", "Voltou ao antivírus"] };
    toast("ok", labels[kind][novo ? 0 : 1], g.nome);
    // atualiza dots do card no grid
    const card = document.querySelector(`.gcard[data-gidx="${i}"]`);
    if (card) { const dots = card.querySelectorAll(".gdot"); const order = ["fso","gpu","prio","av"]; order.forEach((k, di) => { if (dots[di]) dots[di].classList.toggle("on", !!g[k]); }); const active = order.filter(k => g[k]).length; const badge = card.querySelector(".gcard-badge"); if (active > 0) { if (!badge) { const coverEl = card.querySelector(".gcard-cover"); if (coverEl) { const b = document.createElement("div"); b.className = "gcard-badge"; b.textContent = active + " ativo" + (active > 1 ? "s" : ""); coverEl.appendChild(b); } } else { badge.textContent = active + " ativo" + (active > 1 ? "s" : ""); } } else if (badge) badge.remove(); }
    void prev;
  } catch (e) { g[kind] = !novo; toast("err", "Falha", e.message); el.classList.remove("busy"); }
}
async function gameOptimizeAll(i) {
  const g = state.jogos[i]; if (!g || !g.exe) return;
  busy(true, "Otimizando " + g.nome + "…");
  try {
    g.fso = g.gpu = g.prio = true; if (g.pasta) g.av = true;
    await gameSend(g);
    renderJogos();
    // atualiza painel se estiver aberto para este jogo
    if (state.activeGameIdx === i && $("#gamePanel").classList.contains("open")) {
      const tweaksTab = $("#gpTab-tweaks");
      if (tweaksTab) tweaksTab.innerHTML = renderPanelTweaks(g, i);
    }
    toast("ok", "Jogo otimizado", g.nome + " — FSO, GPU, prioridade e antivírus.");
  } catch (e) { toast("err", "Falha", e.message); } finally { busy(false); }
}

/* ---- Backup & Restauração ------------------------------------------------ */
function openBackup() {
  // Abre o modal IMEDIATAMENTE (com placeholder) e carrega a lista depois.
  // Antes a função esperava o /api/backup/lista responder ANTES de mostrar o
  // modal — se essa chamada travasse no PC do cliente, o backup "não abria".
  $("#mTitle").textContent = "Backup & Restauração";
  $("#mBody").innerHTML = `<p>Antes de otimizar, crie um <b>backup de segurança</b> do estado atual. Se algo der errado no PC do cliente, você restaura <b>tudo</b> num clique (registro, serviços, energia e inicialização).</p>
    <div class="bk-empty" id="bkList">Carregando backups…</div>`;
  $("#mFoot").innerHTML = `<button class="mbtn" data-close>Fechar</button><button class="mbtn primary" id="bkCreate">${IC("backup")} Criar backup agora</button>`;
  $("#modal").classList.add("show");
  const c = $("#bkCreate"); if (c) c.onclick = bkCreate;
  loadBackupList();
}
async function loadBackupList() {
  const box = $("#bkList"); if (!box) return;
  let list;
  try { list = await api("/api/backup/lista"); }
  catch (e) {
    box.outerHTML = `<div class="bk-empty" id="bkList">Não consegui carregar os backups existentes (${escHtml(e.message)}). Você ainda pode <b>criar um novo</b> abaixo.</div>`;
    return;
  }
  list = list || [];
  if (!list.length) {
    box.outerHTML = `<div class="bk-empty" id="bkList">Nenhum backup ainda. Crie o primeiro abaixo.</div>`;
    return;
  }
  box.outerHTML = `<div class="bk-list" id="bkList">${[...list].reverse().map((b) => `<div class="bk-item">
      <div class="bk-meta"><b>${escHtml(b.quando)}</b><span>${b.ajustes} ajustes · ${b.startup} inicialização</span></div>
      <div class="bk-act"><button class="abtn" data-bkrestore="${b.id}">Restaurar</button><button class="abtn guide" data-bkdel="${b.id}" title="Excluir">✕</button></div>
    </div>`).join("")}</div>`;
}
async function bkCreate() {
  busy(true, "Criando backup de segurança…");
  try { await api("/api/backup/criar", {}); toast("ok", "Backup criado", "Estado atual salvo com segurança."); openBackup(); }
  catch (e) { toast("err", "Falha", e.message); } finally { busy(false); }
}
function bkRestore(id) {
  confirmModal({
    title: "Restaurar este backup?", danger: true, okLabel: "Restaurar tudo",
    body: "Isto devolve o PC ao estado salvo neste backup — registro, serviços, energia e inicialização. As otimizações feitas depois serão revertidas.",
    onOk: async () => {
      busy(true, "Restaurando o sistema…");
      try {
        const d = await api("/api/backup/restaurar", { id }); afterMutation(d);
        const itens = d.itens || 0, falhas = d.falhas || 0;
        if (falhas && !itens) toast("err", "Nada foi restaurado", "Nenhum item voltou — rode como administrador e tente de novo.");
        else if (falhas) toast("warn", "Restauração parcial", `${itens} itens voltaram · ${falhas} falharam (precisa de admin?).`);
        else toast("ok", "Sistema restaurado", `${itens} itens devolvidos ao estado salvo.`);
      }
      catch (e) { toast("err", "Falha ao restaurar", e.message); } finally { busy(false); }
    },
  });
}
async function bkDelete(id) {
  try { await api("/api/backup/excluir", { id }); openBackup(); toast("info", "Backup excluído", ""); }
  catch (e) { toast("err", "Falha", e.message); }
}

async function openReport() {
  let hist = []; try { hist = await api("/api/historico"); } catch (e) {}
  const applied = []; (hist || []).forEach((b) => b.rules.forEach((r) => applied.push(r.titulo)));
  const scan = state.scan || {}, tot = scan.totais || {}, p = scan.perfil || {};
  const ram = (p.hardware && p.hardware.ram) || {}, disp = p.display || {}, ctx = p.contexto || {}, dns = (p.rede && p.rede.dns) || [];
  const discos = p.discos || [], sys = discos.find((d) => d.sistema) || discos[0] || {};
  const antes = state.scoreInicial != null ? state.scoreInicial : (scan.score || 0), depois = scan.score || 0;
  const cliente = ($("#reportClient").value || "").trim();
  const data = new Date().toLocaleString("pt-BR");
  const reco = (scan.regras || []).filter((r) => r.modo === "consultivo" && r.estado === "recomendado").map((r) => r.titulo);

  // FPS comprovado (antes × depois) — a prova de ganho
  const fps = state.fps, fpsB = state.fpsBase;
  let fpsBlock = "";
  if (fps) {
    const ganho = fpsB && fpsB.fps_avg ? Math.round((fps.fps_avg - fpsB.fps_avg) / fpsB.fps_avg * 100) : null;
    fpsBlock = `<h2>FPS no jogo — prova de ganho${fps.processo ? " (" + escHtml(fps.processo) + ")" : ""}</h2>
      <div class="r-score">
        ${fpsB ? `<div class="col"><span>FPS antes</span><b>${fpsB.fps_avg}</b></div><div class="arrow">→</div>` : ""}
        <div class="col"><span>FPS médio</span><b style="color:#1a9e6b">${fps.fps_avg}</b></div>
        <div class="col"><span>1% low</span><b>${fps.low1}</b></div>
        <div class="col"><span>0.1% low</span><b>${fps.low01}</b></div>
        ${ganho != null ? `<div class="col" style="margin-left:auto;text-align:right"><span>Ganho</span><b style="color:#1a9e6b">${ganho > 0 ? "+" : ""}${ganho}%</b></div>` : ""}
      </div>`;
  }

  // Benchmark (estresse CPU+GPU) e temperatura
  const b = state.bench, th = state.thermal;
  let perfBlock = "";
  if (b || th) {
    const cells = [];
    if (b) {
      cells.push(["Índice ThazzDraco", b.indice + (state.benchBase ? " (antes " + state.benchBase.indice + ")" : "")]);
      if (b.gpu) cells.push(["GPU (estresse)", b.gpu + " pts" + (b.gpu_peak_temp ? " · pico " + b.gpu_peak_temp + "°C" : "")]);
      cells.push(["CPU (todos os núcleos)", (b.cpu_multi || 0) + " Mops/s"]);
      cells.push(["Disco (escrita)", (b.disk_write || 0) + " MB/s"]);
    }
    if (th && th.temNVML) cells.push(["Temp. GPU sob carga", th.peakTemp + "°C" + (th.thTermico ? " — throttling térmico ⚠" : " — saudável")]);
    perfBlock = `<h2>Desempenho medido</h2><div class="r-grid">${cells.map(([k, v]) => `<div><span>${k}</span><span>${escHtml(String(v))}</span></div>`).join("")}</div>`;
  }

  $("#reportArea").innerHTML = `
    <div class="r-head"><img src="logo.png" alt=""><div><h1>Relatório de Otimização</h1><div class="r-sub">ThazzDraco PC FPS Boost${cliente ? " · Cliente: " + escHtml(cliente) : ""} · ${data}</div></div>${state.clientLogo ? `<img src="${state.clientLogo}" class="r-clientlogo" alt="">` : ""}</div>
    <div class="r-score"><div class="col"><span>Score antes</span><b style="color:${scoreHex(antes)}">${antes}</b></div><div class="arrow">→</div><div class="col"><span>Score depois</span><b style="color:${scoreHex(depois)}">${depois}</b></div>
      <div class="col" style="margin-left:auto;text-align:right"><span>Otimizações</span><b style="color:#1a9e6b">${applied.length}</b></div></div>
    ${fpsBlock}
    ${perfBlock}
    <h2>Computador</h2>
    <div class="r-grid">
      <div><span>Tipo</span><span>${ctx.tipo === "notebook" ? "Notebook" : "Desktop"}</span></div>
      <div><span>Memória</span><span>${ram.total_gb || "?"} GB ${ram.configured_mhz ? ram.configured_mhz + " MHz" : ""}</span></div>
      <div><span>Disco do sistema</span><span>${sys.tipo || "?"}</span></div>
      <div><span>Tela</span><span>${disp.refresh_atual_hz || "?"} Hz${disp.refresh_max_hz && disp.refresh_max_hz !== disp.refresh_atual_hz ? " (máx " + disp.refresh_max_hz + ")" : ""}</span></div>
      <div><span>DNS</span><span>${dns[0] || "—"}</span></div>
      <div><span>Energia</span><span>${ctx.na_bateria ? "Na bateria" : "Na tomada"}</span></div>
    </div>
    <h2>Otimizações aplicadas (${applied.length})</h2>
    ${applied.length ? `<ul>${applied.map((t) => `<li>${escHtml(t)}</li>`).join("")}</ul>` : '<p style="color:#7a8a9c;font-size:13px">Nenhuma aplicada nesta sessão ainda.</p>'}
    ${reco.length ? `<h2>Recomendações pendentes</h2><ul>${reco.map((t) => `<li>${escHtml(t)}</li>`).join("")}</ul>` : ""}
    <div class="r-foot"><span>ThazzDraco FPS Otimização &middot; @thazdracofpsotimizacao</span><span>Pendentes: ${tot.pendentes || 0} &middot; Oportunidades: ${tot.oportunidades || 0}</span></div>`;
  $("#reportOverlay").classList.add("show");
}

/* ---- Ações --------------------------------------------------------------- */
function isCleanup(r) { return r.id === CLEANUP_ID; }
// Varredura PROFUNDA: além das otimizações, puxa de verdade discos/S.M.A.R.T.,
// inicialização e jogos — cada etapa corresponde a trabalho real (não é teatro vazio).
const SCAN_STEPS = [
  "Inicializando varredura profunda", "Plano de energia & throttling da CPU", "Processador: clock & núcleos",
  "Modo de jogo & Game DVR", "Agendamento de GPU (HAGS) & MPO", "Perfil de memória (RAM/XMP)",
  "Latência de rede & DNS", "Serviços do Windows", "Telemetria & tarefas de fundo",
  "Registro do sistema", "Discos & saúde S.M.A.R.T.", "Arquivos temporários",
  "Programas de inicialização", "Jogos instalados (Steam/Epic)", "Cruzando otimizações disponíveis",
];
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
function setScanBar(p) { const b = $("#scanBar"); if (b) b.style.width = Math.round(p * 100) + "%"; }

async function doScan() {
  const ov = $("#scanOverlay");
  $("#scanSteps").innerHTML = ""; $("#scanResult").className = "scan-result"; $("#scanResult").innerHTML = "";
  $("#scanTitle").textContent = "Rastreando gargalos"; setScanBar(0);
  ov.classList.add("show");

  // coletas reais EM PARALELO (varredura genuinamente mais profunda)
  const scanP = api("/api/escanear").catch(() => null);
  const saudeP = api("/api/saude").catch(() => null);
  const startP = api("/api/inicializacao").catch(() => null);
  const jogosP = api("/api/jogos").catch(() => null);

  for (let i = 0; i < SCAN_STEPS.length; i++) {
    const el = document.createElement("div");
    el.className = "scan-step";
    el.innerHTML = `<span class="ss-ic">${IC("check")}</span><span>${SCAN_STEPS[i]}</span>`;
    $("#scanSteps").appendChild(el);
    await sleep(360 + Math.round(Math.random() * 180)); // ritmo "trabalhando", ~5-8s no total
    el.classList.add("done");
    setScanBar(((i + 1) / SCAN_STEPS.length) * 0.94);
  }

  const scan = await scanP;
  // espera os extras (discos/inicialização/jogos), mas no máximo ~2.5s a mais
  // (não trava em PC com biblioteca de jogos enorme).
  let saude = null, start = null, jogos = null;
  await Promise.race([
    Promise.all([saudeP, startP, jogosP]).then((a) => { saude = a[0]; start = a[1]; jogos = a[2]; }),
    sleep(2500),
  ]);
  setScanBar(1);
  if (scan) {
    ingest(scan);
    const t = scan.totais || {}, achadas = (t.pendentes || 0) + (t.oportunidades || 0);
    const nd = saude ? (saude.discos || []).length : 0;
    const na = start ? start.length : 0;
    const ng = jogos ? jogos.length : 0;
    $("#scanTitle").textContent = "Análise concluída";
    $("#scanResult").innerHTML = `<div class="sr-num">${achadas}</div><div class="sr-lbl">melhorias encontradas</div>
      <div class="sr-sub">Boost Score <b>${scan.score}</b> · ${nd} discos · ${na} apps de inicialização · ${ng} jogos</div>`;
    $("#scanResult").classList.add("show");
    await sleep(1900);
  } else {
    toast("err", "Falha na varredura", "Tente novamente.");
  }
  ov.classList.remove("show");
  const cta = $("#scanCta"); if (cta) cta.innerHTML = `<span data-icon="scan"></span> Reescanear`;
  const ic = $("#scanCta [data-icon]"); if (ic) ic.innerHTML = IC("scan");
}
function afterMutation(data) {
  const scoreBefore = state.scan ? state.scan.score : null;
  if (data.scan) ingest(data.scan);
  const rep = data.relatorio || {};
  if (rep.requer_reboot) $("#reboot").classList.add("show");
  const n = (rep.aplicadas || []).length;
  if (n) {
    pulseEnergy();
    if (scoreBefore !== null) {
      const scoreAfter = (state.scan && state.scan.score) || 0;
      if (scoreAfter > scoreBefore) showScoreResult(scoreBefore, scoreAfter, n);
    }
  }
}

// F5: exibe modal comparando score antes × depois da otimização
function showScoreResult(antes, depois, n) {
  const diff = depois - antes;
  const colAntes = scoreColor(antes), colDepois = scoreColor(depois);
  infoModal("Resultado da otimização", `
    <div class="score-result">
      <div class="sr-col">
        <span class="sr-lbl">Antes</span>
        <b class="sr-num" style="color:${colAntes}">${antes}</b>
      </div>
      <div class="sr-arrow">▲<span class="sr-diff">+${diff}</span></div>
      <div class="sr-col">
        <span class="sr-lbl">Depois</span>
        <b class="sr-num" style="color:${colDepois}">${depois}</b>
      </div>
    </div>
    <div class="sr-sub">${n} otimização${n > 1 ? "ões" : ""} aplicada${n > 1 ? "s" : ""} · score subiu ${diff} ${diff === 1 ? "ponto" : "pontos"}</div>
  `);
}
let _adminWarned = false;
function adminWarn() {
  const b = $("#noadmin"); if (b) b.classList.add("show");
  toast("err", "Precisa de administrador", "Feche e reabra o ThazzDraco como administrador.");
  if (_adminWarned) return;
  _adminWarned = true;
  infoModal("Rode como administrador", `
    <p>A maioria das otimizações mexe em <b>configurações do sistema</b> (registro do Windows, serviços, plano de energia) — e isso <b>exige privilégio de administrador</b>. Sem ele, elas falham com "acesso negado" e não ligam.</p>
    <p><b>Como resolver:</b> feche o ThazzDraco, clique com o <b>botão direito</b> no ícone e escolha <b>"Executar como administrador"</b> (aceite o aviso do Windows).</p>
    <p style="color:var(--ink-3);font-size:12.5px">As otimizações que mexem só no seu usuário (Game Mode, transparência, etc.) funcionam sem admin — por isso <b>algumas aplicam e outras não</b>.</p>`);
}

async function applyIds(ids, origem, msg) {
  if (!ids.length) return;
  const consent = ids.map((id) => state.byId[id]).filter((r) => r && (r.requer_consentimento || isCleanup(r)));
  const run = async (confirmar) => {
    busy(true, msg || "Aplicando…");
    try {
      const data = await api("/api/aplicar", { ids, confirmar });
      afterMutation(data);
      const rep = data.relatorio || {}, n = (rep.aplicadas || []).length;
      const erros = Object.values(rep.erros || {});
      const negado = erros.some((m) => /denied|negad|acesso/i.test(m || ""));
      if (rep.limpeza_mb) toast("ok", `Limpeza concluída`, `${fmtMB(rep.limpeza_mb)} liberados.`);
      else if (n) toast("ok", `${n} otimização${n > 1 ? "ões" : ""} aplicada${n > 1 ? "s" : ""}`, rep.restore_point ? "Ponto de restauração criado." : "");
      else if (!erros.length) toast("info", "Nada a aplicar", "Já estava aplicado ou não se aplica.");
      if (negado) { adminWarn(); }   // a maioria das otimizações precisa de administrador
      else if (erros.length) toast("err", "Alguns itens falharam", erros[0]);
    } catch (e) { toast("err", "Falha ao aplicar", e.message); } finally { busy(false); }
  };
  if (consent.length) confirmModal({ title: "Confirmar", body: consentBody(consent), okLabel: "Aplicar", danger: consent.some((r) => r.tier === "vermelho"), onOk: () => run(true) });
  else run(false);
}
function consentBody(rules) {
  const risky = rules.some((r) => r.tier === "vermelho"), clean = rules.some(isCleanup);
  const items = rules.map((r) => `<li><b>${r.titulo}</b> — ${r.descricao}</li>`).join("");
  return `<p>Confirma ${clean ? "a limpeza" : "estes ajustes"}?</p>
    <ul style="margin:12px 0 0 18px;display:flex;flex-direction:column;gap:8px">${items}</ul>
    ${risky ? `<div class="warn-box">${IC("warn")}<div>Tweaks avançados podem ter efeitos colaterais. Tudo reversível pelo Histórico.</div></div>` : ""}
    ${clean ? `<div class="warn-box" style="background:#0a2030;border-color:#1d4663;color:#bfe0f5">${IC("shield")}<div>Só apaga temporários/cache. Nunca toca em documentos. (A limpeza em si não tem desfazer.)</div></div>` : ""}`;
}
async function toggleRule(id, el) {
  const r = state.byId[id]; if (!r) return;
  if (el.disabled) return;
  el.disabled = true; el.classList.add("busy");
  try {
    if (r.estado === "aplicado") {
      busy(true, "Desfazendo…");
      try { afterMutation(await api("/api/desfazer", { rule_id: id })); toast("ok", "Desfeito", "Valor anterior restaurado."); }
      catch (e) { toast("err", "Falha ao desfazer", e.message); } finally { busy(false); }
    } else { await applyIds([id], "regra", "Aplicando…"); }
  } finally { el.disabled = false; el.classList.remove("busy"); }
}
async function applyGreens() {
  busy(true, "Otimizando (seguros)…");
  try { const d = await api("/api/aplicar-verdes", { confirmar: false }); afterMutation(d);
    const rep = d.relatorio || {}, n = (rep.aplicadas || []).length;
    const negado = Object.values(rep.erros || {}).some((m) => /denied|negad|acesso/i.test(m || ""));
    toast(n ? "ok" : "info", n ? `${n} otimização${n > 1 ? "ões" : ""} aplicada${n > 1 ? "s" : ""}` : "Nada novo aplicado", n ? "Ganhos seguros ativados." : "");
    if (negado) adminWarn();
  } catch (e) { toast("err", "Falha", e.message); } finally { busy(false); }
}
function optimizeSection() {
  const ids = sectionRules(state.cat).filter((r) => isToggleable(r) && r.aplicavel).map((r) => r.id);
  applyIds(ids, "selecao", "Otimizando seção…");
}
async function undo(opts, label) {
  busy(true, "Desfazendo…");
  try { afterMutation(await api("/api/desfazer", opts)); if (state.page === "historico") renderHistorico(); toast("ok", label || "Desfeito", "Restaurado."); }
  catch (e) { toast("err", "Falha ao desfazer", e.message); } finally { busy(false); }
}

/* ---- Modal --------------------------------------------------------------- */
function confirmModal({ title, body, okLabel, danger, onOk }) {
  $("#mTitle").textContent = title; $("#mBody").innerHTML = body;
  $("#mFoot").innerHTML = `<button class="mbtn" data-close>Cancelar</button><button class="mbtn ${danger ? "danger" : "primary"}" id="mOk">${okLabel || "Confirmar"}</button>`;
  $("#modal").classList.add("show");
  const mOk = $("#mOk");
  mOk.onclick = () => { if (mOk.disabled) return; mOk.disabled = true; closeModal(); onOk && onOk(); };
}
function infoModal(title, body) {
  $("#mTitle").textContent = title; $("#mBody").innerHTML = body;
  $("#mFoot").innerHTML = `<button class="mbtn primary" data-close>Entendi</button>`; $("#modal").classList.add("show");
}
function closeModal() { $("#modal").classList.remove("show"); }

/* ---- Eventos ------------------------------------------------------------- */
function wire() {
  $("#mainnav").onclick = (e) => { const a = e.target.closest("[data-page]"); if (a) showPage(a.dataset.page); };
  $("#subManut").onclick = (e) => { const b = e.target.closest("[data-sub]"); if (b) showSub("manutencao", b.dataset.sub); };
  $("#subMed").onclick = (e) => { const b = e.target.closest("[data-sub]"); if (b) showSub("medicao", b.dataset.sub); };
  $("#btnHist").onclick = () => showPage("historico");
  $("#btnRescan").onclick = () => doScan();
  // F5: limpar histórico de performance
  const btnPerfClear = $("#btnPerfClear");
  if (btnPerfClear) btnPerfClear.onclick = () => {
    try { localStorage.removeItem(PERF_KEY); } catch (e) {}
    renderPerfChart();
    toast("ok", "Histórico apagado", "");
  };

  // F9: Overlay minimalista
  const btnOvl = $("#btnOverlay");
  if (btnOvl) btnOvl.onclick = () => {
    window.open("/overlay.html", "draco_overlay", "width=280,height=200,resizable=no,menubar=no,toolbar=no,status=no,location=no");
  };
  $("#scanCta").onclick = () => doScan();
  $("#btnPerf").onclick = () => setPerf(!document.body.classList.contains("perf"));
  $("#btnReport").onclick = openReport;
  $("#btnBackup").onclick = openBackup;
  $("#reportClose").onclick = () => $("#reportOverlay").classList.remove("show");
  $("#reportPrint").onclick = () => window.print();
  $("#reportLogo").onchange = (e) => {
    const f = e.target.files && e.target.files[0]; if (!f) return;
    const rd = new FileReader();
    rd.onload = () => { state.clientLogo = rd.result; openReport(); };
    rd.readAsDataURL(f);
  };
  $("#btnReboot").onclick = () => confirmModal({ title: "Reiniciar agora?", body: "Salve seus trabalhos. O Windows vai reiniciar para aplicar os ajustes que pedem reinício.", okLabel: "Reiniciar", danger: true, onOk: () => api("/api/reiniciar", {}).catch(() => {}) });
  $("#btnOtimSecao").onclick = optimizeSection;
  $("#btnBench").onclick = runBench;
  $("#btnBenchBase").onclick = benchSetBase;
  $("#btnBenchClear").onclick = benchClearBase;
  $("#btnFps").onclick = runFps;
  $("#fpsRefresh").onclick = renderFpsGames;
  $("#btnFpsBase").onclick = fpsSetBase;
  $("#btnFpsClear").onclick = fpsClearBase;
  $("#fpsDurs").onclick = (e) => { const b = e.target.closest("[data-dur]"); if (!b) return; state.fpsDur = +b.dataset.dur; $$("#fpsDurs .fps-dchip").forEach((x) => x.classList.toggle("active", x === b)); };
  const addGameBtn = $("#btnAddGame"); if (addGameBtn) addGameBtn.onclick = addCustomGame;
  const exeIn = $("#customGameExe"); if (exeIn) exeIn.addEventListener("keydown", e => { if (e.key === "Enter") addCustomGame(); });

  // F14: exportar config de jogos
  const btnExp = $("#btnExportJogos");
  if (btnExp) btnExp.onclick = async () => {
    try {
      const jogos = await api("/api/jogos/exportar");
      const blob = new Blob([JSON.stringify({ versao: 1, jogos }, null, 2)], { type: "application/json" });
      const a = document.createElement("a"); a.href = URL.createObjectURL(blob);
      a.download = "thazzdraco-jogos.json"; a.click(); URL.revokeObjectURL(a.href);
      toast("ok", "Config exportada", `${jogos.length} jogo(s) salvo(s).`);
    } catch (e) { toast("err", "Falha ao exportar", e.message); }
  };
  // F14: importar config de jogos
  const inpImp = $("#inputImportJogos");
  if (inpImp) inpImp.onchange = async (e) => {
    const file = e.target.files[0]; if (!file) return;
    try {
      const text = await file.text(); const data = JSON.parse(text);
      const jogos = data.jogos || (Array.isArray(data) ? data : []);
      if (!jogos.length) return toast("warn", "Arquivo vazio", "Nenhum jogo encontrado.");
      busy(true, "Importando config…");
      const r = await api("/api/jogos/importar", { jogos });
      toast("ok", "Config importada", `${r.aplicados} aplicado(s) · ${r.ignorados} ignorado(s) (não instalado aqui).`);
      renderJogos();
    } catch (e) { toast("err", "Falha ao importar", e.message); }
    finally { busy(false); inpImp.value = ""; }
  };
  $("#btnDiag").onclick = runDiag;
  $("#btnRepair").onclick = runRepair;
  $("#btnBloat").onclick = removeBloat;
  $("#btnDeep").onclick = runDeepClean;
  $("#btnThermal").onclick = runThermal;
  $("#btnUndervolt").onclick = undervoltGuide;

  // F11: Scanner de Serviços
  const btnSvc = $("#btnSvcScan");
  if (btnSvc) btnSvc.onclick = runServiceScan;

  // F12: Auditoria de Drivers
  const btnDrv = $("#btnDriverAudit");
  if (btnDrv) btnDrv.onclick = runDriverAudit;

  // F1: Modo Game ao Vivo — toggle
  const lgbToggle = $("#lgbToggle");
  if (lgbToggle) lgbToggle.onclick = async () => {
    const isOn = lgbToggle.classList.contains("on");
    try {
      await api("/api/modo-game/set", { ativo: !isOn });
      lgbToggle.classList.toggle("on");
      updateLiveGameBar(!isOn, []);
      if (!isOn) startLiveGamePoll(); else stopLiveGamePoll();
    } catch (e) { toast("err", "Falha", e.message); }
  };
  $("#search").oninput = debounce((e) => { state.query = e.target.value.trim(); renderOtimRules(); }, 250);
  $("#btnUndoAll").onclick = () => confirmModal({ title: "Desfazer tudo?", body: "Reverte todas as otimizações aplicadas, restaurando os valores anteriores.", okLabel: "Desfazer tudo", danger: true, onOk: () => undo({ tudo: true }, "Tudo desfeito") });

  $("#subnav").onclick = (e) => { const a = e.target.closest("[data-cat]"); if (!a) return; state.cat = a.dataset.cat; state.query = ""; $("#search").value = ""; renderSubnav(); renderOtimRules(); };

  // delegação global p/ toggles, botões data-*
  document.body.addEventListener("click", (e) => {
    const tg = e.target.closest("[data-toggle]"); if (tg) return toggleRule(tg.dataset.toggle, tg);
    const su = e.target.closest("[data-startup]"); if (su) return startupToggle(+su.dataset.startup, su);
    const gc2 = e.target.closest(".gcard"); if (gc2 && !e.target.closest(".toggle") && !e.target.closest("button")) return openGamePanel(+gc2.dataset.gidx);
    const gw = e.target.closest("[data-gtw]"); if (gw) return gameToggle(+gw.dataset.gtw, gw.dataset.kind, gw);
    const gp = e.target.closest("[data-gprof]"); if (gp) return gameProfile(+gp.dataset.gprof);
    const go = e.target.closest("[data-gopt]"); if (go) return gameOptimizeAll(+go.dataset.gopt);
    const ap = e.target.closest("[data-apply]"); if (ap) return applyIds([ap.dataset.apply], "regra", "Aplicando…");
    const gd = e.target.closest("[data-guide]"); if (gd) { const r = state.byId[gd.dataset.guide]; if (!r) return; return infoModal(r.titulo, `<p>${r.descricao}</p><div class="warn-box" style="background:#0a2030;border-color:#1d4663;color:#bfe0f5">${IC("guide")}<div>${r.orientacao}</div></div>`); }
    const tc = e.target.closest("[data-tec]"); if (tc) { const r = state.byId[tc.dataset.tec]; if (r) return tecModal(r); }
    const bt = e.target.closest("[data-batch]"); if (bt) return undo({ batch_id: bt.dataset.batch }, "Lote desfeito");
    const gc = e.target.closest("[data-goclean]"); if (gc) return navTo("limpeza");
    const br = e.target.closest("[data-bkrestore]"); if (br) return bkRestore(br.dataset.bkrestore);
    const bdl = e.target.closest("[data-bkdel]"); if (bdl) return bkDelete(bdl.dataset.bkdel);
    const df = e.target.closest("[data-diagfix]"); if (df) return diagFix(df.dataset.diagfix);
    const dp = e.target.closest("[data-diagpage]"); if (dp) return navTo(dp.dataset.diagpage);
    const dc = e.target.closest("[data-drvclean]"); if (dc) return driverClean(dc.dataset.drvclean);
    const pr = e.target.closest("[data-preset]"); if (pr) return applyPreset(pr.dataset.preset);
    const tl = e.target.closest("[data-tool]"); if (tl) return runTool(tl.dataset.tool, tl);
  });

  $("#modal").onclick = (e) => { if (e.target.id === "modal" || e.target.hasAttribute("data-close")) closeModal(); };
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeModal(); });

  // B2: tooltip customizado — substitui title="" nativo (não funciona em touch/tablet)
  const _tip = document.createElement("div");
  _tip.id = "ctip";
  _tip.style.cssText = "position:fixed;z-index:9999;background:rgba(0,0,0,.82);color:#fff;font-size:12px;padding:5px 10px;border-radius:6px;pointer-events:none;opacity:0;transition:opacity .12s;max-width:260px;line-height:1.4;white-space:normal";
  document.body.appendChild(_tip);
  let _tipHide;
  document.addEventListener("mouseover", (e) => {
    const el = e.target.closest("[title],[data-tip]"); if (!el) return;
    const txt = el.getAttribute("title") || el.dataset.tip; if (!txt) return;
    if (el.hasAttribute("title")) { el.dataset.tip = txt; el.removeAttribute("title"); }
    clearTimeout(_tipHide); _tip.textContent = txt; _tip.style.opacity = "1";
    const r = el.getBoundingClientRect(), tw = _tip.offsetWidth || 200, th = _tip.offsetHeight || 28;
    _tip.style.left = Math.max(6, Math.min(r.left + r.width / 2 - tw / 2, innerWidth - tw - 6)) + "px";
    const ty = r.top - th - 8; _tip.style.top = (ty < 4 ? r.bottom + 6 : ty) + "px";
  });
  document.addEventListener("mouseout", () => { _tipHide = setTimeout(() => (_tip.style.opacity = "0"), 80); });
}

/* ---- GPU Panel ----------------------------------------------------------- */
async function renderGpu() {
  const liveBox = $("#gpuLive"), nvBox = $("#nvPanel"), rulesBox = $("#gpuRules");
  if (!liveBox) return;
  // 1. GPU ao vivo via /api/metricas
  try {
    const m = await api("/api/metricas");
    const gpus = m.gpus || [];
    liveBox.innerHTML = `<div class="bench-grid gpu-live-grid">${gpus.map(g => `<div class="bench-card">
      <div class="bench-h">${escHtml(g.nome || "GPU")}</div>
      <div class="bench-v">${g.uso_pct ?? "—"}<span>%</span></div>
      <div class="bench-hint">Temp ${g.temp_c != null ? g.temp_c + "°C" : "N/D"} · VRAM ${g.vram_usada_mb != null ? (Math.round(g.vram_usada_mb / 102.4) / 10).toFixed(1) + " GB" : "N/D"}</div>
    </div>`).join("") || `<div class="empty">${IC("gpu")}<div>Nenhuma GPU detectada nas métricas.</div></div>`}</div>`;
  } catch (e) { if (liveBox) liveBox.innerHTML = ""; }
  // 2. Painel NVIDIA
  if (!nvBox) return;
  try {
    const nv = await api("/api/gpu/painel");
    state.gpuPanel = nv;
    if (nv.disponivel) {
      const fanHtml = nv.fan_pct >= 0 ? nv.fan_pct + "%" : "N/D";
      nvBox.innerHTML = `<div class="ptitle" style="margin-top:16px">NVIDIA · ${escHtml(nv.nome)}</div>
        <div class="bench-grid gpu-nv-grid">
          <div class="bench-card"><div class="bench-h">Power Limit</div><div class="bench-v">${nv.power_limit_w}<span>W</span></div><div class="bench-hint">Máx ${nv.power_max_w}W · Padrão ${nv.power_default_w}W</div></div>
          <div class="bench-card"><div class="bench-h">Consumo atual</div><div class="bench-v">${nv.power_draw_w}<span>W</span></div><div class="bench-hint">P-State: ${nv.pstate}</div></div>
          <div class="bench-card"><div class="bench-h">Temperatura</div><div class="bench-v">${nv.temp_c}<span>°C</span></div><div class="bench-hint">Cooler: ${fanHtml}</div></div>
          <div class="bench-card"><div class="bench-h">Clock gráfico</div><div class="bench-v">${nv.clock_mhz}<span>MHz</span></div><div class="bench-hint">Máx ${nv.clock_max_mhz} MHz</div></div>
        </div>
        <div class="gpu-power-ctrl">
          <div class="ptitle" style="font-size:13px;margin-top:8px">Ajustar Power Limit</div>
          <div class="gpu-power-row">
            <input type="range" id="nvPowerSlider" min="${Math.max(50, Math.round(nv.power_default_w * 0.6))}" max="${nv.power_max_w}" value="${nv.power_limit_w}" step="5" class="gpu-slider">
            <span id="nvPowerVal" class="gpu-slider-val">${nv.power_limit_w} W</span>
            <button class="btnG" id="nvPowerSet">Aplicar</button>
            <button class="btnG" id="nvPowerReset">Padrão (${nv.power_default_w} W)</button>
          </div>
          <p style="color:var(--ink-3);font-size:12px;margin:8px 0 0">Reduzir diminui calor e ruído. Aumentar (boost) melhora FPS em jogos que saturem a GPU. Requer administrador.</p>
        </div>`;
      const sl = $("#nvPowerSlider"), vl = $("#nvPowerVal");
      if (sl && vl) sl.oninput = () => { vl.textContent = sl.value + " W"; };
      const setBtn = $("#nvPowerSet");
      if (setBtn) setBtn.onclick = async () => {
        const w = parseInt(sl.value, 10); busy(true, "Definindo power limit…");
        try { const r = await api("/api/gpu/poder", { watts: w, action: "set" }); toast(r.ok ? "ok" : "err", "Power Limit", r.mensagem || r.erro || ""); renderGpu(); }
        catch (e) { toast("err", "Falha", e.message); } finally { busy(false); }
      };
      const rstBtn = $("#nvPowerReset");
      if (rstBtn) rstBtn.onclick = async () => {
        busy(true, "Restaurando…");
        try { const r = await api("/api/gpu/poder", { action: "reset" }); toast(r.ok ? "ok" : "err", "Power Limit restaurado", r.mensagem || r.erro || ""); renderGpu(); }
        catch (e) { toast("err", "Falha", e.message); } finally { busy(false); }
      };
    } else {
      // Sem NVIDIA — tenta painel AMD
      try {
        const amd = await api("/api/gpu/amd");
        if (amd.disponivel) {
          const vram = amd.vram_mb >= 1024
            ? (amd.vram_mb / 1024).toFixed(1) + " GB"
            : amd.vram_mb > 0 ? amd.vram_mb + " MB" : "N/D";
          const res = (amd.resolucao_w && amd.resolucao_h)
            ? `${amd.resolucao_w}×${amd.resolucao_h}` : "—";
          nvBox.innerHTML = `<div class="ptitle" style="margin-top:16px">AMD · ${escHtml(amd.nome)}</div>
            <div class="bench-grid gpu-nv-grid">
              <div class="bench-card"><div class="bench-h">VRAM</div><div class="bench-v">${escHtml(vram)}</div><div class="bench-hint">Memória de vídeo dedicada</div></div>
              <div class="bench-card"><div class="bench-h">Driver</div><div class="bench-v" style="font-size:14px">${escHtml(amd.driver_version || "—")}</div><div class="bench-hint">Versão do driver AMD</div></div>
              <div class="bench-card"><div class="bench-h">Taxa de atualização</div><div class="bench-v">${amd.refresh_hz || "—"}<span>Hz</span></div><div class="bench-hint">Resolução: ${escHtml(res)}</div></div>
            </div>
            <div class="gpu-power-ctrl">
              <div class="ptitle" style="font-size:13px;margin-top:8px">AMD Adrenalin · configurações recomendadas para jogos</div>
              <div class="amd-guide">
                <div class="amd-tip"><b>Anti-Lag+</b> — Ativar: reduz latência de input (melhor que NVIDIA Reflex em alguns títulos).</div>
                <div class="amd-tip"><b>Radeon Chill</b> — Desativar: limita FPS dinamicamente; deixe desativado para máximo desempenho.</div>
                <div class="amd-tip"><b>Enhanced Sync</b> — Desativar: pode causar tela preta. Use FreeSync ou V-Sync do jogo em vez disso.</div>
                <div class="amd-tip"><b>Shader Cache</b> — Ativar: reduz gaguejadas de compilação ao carregar novas cenas.</div>
                <div class="amd-tip"><b>Image Sharpening</b> — Opcional: +10-15% de definição sem custo de FPS. Gosto pessoal.</div>
              </div>
              <p style="color:var(--ink-3);font-size:11px;margin:10px 0 0">Acesse: AMD Software · Adrenalin → Configurações (engrenagem) → Gráficos.</p>
            </div>`;
        } else {
          nvBox.innerHTML = `<div class="empty" style="padding:20px 0">${IC("gpu")}<div>Nenhuma GPU NVIDIA ou AMD dedicada detectada via driver.</div></div>`;
        }
      } catch (_) {
        nvBox.innerHTML = `<div class="empty" style="padding:20px 0">${IC("gpu")}<div>NVIDIA não detectada. GPU AMD: use o AMD Software · Adrenalin para ajustes.</div></div>`;
      }
    }
  } catch (e) { if (nvBox) nvBox.innerHTML = ""; }
  // 3. Regras de GPU do motor de regras
  if (rulesBox) {
    if (state.scan) {
      const gpuRules = state.rules.filter(r => /gpu|exib|display|tela|refr/i.test(r.categoria || ""));
      rulesBox.innerHTML = gpuRules.length
        ? gpuRules.map(ruleRow).join("")
        : `<div class="empty">${IC("ok")}<div>Nenhuma regra de GPU pendente. Execute a varredura para verificar.</div></div>`;
    } else {
      rulesBox.innerHTML = `<div class="empty">${IC("bolt")}<div>Execute a varredura no Núcleo para ver as regras de GPU.</div></div>`;
    }
  }
}

/* ---- Benchmark ----------------------------------------------------------- */
// Métricas medidas de verdade (CPU/memória/disco). Sem estimativa.
const BENCH_METRICS = [
  { key: "indice",     label: "Índice ThazzDraco", unit: "",       hint: "Composto (maior = melhor)", big: true },
  { key: "gpu",        label: "GPU (estresse)",    unit: "pts",    hint: "Throughput gráfico sob carga máxima (WebGL)" },
  { key: "cpu_single", label: "CPU · 1 núcleo",    unit: "Mops/s", hint: "Velocidade por núcleo (single-thread)" },
  { key: "cpu_multi",  label: "CPU · todos",       unit: "Mops/s", hint: "Soma de todos os núcleos lógicos" },
  { key: "mem_bw",     label: "Memória",           unit: "GB/s",   hint: "Banda de leitura/escrita da RAM" },
  { key: "disk_write", label: "Disco · escrita",   unit: "MB/s",   hint: "Escrita sequencial real (sincronizada)" },
];

// Índice composto (transparente) — agora inclui a GPU. Calculado no front porque
// o estresse de GPU roda no navegador (WebGL).
function benchIndice(b) {
  return Math.round(
    (b.cpu_single || 0) / 350 * 180 +
    (b.cpu_multi || 0) / 3000 * 260 +
    (b.mem_bw || 0) / 16 * 130 +
    (b.disk_write || 0) / 450 * 130 +
    (b.gpu || 0) / 16000 * 300);
}

function renderBench() {
  try { if (!state.benchBase) { const s = localStorage.getItem("tz_bench_base"); if (s) state.benchBase = JSON.parse(s); } } catch (e) {}
  const b = state.bench, base = state.benchBase;
  const grid = $("#benchGrid"); if (!grid) return;
  if (!b) {
    grid.innerHTML = `<div class="empty" style="padding:30px">${IC("gauge")}<div>Clique em <b>Rodar estresse</b> para martelar <b>CPU, GPU, memória e disco</b>.${base ? "<br><span style='color:var(--ink-3)'>Há uma referência \"antes\" salva — o ganho aparece após rodar.</span>" : ""}</div></div>`;
    return;
  }
  grid.innerHTML = BENCH_METRICS.map((m) => {
    const v = b[m.key] || 0;
    const bv = base ? (base[m.key] || 0) : null;
    let delta = "";
    if (bv != null && bv > 0) {
      const pct = Math.round((v - bv) / bv * 100);
      const cls = pct > 1 ? "up" : pct < -1 ? "down" : "flat";
      const arrow = pct > 1 ? "▲" : pct < -1 ? "▼" : "▬";
      delta = `<span class="bench-delta ${cls}">${arrow} ${pct > 0 ? "+" : ""}${pct}%<i>vs. antes ${bv}${m.unit}</i></span>`;
    }
    let hint = m.hint;
    if (m.key === "gpu" && b.gpu_peak_uso) hint = `Pico: ${b.gpu_peak_uso}% de uso${b.gpu_peak_temp ? ` · ${b.gpu_peak_temp}°C` : ""} sob carga`;
    else if (m.key === "gpu" && (b.gpu || 0) > 0 && !b.gpu_peak_uso) hint = `⚠ Uso 0% — provável acesso remoto (TeamViewer). CPU/memória/disco são reais; o número da GPU pode não refletir a placa física.`;
    return `<div class="bench-card${m.big ? " big" : ""}">
      <div class="bench-h">${m.label}</div>
      <div class="bench-v">${v}<span>${m.unit}</span></div>
      <div class="bench-hint">${hint}</div>
      ${delta}</div>`;
  }).join("");
}

const GPU_STRESS_SECS = 15;
async function runBench() {
  const btn = $("#btnBench"); if (btn) { btn.disabled = true; }
  const run = $("#benchRun");
  let pollGpu = null;
  try {
    // Fase 1 — CPU / memória / disco (backend, ~1s)
    if (run) run.innerHTML = `<div class="bench-running">${IC("ring")}<div>Estressando <b>CPU · memória · disco</b>… não use o PC.</div></div>`;
    const r = await api("/api/benchmark", {});

    // Fase 2 — GPU (WebGL real, ~15s) com telemetria ao vivo
    let peakUso = 0, peakTemp = 0, lastUso = 0, lastTemp = 0;
    pollGpu = setInterval(async () => {
      try { const m = await api("/api/metricas"); const g = (m.gpus || [])[0] || {};
        if (g.uso_pct >= 0) { lastUso = g.uso_pct; peakUso = Math.max(peakUso, g.uso_pct); }
        if (g.temp_c >= 0) { lastTemp = g.temp_c; peakTemp = Math.max(peakTemp, g.temp_c); }
      } catch (e) {}
    }, 1200);
    const draw = (el, tot) => {
      const pct = Math.round(el / GPU_STRESS_SECS * 100);
      const tTxt = lastTemp > 0 ? ` · <b>${lastTemp}°C</b>` : "";
      run.innerHTML = `<div class="fps-capturing"><div class="fps-cap-top">${IC("gauge")}<div><b>Estressando a GPU…</b> ${Math.ceil(GPU_STRESS_SECS - el)}s · GPU <b>${lastUso}%</b>${tTxt}</div></div>
        <div class="fps-prog"><i style="width:${pct}%"></i></div></div>`;
    };
    const gres = await gpuStress(GPU_STRESS_SECS, draw);
    clearInterval(pollGpu); pollGpu = null;

    if (gres.ok) { r.gpu = gres.score; r.gpu_pps = gres.pps; r.gpu_peak_uso = peakUso; r.gpu_peak_temp = peakTemp; }
    r.indice = benchIndice(r); // recalcula incluindo a GPU
    state.bench = r;
    const pk = peakTemp > 0 ? `${peakUso}% · ${peakTemp}°C` : `${peakUso}%`;
    const gpuTxt = gres.ok ? ` · GPU ${gres.score} pts (pico ${pk})` : ` · ${gres.motivo}`;
    if (run) run.innerHTML = `<div class="bench-done">${IC("ok")}<div>Estresse concluído · ${r.threads} threads${gpuTxt}.</div></div>`;
    renderBench();
  } catch (e) {
    if (run) run.innerHTML = `<div class="bench-done err">${IC("err")}<div>Falha ao rodar o benchmark.</div></div>`;
  } finally {
    if (pollGpu) clearInterval(pollGpu); // nunca deixa o poll de métricas órfão
    if (btn) btn.disabled = false;
  }
}

function benchSetBase() {
  if (!state.bench) return toast("warn", "Rode o benchmark primeiro", "Preciso de um resultado para guardar como \"antes\".");
  state.benchBase = state.bench;
  try { localStorage.setItem("tz_bench_base", JSON.stringify(state.bench)); } catch (e) {}
  toast("ok", "Referência salva", "Otimize ou feche apps e rode de novo para ver o ganho.");
  renderBench();
}
function benchClearBase() {
  state.benchBase = null;
  try { localStorage.removeItem("tz_bench_base"); } catch (e) {}
  toast("ok", "Referência limpa", "");
  renderBench();
}

/* ---- FPS real (PresentMon) ----------------------------------------------- */
const FPS_METRICS = [
  { key: "fps_avg", label: "FPS médio",  hint: "Quadros por segundo na média",        big: true,  better: "up" },
  { key: "low1",    label: "1% low",     hint: "Média dos piores 1% — a fluidez real", better: "up" },
  { key: "low01",   label: "0.1% low",   hint: "Piores 0.1% — engasgos mais severos",  better: "up" },
  { key: "fps_min", label: "Pior quadro", hint: "FPS do quadro mais lento",            better: "up" },
];

async function renderFpsGames() {
  if (state.fpsRefreshing) return;
  state.fpsRefreshing = true;
  const btn = $("#fpsRefresh");
  if (btn) { btn.disabled = true; btn.innerHTML = IC("ring"); }
  const sel = $("#fpsGame"); if (!sel) { state.fpsRefreshing = false; if (btn) { btn.disabled = false; btn.innerHTML = IC("refresh"); } return; }
  sel.innerHTML = `<option value="">Detectando jogos…</option>`;
  try {
    const list = await api("/api/fps/jogos");
    if (!list || !list.length) { sel.innerHTML = `<option value="">Nenhum jogo detectado — abra o jogo ou digite o .exe abaixo</option>`; return; }
    list.sort((a, b) => (b.foreground ? 1 : 0) - (a.foreground ? 1 : 0));
    sel.innerHTML = list.map((g) =>
      `<option value="${escHtml(g.exe)}">${escHtml(g.titulo)} — ${escHtml(g.exe)}${g.foreground ? " ✓ em foco" : ""}</option>`).join("");
  } catch (e) { sel.innerHTML = `<option value="">Falha ao listar — digite o .exe abaixo</option>`; }
  finally { state.fpsRefreshing = false; if (btn) { btn.disabled = false; btn.innerHTML = IC("refresh"); } }
}

// fpsGauge — velocímetro redesenhado: mais limpo, sem sobreposição do "antes"
function fpsGauge(r, base) {
  const cx = 200, cy = 190, RR = 148;
  const cur = Math.round(r.fps_avg || 0);
  const prev = base ? Math.round(base.fps_avg || 0) : null;
  const peak = Math.max(cur, prev || 0, 60);
  let MAX = Math.ceil(peak * 1.18 / 30) * 30; if (MAX < 90) MAX = 90;
  // converte FPS em ângulo (180° = arco de 0 a MAX, da esquerda para a direita)
  const ang = (f) => 180 - Math.min(f, MAX) / MAX * 180;
  const pt = (f, rr) => { const a = ang(f) * Math.PI / 180; return [cx - rr * Math.cos(a), cy - rr * Math.sin(a)]; };
  const arcPath = (f0, f1, rr) => {
    const p0 = pt(f0, rr), p1 = pt(f1, rr);
    const lg = ((f1 - f0) / MAX > 0.5) ? 1 : 0;
    return `M${p0[0].toFixed(1)} ${p0[1].toFixed(1)} A${rr} ${rr} 0 ${lg} 0 ${p1[0].toFixed(1)} ${p1[1].toFixed(1)}`;
  };
  // ticks e labels
  let ticks = "";
  const steps = MAX <= 120 ? 6 : MAX <= 240 ? 8 : 10;
  const step = MAX / steps;
  for (let i = 0; i <= steps; i++) {
    const f = i * step;
    const a = pt(f, RR + 10), b = pt(f, RR + 22);
    ticks += `<line x1="${a[0].toFixed(1)}" y1="${a[1].toFixed(1)}" x2="${b[0].toFixed(1)}" y2="${b[1].toFixed(1)}" stroke="#2a507a" stroke-width="2"/>`;
    const l = pt(f, RR + 34);
    ticks += `<text x="${l[0].toFixed(1)}" y="${(l[1] + 4).toFixed(1)}" fill="#4a6f90" font-size="10.5" text-anchor="middle">${Math.round(f)}</text>`;
  }
  // marcador "antes" — linha grossa laranja bem visível
  let antesMark = "";
  if (prev != null) {
    const a1 = pt(prev, RR - 18), a2 = pt(prev, RR + 6);
    antesMark = `<line x1="${a1[0].toFixed(1)}" y1="${a1[1].toFixed(1)}" x2="${a2[0].toFixed(1)}" y2="${a2[1].toFixed(1)}" stroke="#ff9040" stroke-width="3.5" stroke-linecap="round"/>`;
    const al = pt(prev, RR - 32);
    antesMark += `<text x="${al[0].toFixed(1)}" y="${(al[1] + 4).toFixed(1)}" fill="#ff9040" font-size="11" font-weight="700" text-anchor="middle">${prev}</text>`;
  }
  const needleAng = ang(cur);
  const needleTip = pt(cur, RR - 8);
  return `<div class="fps-gauge-wrap"><svg viewBox="0 0 400 220" width="100%" class="fps-gauge">
      <path d="M${(cx-RR).toFixed(0)} ${cy} A${RR} ${RR} 0 0 1 ${(cx+RR).toFixed(0)} ${cy}" fill="none" stroke="#0e1e2e" stroke-width="14" stroke-linecap="round"/>
      ${ticks}
      <path d="${arcPath(0, cur, RR)}" fill="none" stroke="url(#fg)" stroke-width="14" stroke-linecap="round"/>
      <defs><linearGradient id="fg" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#1a6fa8"/><stop offset="1" stop-color="#5fd2ff"/></linearGradient></defs>
      ${antesMark}
      <line x1="${cx}" y1="${cy}" x2="${needleTip[0].toFixed(1)}" y2="${needleTip[1].toFixed(1)}" id="fpsNeedle" stroke="#eaf6ff" stroke-width="3" stroke-linecap="round" data-end="${needleAng.toFixed(1)}"
        style="transform-box:view-box;transform-origin:${cx}px ${cy}px;transition:none"/>
      <circle cx="${cx}" cy="${cy}" r="10" fill="#0b1825" stroke="#2ea8e6" stroke-width="2.5"/>
      <text x="${cx}" y="${cy - 44}" fill="#eaf6ff" font-size="58" font-weight="600" text-anchor="middle" font-family="Bahnschrift,sans-serif">${cur}</text>
      <text x="${cx}" y="${cy - 20}" fill="#4a7a9b" font-size="11" letter-spacing="3" text-anchor="middle">FPS MÉDIO</text>
    </svg></div>`;
}

function renderFps() {
  try { if (!state.fpsBase) { const s = localStorage.getItem("tz_fps_base"); if (s) state.fpsBase = JSON.parse(s); } } catch (e) {}
  const box = $("#fpsResult"); if (!box) return;
  const r = state.fps, base = state.fpsBase;
  if (!r) { box.innerHTML = ""; return; }

  // Banner comparação antes×depois (quando existe referência)
  let compareBanner = "";
  if (base) {
    const cur = Math.round(r.fps_avg || 0), prev = Math.round(base.fps_avg || 0);
    const pct = prev > 0 ? Math.round((cur - prev) / prev * 100) : 0;
    const c = pct > 1 ? "var(--green)" : pct < -1 ? "var(--red)" : "var(--ink-2)";
    const sign = pct > 0 ? "+" : "";
    compareBanner = `<div class="fps-compare">
      <div class="fpc-col fpc-before"><span class="fpc-lbl">ANTES</span><span class="fpc-val">${prev}</span><span class="fpc-unit">fps</span></div>
      <div class="fpc-arrow">${IC("undo")}</div>
      <div class="fpc-col fpc-after"><span class="fpc-lbl">DEPOIS</span><span class="fpc-val" style="color:${c}">${cur}</span><span class="fpc-unit">fps</span></div>
      <div class="fpc-gain" style="color:${c}">${sign}${pct}%<span>GANHO</span></div>
    </div>`;
  }

  const cards = FPS_METRICS.filter((m) => m.key !== "fps_avg").map((m) => {
    const v = r[m.key] || 0, bv = base ? (base[m.key] || 0) : null;
    let delta = "";
    if (bv != null && bv > 0) {
      const pct = Math.round((v - bv) / bv * 100);
      const good = pct > 1, bad = pct < -1;
      const cls = good ? "up" : bad ? "down" : "flat", arrow = good ? "▲" : bad ? "▼" : "▬";
      delta = `<span class="bench-delta ${cls}">${arrow} ${pct > 0 ? "+" : ""}${pct}%<i>antes ${bv}</i></span>`;
    }
    return `<div class="bench-card">
      <div class="bench-h">${m.label}</div>
      <div class="bench-v">${v}<span>FPS</span></div>
      <div class="bench-hint">${m.hint}</div>${delta}</div>`;
  }).join("");
  const extra = `<div class="fps-extra">
    <span><i>Melhor</i>${r.fps_max} FPS</span>
    <span><i>Engasgo</i>${r.stutter_pct}%</span>
    <span><i>Quadros</i>${r.frames}</span>
    <span><i>Descartados</i>${r.dropped}</span>
    <span><i>Duração</i>${r.duracao_s}s</span></div>`;
  box.innerHTML = `${compareBanner}${fpsGauge(r, base)}<div class="bench-grid fps-grid">${cards}</div>${frametimeGraph(r.frametimes)}${extra}`;
  // anima agulha após render (precisa de 1 frame para o CSS pegar)
  const nd = box.querySelector("#fpsNeedle");
  if (nd && nd.dataset.end != null) {
    requestAnimationFrame(() => {
      nd.style.transition = "transform 1.2s cubic-bezier(.2,.7,.2,1)";
      nd.style.transform = `rotate(${+nd.dataset.end - 90}deg)`;
    });
  }
}

// frametimeGraph desenha a série de frametime (ms) como área SVG. Menor = melhor.
function frametimeGraph(ft) {
  if (!ft || ft.length < 2) return "";
  const W = 1000, H = 150, n = ft.length;
  const max = ft.reduce((a, v) => (v > a ? v : a), 1), pad = max * 0.12;
  const top = max + pad;
  const x = (i) => (i / (n - 1)) * W;
  const y = (v) => H - (v / top) * H;
  let d = `M0 ${y(ft[0]).toFixed(1)}`;
  for (let i = 1; i < n; i++) d += ` L${x(i).toFixed(1)} ${y(ft[i]).toFixed(1)}`;
  const area = d + ` L${W} ${H} L0 ${H} Z`;
  const avg = ft.reduce((a, b) => a + b, 0) / n;
  return `<div class="fps-graph"><div class="fps-graph-h">Frametime (ms) · ${ft.length} amostras · <b>menor e mais reto = melhor</b></div>
    <svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none">
      <defs><linearGradient id="ftg" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#2ea8e6" stop-opacity=".45"/><stop offset="1" stop-color="#2ea8e6" stop-opacity="0"/></linearGradient></defs>
      <path d="${area}" fill="url(#ftg)"/><path d="${d}" fill="none" stroke="#41c7ff" stroke-width="1.4"/>
      <line x1="0" y1="${y(avg).toFixed(1)}" x2="${W}" y2="${y(avg).toFixed(1)}" stroke="#35e0a0" stroke-width="1" stroke-dasharray="6 6" opacity=".7"/>
    </svg></div>`;
}

async function runFps() {
  const proc = ($("#fpsManual").value.trim() || $("#fpsGame").value || "").trim();
  if (!proc) return toast("warn", "Escolha o jogo", "Selecione um jogo na lista ou digite o .exe.");
  const btn = $("#btnFps"); if (btn) btn.disabled = true;
  const run = $("#fpsRun");
  try {
    const r = await api("/api/fps/iniciar", { processo: proc, segundos: state.fpsDur });
    if (!r.ok) { if (run) run.innerHTML = `<div class="bench-done err">${IC("err")}<div>${escHtml(r.erro || "Falha ao iniciar.")}</div></div>`; if (btn) btn.disabled = false; return; }
    pollFps();
  } catch (e) { if (run) run.innerHTML = `<div class="bench-done err">${IC("err")}<div>Falha ao iniciar a captura.</div></div>`; if (btn) btn.disabled = false; }
}

function pollFps() {
  const run = $("#fpsRun");
  const tick = async () => {
    let st; try { st = await api("/api/fps/status"); } catch (e) { return; }
    if (st.estado === "capturando") {
      if (st.processando) run.innerHTML = `<div class="bench-running">${IC("ring")}<div>Processando os quadros…</div></div>`;
      else {
        const rest = Math.ceil(st.restante_s || 0), pct = Math.round((1 - rest / (st.duracao_s || 1)) * 100);
        const gameInfo = (state.jogos || []).find(g => g.exe && st.processo &&
          g.exe.toLowerCase().endsWith(st.processo.toLowerCase()));
        let paramHtml = "";
        if (gameInfo) {
          const badges = [];
          if (gameInfo.fso)  badges.push(`<span class="fps-param fso">FSO off</span>`);
          if (gameInfo.gpu)  badges.push(`<span class="fps-param gpu">GPU máx</span>`);
          if (gameInfo.prio) badges.push(`<span class="fps-param prio">CPU alta</span>`);
          if (gameInfo.av)   badges.push(`<span class="fps-param av">AV excluído</span>`);
          if (gameInfo.perfil) badges.push(`<span class="fps-param prof">Perfil: ${escHtml(gameInfo.perfil.tipo)}</span>`);
          if (badges.length) paramHtml = `<div class="fps-cap-params">${badges.join("")}</div>`;
        }
        // B3: aviso se app está visível (usuário provavelmente não está no jogo)
        const focoWarn = !document.hidden
          ? `<div class="fps-focus-warn">${IC("warn")} Você está no ThazzDraco — volte para o jogo para a medição ser precisa.</div>` : "";
        run.innerHTML = `<div class="fps-capturing">
          <div class="fps-cap-top">${IC("gauge")}<div><b>Medindo ${escHtml(st.processo)}…</b> jogue normalmente — <b>${rest}s</b> restantes</div></div>
          ${paramHtml}${focoWarn}
          <div class="fps-prog"><i style="width:${pct}%"></i></div></div>`;
      }
      return;
    }
    clearInterval(state.fpsPoll); state.fpsPoll = null;
    const btn = $("#btnFps"); if (btn) btn.disabled = false;
    if (st.estado === "erro") { run.innerHTML = `<div class="bench-done err">${IC("err")}<div>${escHtml(st.erro || "Falha na captura.")}</div></div>`; return; }
    if (st.estado === "pronto" && st.resultado) {
      state.fps = st.resultado;
      run.innerHTML = `<div class="bench-done">${IC("ok")}<div>Medição concluída · ${st.resultado.frames} quadros em ${st.resultado.duracao_s}s.</div></div>`;
      renderFps();
    }
  };
  tick();
  state.fpsPoll = setInterval(tick, 500);
}

function fpsSetBase() {
  if (!state.fps) return toast("warn", "Meça primeiro", "Preciso de uma medição para guardar como \"antes\".");
  state.fpsBase = state.fps;
  try { localStorage.setItem("tz_fps_base", JSON.stringify(state.fps)); } catch (e) {}
  toast("ok", "Referência salva", "Otimize e meça de novo para ver o ganho de FPS.");
  renderFps();
}
function fpsClearBase() {
  state.fpsBase = null;
  try { localStorage.removeItem("tz_fps_base"); } catch (e) {}
  toast("ok", "Referência limpa", "");
  renderFps();
}

/* ---- Diagnóstico de gargalos --------------------------------------------- */
const SEV = {
  critico: { lbl: "Crítico", cls: "crit" },
  atencao: { lbl: "Atenção", cls: "warn" },
  info:    { lbl: "Info",    cls: "info" },
  bom:     { lbl: "OK",      cls: "good" },
};

async function runDiag() {
  const btn = $("#btnDiag"); if (btn) btn.disabled = true;
  const list = $("#diagList");
  if (list) list.innerHTML = `<div class="empty" style="padding:34px">${IC("ring")}<div>Analisando hardware e configuração…</div></div>`;
  try {
    const d = await api("/api/diagnostico");
    state.diag = d;
    renderDiag();
  } catch (e) {
    if (list) list.innerHTML = `<div class="empty">${IC("err")}<div>Falha ao diagnosticar.</div></div>`;
  } finally { if (btn) btn.disabled = false; }
  renderDriver(); // bloco do driver (independente)
}

/* ---- Driver de GPU ------------------------------------------------------- */
async function renderDriver() {
  const box = $("#diagDriver"); if (!box) return;
  let d; try { d = await api("/api/driver"); } catch (e) { box.innerHTML = ""; return; }
  state.driver = d;
  const gpus = d.gpus || [];
  if (!gpus.length) { box.innerHTML = ""; return; }
  const rows = gpus.map((g) => {
    const idade = g.idade_meses >= 0 ? (g.idade_meses < 1 ? "menos de 1 mês" : `~${g.idade_meses} ${g.idade_meses === 1 ? "mês" : "meses"}`) : "data N/D";
    const tag = g.dedicada ? `<span class="drv-tag">principal</span>` : `<span class="drv-tag sec">integrada</span>`;
    return `<div class="drv-row">
      <div class="drv-gpu">${escHtml(g.nome)} ${tag}</div>
      <div class="drv-meta"><span>Driver <b>${escHtml(g.versao || "?")}</b></span><span>${g.data || "—"}</span><span>${idade}</span></div></div>`;
  }).join("");
  const resid = d.residuos
    ? `<button class="btnG drv-clean" data-drvclean="${escHtml(d.residuos.pasta)}"><span data-icon="broom"></span> Limpar resíduos (${d.residuos.mb} MB)</button>`
    : "";
  const temNvidia = gpus.some((g) => g.vendor === "NVIDIA"), temAmd = gpus.some((g) => g.vendor === "AMD");
  const guiaBtn = (temNvidia || temAmd)
    ? `<button class="btnG drv-settings" id="btnGpuSettings"><span data-icon="bolt"></span> Ajustes do painel</button>` : "";
  box.innerHTML = `<div class="drv-card">
    <div class="drv-head">${IC("gpu")}<h3>Driver de GPU</h3>
      <button class="btnG drv-guide" id="btnDrvGuide"><span data-icon="guide"></span> Guia: instalar limpo</button>${guiaBtn}${resid}</div>
    ${rows}
    <div class="drv-note">Mostramos a versão e a data reais — sem alarme de "desatualizado". Atualize só se quiser; o guia explica como fazer <b>sem quebrar a tela</b>.</div>
  </div>`;
  const gb = $("#btnDrvGuide"); if (gb) gb.onclick = driverGuide;
  const gs = $("#btnGpuSettings"); if (gs) gs.onclick = () => gpuSettingsGuide(temNvidia, temAmd);
}

// Guia dos ajustes de painel NVIDIA/AMD (do Manual Mestre §12/§13). Honesto:
// orientamos — não dá para aplicar o painel do fabricante por API limpa.
function gpuSettingsGuide(nvidia, amd) {
  let html = `<p class="gp-sub">Os ajustes de maior impacto ficam no painel do fabricante — aplique lá. O de <b>maior efeito é o Modo de Baixa Latência</b>.</p>`;
  if (nvidia) html += `
    <p><b>Painel de Controle NVIDIA → Gerenciar configurações 3D (Globais):</b></p>
    <ol class="drv-steps">
      <li><b>Modo de baixa latência:</b> <b>Ultra</b> (competitivo) ou Ativado — o ajuste de maior impacto (menos input lag).</li>
      <li><b>Gerenciamento de energia:</b> Preferir desempenho máximo.</li>
      <li><b>Cache de sombreador:</b> 10 GB ou Ilimitado (menos stutter).</li>
      <li><b>V-Sync:</b> Desligado (deixe o G-SYNC + limite de FPS cuidar).</li>
      <li><b>Filtragem de textura - Qualidade:</b> Alto desempenho · <b>Threaded optimization:</b> Ativado.</li>
      <li><b>G-SYNC</b> ligado + limite de FPS <b>3 abaixo</b> da taxa do monitor (ex.: 141 em 144 Hz).</li>
    </ol>`;
  if (amd) html += `
    <p><b>AMD Adrenalin → Gaming → Graphics (Global):</b></p>
    <ol class="drv-steps">
      <li><b>Radeon Anti-Lag / Anti-Lag 2:</b> Ativado (reduz input lag).</li>
      <li><b>Wait for Vertical Refresh:</b> Sempre desativado (use FreeSync + cap).</li>
      <li><b>Image Sharpening:</b> Ativado (recupera nitidez).</li>
      <li><b>Surface Format Optimization:</b> Ativado (mais FPS, menos VRAM).</li>
      <li><b>FreeSync</b> ligado no monitor e no Adrenalin · <b>SAM/Resizable BAR</b> na BIOS (FPS grátis).</li>
      <li><b>HYPR-RX</b> liga vários recursos de uma vez (atalho).</li>
    </ol>`;
  html += `<div class="gp-box">${IC("bolt")}<div><b>Já fazemos por aqui:</b> HAGS, cache de shader (Limpeza profunda), otimização por jogo e o teste térmico. Estes ajustes do painel complementam.</div></div>`;
  infoModal("Ajustes de painel da GPU", html);
}

function driverGuide() {
  const html = `
    <div class="warn-box" style="background:#2a1110;border-color:#5c2420;color:#ffb9ad">${IC("warn")}<div>
      <b>Atenção (TeamViewer/acesso remoto):</b> remover o driver de vídeo <b>derruba a tela</b> e pode
      <b>cortar o acesso remoto</b>. Faça isto só com um <b>monitor físico</b> na máquina, nunca apenas por TeamViewer.</div></div>
    <p><b>Instalação limpa de driver (estilo DDU) — passo a passo:</b></p>
    <ol class="drv-steps">
      <li><b>Baixe o driver mais novo</b> da sua placa (no PC do cliente, antes de remover):<br>
        <code>nvidia.com/Download</code> · <code>amd.com/support</code> · <code>intel.com/content/www/us/en/download-center</code></li>
      <li><b>Baixe o DDU</b> (Display Driver Uninstaller): <code>wagnardsoft.com</code> — é o padrão da indústria.</li>
      <li>Reinicie em <b>Modo de Segurança</b> (Win+R → <code>msconfig</code> → Inicialização de sistema → Inicialização segura).</li>
      <li>Rode o <b>DDU</b> → "Limpar e reiniciar" para a sua marca (remove tudo: resíduos, perfis, registro).</li>
      <li>Ao voltar, <b>instale o driver</b> que você baixou. Na NVIDIA, escolha <b>"Instalação limpa"</b>; recomendado o pacote <b>sem GeForce Experience</b> para menos serviços em segundo plano.</li>
      <li>Reabra o ThazzDraco e rode o <b>Benchmark</b> e o <b>FPS</b> (antes × depois) para comprovar o ganho.</li>
    </ol>
    <p style="color:var(--ink-3);font-size:12.5px">Dica: depois de instalar o driver bom, dá pra impedir o Windows de sobrescrevê-lo — peça que eu adiciono esse ajuste reversível.</p>`;
  infoModal("Instalar driver de GPU limpo", html);
}

async function driverClean(pasta) {
  confirmModal({
    title: "Limpar resíduos de driver?",
    body: `<p>Apaga a pasta de <b>extração do instalador</b> (<code>${escHtml(pasta)}</code>) — é lixo de instalação, recriado no próximo install. <b>Não toca</b> no driver ativo.</p>`,
    okLabel: "Limpar",
    onOk: async () => {
      busy(true, "Limpando resíduos…");
      try { const r = await api("/api/driver/limpar", { pasta }); if (r.ok) toast("ok", "Resíduos removidos", ""); else toast("err", "Falha", r.erro || ""); }
      catch (e) { toast("err", "Falha ao limpar", e.message); }
      finally { busy(false); renderDriver(); }
    },
  });
}

function renderDiag() {
  const d = state.diag; if (!d) return;
  const r = d.resumo || {};
  $("#diagSummary").innerHTML =
    `<span class="diag-pill crit">${r.criticos || 0} críticos</span>
     <span class="diag-pill warn">${r.atencoes || 0} atenções</span>
     <span class="diag-pill good">${r.bons || 0} OK</span>`;
  $("#diagList").innerHTML = (d.gargalos || []).map((g) => {
    const s = SEV[g.severidade] || SEV.info;
    let action = "";
    if (g.acao && g.acao.startsWith("regra:")) action = `<button class="btnG diag-fix" data-diagfix="${g.acao.slice(6)}"><span data-icon="bolt"></span> Corrigir agora</button>`;
    else if (g.acao && g.acao.startsWith("pagina:")) { const pg = g.acao.slice(7), lbl = { limpeza: "Limpeza", inicializacao: "Inicialização", jogos: "Jogos", desempenho: "Desempenho" }[pg] || pg; action = `<button class="btnG diag-go" data-diagpage="${pg}">Abrir ${escHtml(lbl)}</button>`; }
    const corr = g.severidade === "bom" ? "" :
      `<div class="diag-fixbox">${IC(g.acao === "manual" ? "guide" : "ok")}<div><b>Como resolver:</b> ${escHtml(g.correcao)}</div></div>`;
    const imp = g.impacto && g.severidade !== "bom" ? `<p class="diag-imp">${escHtml(g.impacto)}</p>` : "";
    return `<div class="diag-card ${s.cls}">
      <div class="diag-top">
        <span class="diag-badge ${s.cls}">${s.lbl}</span>
        <h3>${escHtml(g.titulo)}</h3>
        ${action}
      </div>
      <div class="diag-detect">${escHtml(g.detectado)}</div>
      ${imp}${corr}</div>`;
  }).join("");
}

async function diagFix(ruleId) {
  await applyIds([ruleId], "diagnostico", "Corrigindo…");
  runDiag(); // re-mede para refletir a correção
}

/* ---- Reparo do Windows --------------------------------------------------- */
async function runRepair() {
  const etapas = $$("#repairSteps input:checked").map((c) => c.value);
  if (!etapas.length) return toast("warn", "Selecione uma etapa", "Marque ao menos um reparo.");
  confirmModal({
    title: "Iniciar reparo do Windows?",
    body: `<p>Vai rodar <b>${etapas.length} etapa(s)</b> de reparo do Windows. <b>Não apaga dados</b>, mas pode levar de 5 a 30 min — evite desligar o PC. Recomendado ter <b>backup</b> antes.</p>`,
    okLabel: "Iniciar reparo",
    onOk: async () => {
      const btn = $("#btnRepair"); if (btn) btn.disabled = true;
      try {
        const r = await api("/api/reparo/iniciar", { etapas });
        if (!r.ok) { toast("err", "Falha ao iniciar", r.erro || ""); if (btn) btn.disabled = false; return; }
        pollRepair();
      } catch (e) { toast("err", "Falha ao iniciar", e.message); if (btn) btn.disabled = false; }
    },
  });
}

function pollRepair() {
  if (state.repairPoll) return;
  const box = $("#repairRun");
  const tick = async () => {
    let st; try { st = await api("/api/reparo/status"); } catch (e) { return; }
    const done = st.estado === "concluido" || st.estado === "erro";
    // B5: mensagem de erro mostra qual etapa falhou
    const head = st.estado === "rodando"
      ? `<b>${escHtml(st.etapa || "Reparando…")}</b> · etapa ${(+st.etapa_idx + 1)}/${st.etapas_tot} · ${st.pct}%`
      : st.estado === "erro"
        ? `<b style="color:var(--red)">Falhou</b> na etapa <b>${escHtml(st.etapa || "desconhecida")}</b> — veja o log abaixo`
        : `<b style="color:var(--green)">Reparo concluído</b>`;
    const logHtml = (st.log || []).map((l) => `<div class="rl${l.startsWith("===") ? " sec" : ""}">${escHtml(l)}</div>`).join("");
    box.innerHTML = `<div class="repair-live">
      <div class="rep-head">${st.estado === "rodando" ? IC("ring") : IC(st.estado === "erro" ? "err" : "ok")}<div>${head}</div></div>
      <div class="rep-prog"><i style="width:${st.estado === "rodando" ? st.pct : 100}%"></i></div>
      <div class="rep-console" id="repConsole">${logHtml || '<div class="rl">Iniciando…</div>'}</div></div>`;
    const c = $("#repConsole"); if (c) c.scrollTop = c.scrollHeight;
    if (done) {
      clearInterval(state.repairPoll); state.repairPoll = null;
      const btn = $("#btnRepair"); if (btn) btn.disabled = false;
      if (st.estado === "concluido") toast("ok", "Reparo concluído", "Reinicie o PC para efeito pleno.");
      else if (st.estado === "erro") toast("err", `Falhou: ${st.etapa || "etapa desconhecida"}`, "Veja o log para detalhes.");
    }
  };
  tick();
  state.repairPoll = setInterval(tick, 1500);
}

/* ---- Saúde térmica (teste de estresse) ----------------------------------- */
const THERMAL_SECS = 25;
async function runThermal() {
  const btn = $("#btnThermal"); if (btn) btn.disabled = true;
  const res = $("#thermalResult");
  let peakTemp = 0, peakUso = 0, lastTemp = 0, temNVML = false, thTermico = false, thPot = false;
  const poll = setInterval(async () => {
    try {
      const m = await api("/api/metricas"); const g = (m.gpus || [])[0] || {};
      if (g.temp_c >= 0) { temNVML = true; lastTemp = g.temp_c; peakTemp = Math.max(peakTemp, g.temp_c); }
      if (g.uso_pct >= 0) peakUso = Math.max(peakUso, g.uso_pct);
      if (g.throttle === "termico") thTermico = true;
      if (g.throttle === "potencia") thPot = true;
    } catch (e) {}
  }, 1200);
  const draw = (el) => {
    const left = Math.ceil(THERMAL_SECS - el), info = temNVML ? `<b>${lastTemp}°C</b>` : `GPU <b>${peakUso}%</b>`;
    res.innerHTML = `<div class="fps-capturing"><div class="fps-cap-top">${IC("gauge")}<div><b>Estressando a GPU…</b> ${left}s · ${info}</div></div>
      <div class="fps-prog"><i style="width:${Math.round(el / THERMAL_SECS * 100)}%"></i></div></div>`;
  };
  try {
    await gpuStress(THERMAL_SECS, draw);
    state.thermal = { temNVML, peakTemp, peakUso, thTermico, thPot };
    renderThermalVerdict(state.thermal);
  } catch (e) {
    res.innerHTML = `<div class="bench-done err">${IC("err")}<div>Falha no teste térmico.</div></div>`;
  } finally {
    clearInterval(poll); // nunca deixa o poll de métricas órfão
    if (btn) btn.disabled = false;
  }
}

function renderThermalVerdict(d) {
  const res = $("#thermalResult"); if (!res) return;
  let nivel, titulo, conselho;
  if (!d.temNVML) {
    nivel = "info"; titulo = "Temperatura indisponível nesta GPU";
    conselho = `Medimos o uso (pico ${d.peakUso}%), mas a temperatura precisa do sensor do fabricante (NVIDIA via NVML). Em AMD/Intel, use o HWiNFO para acompanhar a temperatura ao vivo.`;
  } else if (d.thTermico || d.peakTemp >= 84) {
    nivel = "crit"; titulo = `Throttling térmico — pico ${d.peakTemp}°C`;
    conselho = "A GPU está reduzindo o clock por calor — isso CUSTA FPS. Ações: limpe a poeira dos coolers, confira se os fans giram, melhore o fluxo de ar do gabinete e considere refazer a pasta térmica. Um undervolt também derruba muito a temperatura.";
  } else if (d.peakTemp >= 78) {
    nivel = "warn"; titulo = `Esquentando — pico ${d.peakTemp}°C`;
    conselho = "Está no limite saudável, sem throttling. De olho na ventilação; limpeza de poeira, melhor airflow ou um undervolt leve dão mais folga.";
  } else {
    nivel = "good"; titulo = `Refrigeração saudável — pico ${d.peakTemp}°C`;
    conselho = `Pico de ${d.peakTemp}°C sob carga máxima, sem throttling térmico. Está ótimo — nada a fazer.`;
  }
  const pot = d.thPot ? `<div class="diag-fixbox">${IC("bolt")}<div><b>Limite de potência:</b> a GPU bateu no teto de energia (power limit do modelo). É normal sob carga máxima.</div></div>` : "";
  const badge = { good: "OK", crit: "Crítico", warn: "Atenção", info: "Info" }[nivel];
  res.innerHTML = `<div class="thermal-card ${nivel}">
    <div class="th-head"><span class="diag-badge ${nivel}">${badge}</span><h3>${escHtml(titulo)}</h3></div>
    <div class="th-meta">Pico: <b>${d.temNVML ? d.peakTemp + "°C" : "—"}</b> · uso ${d.peakUso}% · ${d.thTermico ? "throttlou térmico" : "sem throttling térmico"}</div>
    <p class="diag-imp">${escHtml(conselho)}</p>${pot}</div>`;
}

function undervoltGuide() {
  const html = `
    <p class="gp-sub">Undervolt = mesma performance com <b>menos voltagem</b> → menos calor, menos ruído e FPS mais estável (menos throttling). Tudo <b>reversível</b>.</p>
    <p><b>GPU (NVIDIA/AMD) — MSI Afterburner:</b></p>
    <ol class="drv-steps">
      <li>Instale o <code>MSI Afterburner</code>.</li>
      <li>Abra a curva de voltagem/clock (<code>Ctrl+F</code>).</li>
      <li>Num ponto de voltagem mais baixo (ex.: 875–950 mV), suba o clock até o valor normal e <b>achate a curva à direita</b> (Shift+arrastar).</li>
      <li>Aplique e <b>teste estabilidade</b> (rode o Benchmark + jogos ~30 min). Travou? Suba um pouco a voltagem.</li>
    </ol>
    <p><b>CPU AMD (Ryzen):</b> BIOS → PBO → <b>Curve Optimizer</b> negativo (-15 a -30 por núcleo) ou pelo Ryzen Master.</p>
    <p><b>CPU Intel:</b> Intel XTU ou BIOS → offset de voltagem do core (-50 a -120 mV).</p>
    <div class="gp-box warn">${IC("warn")}<div>Undervolt agressivo demais trava o PC — vá com calma e teste a estabilidade. Tudo volta ao padrão se precisar.</div></div>`;
  infoModal("Guia de undervolt", html);
}

/* ---- Limpeza profunda ---------------------------------------------------- */
function fmtCleanMB(mb) { return mb >= 1024 ? (mb / 1024).toFixed(1) + " GB" : mb + " MB"; }
async function renderDeepClean() {
  const box = $("#deepBox"); if (!box) return;
  box.innerHTML = `<div class="empty" style="padding:24px">${IC("ring")}<div>Calculando o que dá para limpar…</div></div>`;
  let d; try { d = await api("/api/limpeza-profunda/scan"); } catch (e) { box.innerHTML = `<div class="empty">${IC("err")}<div>Falha ao escanear.</div></div>`; return; }
  state.deep = d.categorias || [];
  if (!state.deep.length) { box.innerHTML = `<div class="empty">${IC("ok")}<div>Nada relevante para limpeza profunda. 👍</div></div>`; $("#deepActions").style.display = "none"; return; }
  box.innerHTML = state.deep.map((c, i) => `
    <label class="deep-row${c.aviso ? " warn" : ""}">
      <input type="checkbox" data-deep="${i}" ${c.recomendado ? "checked" : ""}>
      <div class="dc-body"><div class="dc-top"><b>${escHtml(c.nome)}</b><span class="dc-mb">${fmtCleanMB(c.mb)}</span></div>
        <span class="dc-desc">${escHtml(c.descricao)}</span>
        ${c.aviso ? `<span class="dc-aviso">${IC("warn")} ${escHtml(c.aviso)}</span>` : ""}</div>
    </label>`).join("");
  $$("#deepBox [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
  $("#deepActions").style.display = "";
  box.onchange = updateDeepCount; updateDeepCount();
}
function updateDeepCount() {
  const sel = $$("#deepBox input:checked").map((c) => state.deep[+c.dataset.deep]).filter(Boolean);
  const mb = sel.reduce((s, c) => s + c.mb, 0);
  const b = $("#btnDeep"); if (b) b.innerHTML = `${IC("broom")} Limpar selecionados (${fmtCleanMB(mb)})`;
}
function runDeepClean() {
  const sel = $$("#deepBox input:checked").map((c) => state.deep[+c.dataset.deep]).filter(Boolean);
  if (!sel.length) return toast("warn", "Nada selecionado", "Marque o que quer limpar.");
  const temAviso = sel.some((c) => c.aviso);
  confirmModal({
    title: `Limpar ${sel.length} categoria(s)?`,
    body: `<p>Vai liberar até <b>${fmtCleanMB(sel.reduce((s, c) => s + c.mb, 0))}</b>. Tudo é <b>regenerável</b> — nada de documentos, saves ou dados pessoais.</p>${temAviso ? `<div class="warn-box" style="background:#2a1110;border-color:#5c2420;color:#ffb9ad">${IC("warn")}<div>Você marcou itens sensíveis (Lixeira e/ou Windows.old). Confirme que pode apagar de vez.</div></div>` : ""}`,
    okLabel: "Limpar agora", danger: temAviso,
    onOk: async () => {
      busy(true, "Limpando…");
      try {
        const r = await api("/api/limpeza-profunda/limpar", { ids: sel.map((c) => c.id), confirmar: true });
        toast("ok", "Limpeza concluída", `${fmtCleanMB(r.liberado_mb || 0)} liberados.`);
        renderDeepClean();
      } catch (e) { toast("err", "Falha ao limpar", e.message); } finally { busy(false); }
    },
  });
}

/* ---- Debloat (remover bloatware) ----------------------------------------- */
async function renderBloat() {
  const box = $("#bloatBox"); if (!box) return;
  box.innerHTML = `<div class="empty" style="padding:24px">${IC("ring")}<div>Lendo apps instalados…</div></div>`;
  let d; try { d = await api("/api/debloat/lista"); } catch (e) { box.innerHTML = `<div class="empty">${IC("err")}<div>Falha ao ler apps.</div></div>`; return; }
  state.bloat = d.apps || [];
  if (!state.bloat.length) { box.innerHTML = `<div class="empty">${IC("ok")}<div>Nenhum bloatware conhecido instalado. 👍</div></div>`; $("#bloatActions").style.display = "none"; return; }
  const byCat = {};
  state.bloat.forEach((a, i) => { (byCat[a.categoria] = byCat[a.categoria] || []).push({ a, i }); });
  box.innerHTML = Object.entries(byCat).map(([cat, items]) => `
    <div class="bloat-cat"><div class="bloat-cat-h">${escHtml(cat)}</div>
      ${items.map(({ a, i }) => `<label class="bloat-row">
        <input type="checkbox" data-bloat="${i}" ${a.recomendado ? "checked" : ""}>
        <span class="bl-nome">${escHtml(a.nome)}</span>
        ${a.nota ? `<span class="bl-nota" title="${escHtml(a.nota)}">${IC("warn")}</span>` : ""}
      </label>`).join("")}
    </div>`).join("");
  $$("#bloatBox [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
  $("#bloatActions").style.display = "";
  updateBloatCount();
  box.onchange = updateBloatCount;
}
function updateBloatCount() {
  const n = $$("#bloatBox input:checked").length;
  const b = $("#btnBloat"); if (b) b.innerHTML = `${IC("broom")} Remover ${n} selecionado${n === 1 ? "" : "s"}`;
}
function removeBloat() {
  const sel = $$("#bloatBox input:checked").map((c) => state.bloat[+c.dataset.bloat]).filter(Boolean);
  if (!sel.length) return toast("warn", "Nada selecionado", "Marque os apps que quer remover.");
  const nomes = sel.slice(0, 8).map((a) => a.nome).join(", ") + (sel.length > 8 ? `… (+${sel.length - 8})` : "");
  confirmModal({
    title: `Remover ${sel.length} app(s)?`,
    body: `<p>Vai desinstalar: <b>${escHtml(nomes)}</b>.</p><p style="color:var(--ink-3);font-size:12.5px">Só apps de usuário, <b>nada de sistema</b>. Reversível: reinstala pela Microsoft Store quando quiser.</p>`,
    okLabel: "Remover",
    onOk: async () => {
      busy(true, `Removendo ${sel.length} app(s)… (pode levar ~1 min)`);
      try {
        const r = await api("/api/debloat/remover", { pacotes: sel.map((a) => a.pacote) });
        const nok = (r.removidos || []).length, nfail = Object.keys(r.falhas || {}).length;
        toast(nok ? "ok" : "warn", `${nok} app(s) removido(s)`, nfail ? `${nfail} falharam (alguns são protegidos).` : "Espaço e segundo plano liberados.");
        renderBloat();
      } catch (e) { toast("err", "Falha ao remover", e.message); } finally { busy(false); }
    },
  });
}

/* ---- Ferramentas rápidas (Manual Mestre) --------------------------------- */
async function renderTools() {
  const box = $("#toolsGrid"); if (!box) return;
  let hib = { ligada: false, mb: 0 };
  try { hib = await api("/api/ferramentas/hibernacao/status"); } catch (e) {}
  const tools = [
    { id: "turbo", icon: "bolt", nome: "Modo Turbo", desc: "Ativa Energia Máxima + limpa rede + TRIM nos SSDs em uma só chamada — boost completo antes de jogar.", btn: "⚡ Ativar Turbo", highlight: true },
    { id: "dns", icon: "netcat", nome: "DNS Rápido", desc: "Troca o DNS para Cloudflare, Google ou Quad9 num clique — resolve lentidão em logins e matchmaking online.", btn: "Configurar DNS" },
    { id: "rede", icon: "netcat", nome: "Faxina de rede", desc: "Limpa o cache DNS e reseta TCP/IP + Winsock — resolve lentidão e erros de conexão.", btn: "Limpar rede" },
    { id: "trim", icon: "sys", nome: "TRIM nos SSDs", desc: "Roda o TRIM (manutenção correta de SSD). HDDs o Windows cuida sozinho.", btn: "Rodar TRIM" },
    { id: "energia", icon: "bolt", nome: "Plano Desempenho Máximo", desc: "Desbloqueia e ativa o plano de energia oculto \"Ultimate Performance\" (ideal desktop na tomada).", btn: "Ativar" },
    { id: "defender", icon: "shield", nome: "Varredura rápida (Defender)", desc: "PC lento às vezes é vírus/minerador. Dispara uma varredura rápida do Microsoft Defender.", btn: "Escanear" },
    { id: "hibernacao", icon: "broom", nome: hib.ligada ? "Liberar hibernação" : "Hibernação desligada",
      desc: hib.ligada ? `O hiberfil.sys ocupa ~${fmtCleanMB(hib.mb)}. Desligar libera esse espaço (e desativa a Inicialização Rápida).` : "A hibernação já está desligada (hiberfil.sys liberado).",
      btn: hib.ligada ? `Liberar ${fmtCleanMB(hib.mb)}` : "Reativar", on: hib.ligada },
    { id: "pinger", icon: "netcat", nome: "Pinger / Latência", desc: "Testa a latência para servidores de jogos (Riot, Steam, Epic) e DNS (Cloudflare, Google). Mostra min/máx/média/perda.", btn: "Testar latência" },
    { id: "hosts", icon: "shield", nome: "Editor de HOSTS", desc: "Edita o arquivo hosts do Windows. Bloqueie anúncios, telemetria ou redirecione domínios manualmente.", btn: "Abrir HOSTS" },
    { id: "wu", icon: "sys", nome: "Windows Update", desc: "Pause as atualizações automáticas enquanto joga ou grava. Retome com um clique quando quiser atualizar.", btn: "Verificar status" },
    { id: "dpc", icon: "activity", nome: "Latência DPC · Diagnóstico", desc: "Mede o % de tempo da CPU gasto em interrupções diferidas (DPC). Alta latência DPC causa áudio glitchy e micro-engasgos em jogos.", btn: "Medir DPC" },
    { id: "affinity", icon: "sys", nome: "Afinidade de CPU", desc: "Define quais cores de CPU cada processo pode usar. Útil para isolar jogos em núcleos físicos e evitar E-cores (Intel).", btn: "Gerenciar processos" },
  ];
  box.innerHTML = tools.map((t) => `
    <div class="tool-card${t.highlight ? " tool-turbo" : ""}">
      <div class="tool-h">${IC(t.icon)}<b>${escHtml(t.nome)}</b></div>
      <p>${escHtml(t.desc)}</p>
      <button class="btnG tool-btn${t.highlight ? " btn-turbo" : ""}" data-tool="${t.id}" data-on="${t.on ? 1 : 0}">${escHtml(t.btn)}</button>
    </div>`).join("");
  $$("#toolsGrid [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
}

async function runTool(id, el) {
  const fire = async (path, body, msg) => {
    if (el) el.disabled = true;
    busy(true, msg || "Executando…");
    try {
      const r = await api(path, body || {});
      return r;
    } catch (e) { toast("err", "Falha", e.message); return null; }
    finally { busy(false); if (el) el.disabled = false; }
  };
  if (id === "turbo") {
    confirmModal({ title: "Ativar Modo Turbo?",
      body: "<p>Executa 3 ações de uma vez: <b>Energia Máxima</b> (Ultimate Performance), <b>Limpeza de rede</b> (DNS + TCP/IP) e <b>TRIM nos SSDs</b>. Seguro e reversível.</p>",
      okLabel: "⚡ Ativar", onOk: async () => {
        busy(true, "Modo Turbo — aguarde…");
        try {
          const r = await api("/api/turbo");
          toast("ok", "Turbo ativado!", `Energia · Rede · TRIM — tudo pronto.`);
        } catch (e) { toast("err", "Turbo falhou", e.message); }
        finally { busy(false); }
      }
    });
  } else if (id === "dns") {
    const DNS_OPT = [
      { id: "cloudflare", nome: "Cloudflare", ip: "1.1.1.1 / 1.0.0.1",           nota: "Mais rápido globalmente · foco em privacidade" },
      { id: "google",     nome: "Google",     ip: "8.8.8.8 / 8.8.4.4",             nota: "Confiável · boa cobertura mundial" },
      { id: "quad9",      nome: "Quad9",      ip: "9.9.9.9 / 149.112.112.112",     nota: "Bloqueia domínios maliciosos · segurança" },
      { id: "opendns",    nome: "OpenDNS",    ip: "208.67.222.222 / 208.67.220.220", nota: "Estável · controle parental opcional" },
      { id: "dhcp",       nome: "Automático (DHCP)", ip: "Volta ao DNS do roteador", nota: "" },
    ];
    $("#mTitle").textContent = "DNS Rápido";
    $("#mBody").innerHTML = `<p style="margin:0 0 14px">Escolha o servidor DNS. <b>Efeito imediato</b> — sem reiniciar.</p>
      <div class="dns-grid">${DNS_OPT.map(p => `<button class="dns-opt" data-dns="${p.id}"><b>${escHtml(p.nome)}</b><span>${escHtml(p.ip)}</span>${p.nota ? `<small>${escHtml(p.nota)}</small>` : ""}</button>`).join("")}</div>`;
    $("#mFoot").innerHTML = `<button class="mbtn" data-close>Cancelar</button>`;
    $("#modal").classList.add("show");
    $$("#mBody .dns-opt").forEach(btn => {
      btn.onclick = async () => {
        closeModal();
        busy(true, "Aplicando DNS…");
        try { const r = await api("/api/ferramentas/dns", { provider: btn.dataset.dns }); toast(r.ok ? "ok" : "err", r.ok ? "DNS configurado" : "Falha ao configurar DNS", r.mensagem || r.erro || ""); }
        catch (e) { toast("err", "Falha", e.message); } finally { busy(false); }
      };
    });
  } else if (id === "rede") {
    confirmModal({ title: "Faxina de rede?", body: "<p>Limpa o DNS e reseta TCP/IP + Winsock. <b>Não derruba a conexão agora</b> (sem release/renew), mas o reset só vale após <b>reiniciar</b>. Pode resetar configs de VPN/proxy.</p>", okLabel: "Limpar rede",
      onOk: async () => { const r = await fire("/api/ferramentas/rede", {}, "Limpando rede…"); if (r) toast("ok", "Rede limpa", "Reinicie para concluir o reset."); } });
  } else if (id === "trim") {
    const r = await fire("/api/ferramentas/trim", {}, "Rodando TRIM…"); if (r) toast(r.ok ? "ok" : "warn", "TRIM", r.mensagem || "");
  } else if (id === "energia") {
    const r = await fire("/api/ferramentas/energia-max", {}, "Ativando plano…"); if (r) toast("ok", "Plano de energia", r.mensagem || "");
  } else if (id === "defender") {
    const r = await fire("/api/ferramentas/defender", {}, "Iniciando varredura…"); if (r) toast(r.ok ? "ok" : "err", r.ok ? "Varredura iniciada" : "Falha", r.ok ? "Roda em segundo plano; veja o Defender." : (r.erro || ""));
  } else if (id === "hibernacao") {
    const ligada = el.dataset.on === "1";
    confirmModal({ title: ligada ? "Liberar hibernação?" : "Reativar hibernação?",
      body: ligada ? "<p>Apaga o <b>hiberfil.sys</b> (libera GBs) e <b>desativa a Inicialização Rápida</b>. Reversível. Em notebook que você usa hibernação, prefira manter ligada.</p>" : "<p>Reativa a hibernação e a Inicialização Rápida.</p>",
      okLabel: ligada ? "Liberar" : "Reativar", danger: ligada,
      onOk: async () => { const r = await fire("/api/ferramentas/hibernacao", { on: !ligada }, "Aplicando…"); if (r) { toast("ok", ligada ? "Hibernação liberada" : "Hibernação reativada", ""); renderTools(); } } });
  } else if (id === "pinger") {
    await openPingerModal();
  } else if (id === "hosts") {
    await openHostsModal();
  } else if (id === "wu") {
    await openWUModal();
  } else if (id === "dpc") {
    await openDPCModal();
  } else if (id === "affinity") {
    await openAffinityModal();
  }
}

/* ---- Browser Cleaner ------------------------------------------------------- */
function renderBrowserClean() {
  const btnScan   = $("#btnBrowserScan");
  const btnClean  = $("#btnBrowserClean");
  const grid      = $("#browserGrid");
  const summary   = $("#browserSummary");
  const actions   = $("#browserActions");
  if (!btnScan) return;

  state.browserData = null;

  btnScan.onclick = async () => {
    btnScan.disabled = true;
    busy(true, "Verificando browsers…");
    try {
      const r = await api("/api/browser/scan");
      state.browserData = r.browsers || [];
      renderBrowserGrid(state.browserData);
      const detected = state.browserData.filter(b => b.detectado);
      const totalMB = detected.reduce((s, b) => s + b.total_mb, 0);
      summary.textContent = detected.length
        ? `${detected.length} browser${detected.length > 1 ? "s" : ""} detectado${detected.length > 1 ? "s" : ""} · ${fmtCleanMB(totalMB)} recuperáveis`
        : "Nenhum browser detectado.";
      actions.style.display = detected.length ? "" : "none";
    } catch (e) { toast("err", "Falha ao verificar", e.message); }
    finally { busy(false); btnScan.disabled = false; }
  };

  btnClean.onclick = async () => {
    const reqs = [];
    $$("#browserGrid .browser-cat-chk:checked").forEach(chk => {
      const browser = chk.dataset.browser;
      const cat = chk.dataset.cat;
      let req = reqs.find(r => r.browser === browser);
      if (!req) { req = { browser, cats: [] }; reqs.push(req); }
      req.cats.push(cat);
    });
    if (!reqs.length) { toast("warn", "Nada selecionado", "Marque ao menos uma categoria."); return; }
    confirmModal({
      title: "Limpar dados de browser?",
      body: "<p>Os dados selecionados serão apagados permanentemente. <b>Feche os browsers</b> antes de continuar. Cookies removem login dos sites.</p>",
      okLabel: "Limpar", danger: true,
      onOk: async () => {
        busy(true, "Limpando browsers…");
        try {
          const r = await api("/api/browser/limpar", { reqs });
          if (r.ok) {
            const det = (r.detalhes || []).join(" · ") || `${fmtCleanMB(r.liberado_mb || 0)} liberados`;
            toast("ok", "Browsers limpos", det);
            btnScan.click(); // re-scan
          } else {
            toast("err", "Falha", r.erro || "");
          }
        } catch (e) { toast("err", "Falha", e.message); }
        finally { busy(false); }
      }
    });
  };
}

function renderBrowserGrid(browsers) {
  const grid = $("#browserGrid"); if (!grid) return;
  const icons = { chrome: "🌐", edge: "🔵", brave: "🦁", firefox: "🦊" };
  grid.innerHTML = browsers.map(b => {
    if (!b.detectado) return `<div class="browser-card browser-card-off"><div class="bc-head">${icons[b.id] || "🌐"} <b>${escHtml(b.nome)}</b></div><p class="bc-none">Não detectado</p></div>`;
    const cats = (b.cats || []).map(c => `
      <label class="bc-cat">
        <input type="checkbox" class="browser-cat-chk" data-browser="${b.id}" data-cat="${c.id}" ${c.mb > 0 ? "checked" : ""}>
        <span class="bc-cat-name">${escHtml(c.nome)}</span>
        <span class="bc-cat-mb">${fmtCleanMB(c.mb)}</span>
        ${c.nota ? `<small class="bc-cat-nota">${escHtml(c.nota)}</small>` : ""}
      </label>`).join("");
    return `<div class="browser-card">
      <div class="bc-head">${icons[b.id] || "🌐"} <b>${escHtml(b.nome)}</b> <span class="bc-total">${fmtCleanMB(b.total_mb)}</span></div>
      <div class="bc-cats">${cats}</div>
    </div>`;
  }).join("");
}

/* ---- Pinger --------------------------------------------------------------- */
async function openPingerModal() {
  let presets = [];
  try { const r = await api("/api/ferramentas/ping/presets"); presets = r.presets || []; } catch (e) {}

  $("#mTitle").textContent = "Pinger · Teste de Latência";
  $("#mBody").innerHTML = `
    <div class="ping-setup">
      <div class="ping-presets">${presets.map(p => `<button class="ping-chip" data-host="${escHtml(p.host)}">${escHtml(p.nome)}</button>`).join("")}</div>
      <div class="ping-input-row">
        <input id="pingHost" class="game-add-input" placeholder="Host ou IP (ex: 1.1.1.1)" autocomplete="off" spellcheck="false" style="flex:1">
        <button class="btnG" id="btnDoPing">Pingar</button>
      </div>
    </div>
    <div id="pingResult" class="ping-result"></div>`;
  $("#mFoot").innerHTML = `<button class="mbtn" data-close>Fechar</button>`;
  $("#modal").classList.add("show");

  $$("#mBody .ping-chip").forEach(chip => {
    chip.onclick = () => { $("#pingHost").value = chip.dataset.host; $("#btnDoPing").click(); };
  });

  $("#btnDoPing").onclick = async () => {
    const host = ($("#pingHost").value || "").trim();
    if (!host) { toast("warn", "Host vazio", "Digite um endereço para pingar."); return; }
    const btn = $("#btnDoPing");
    btn.disabled = true;
    const res = $("#pingResult");
    res.innerHTML = `<div class="ping-row ping-loading">Pingando <b>${escHtml(host)}</b>…</div>`;
    try {
      const r = await api("/api/ferramentas/ping", { host });
      const p = r.resultado;
      if (!p) { res.innerHTML = `<div class="ping-row ping-err">Sem resultado.</div>`; return; }
      const qual = p.avg_ms === 0 ? "ping-err" : p.avg_ms < 50 ? "ping-ok" : p.avg_ms < 120 ? "ping-warn" : "ping-err";
      res.innerHTML = `
        <div class="ping-stats ${qual}">
          <div class="ps-item"><span>Mín</span><b>${p.min_ms} ms</b></div>
          <div class="ps-item"><span>Média</span><b>${p.avg_ms} ms</b></div>
          <div class="ps-item"><span>Máx</span><b>${p.max_ms} ms</b></div>
          <div class="ps-item"><span>Jitter</span><b>${p.jitter} ms</b></div>
          <div class="ps-item"><span>Perda</span><b>${p.pct_perda}%</b></div>
        </div>
        ${p.erro ? `<div class="ping-row ping-err">${escHtml(p.erro)}</div>` : ""}
        <div class="ping-bar-wrap">${renderPingBars(p)}</div>`;
    } catch (e) { res.innerHTML = `<div class="ping-row ping-err">Erro: ${escHtml(e.message)}</div>`; }
    finally { btn.disabled = false; }
  };
}

function renderPingBars(p) {
  if (!p || p.avg_ms === 0) return "";
  const max = Math.max(p.max_ms, 1);
  const bars = [
    { label: "Min", ms: p.min_ms }, { label: "Avg", ms: p.avg_ms }, { label: "Max", ms: p.max_ms },
  ];
  return bars.map(b => {
    const pct = Math.round((b.ms / max) * 100);
    const cl = b.ms < 50 ? "pb-ok" : b.ms < 120 ? "pb-warn" : "pb-bad";
    return `<div class="ping-bar-row"><span class="pb-lbl">${b.label}</span><div class="pb-track"><div class="pb-fill ${cl}" style="width:${pct}%"></div></div><span class="pb-val">${b.ms} ms</span></div>`;
  }).join("");
}

/* ---- Hosts Editor --------------------------------------------------------- */
async function openHostsModal() {
  $("#mTitle").textContent = "Editor de HOSTS";
  $("#mBody").innerHTML = `<div class="hosts-loading">Carregando…</div>`;
  $("#mFoot").innerHTML = `<button class="mbtn" data-close>Fechar</button>`;
  $("#modal").classList.add("show");
  await loadHostsEntries();
}

async function loadHostsEntries() {
  const body = $("#mBody"); if (!body) return;
  body.innerHTML = `<div class="hosts-loading">Carregando…</div>`;
  try {
    const r = await api("/api/hosts");
    const entries = r.entries || [];
    body.innerHTML = `
      <div class="hosts-add">
        <input id="hostsIP" class="game-add-input" placeholder="IP (ex: 0.0.0.0)" style="width:130px" autocomplete="off">
        <input id="hostsHost" class="game-add-input" placeholder="Domínio (ex: telemetry.microsoft.com)" style="flex:1" autocomplete="off">
        <input id="hostsComment" class="game-add-input" placeholder="Nota (opcional)" style="width:140px" autocomplete="off">
        <button class="btnG" id="btnHostsAdd">+ Adicionar</button>
      </div>
      <div class="hosts-list">
        ${entries.length ? entries.map(e => `
          <div class="hosts-row${e.disabled ? " hosts-disabled" : ""}${e.managed ? " hosts-managed" : ""}">
            <span class="hosts-ip">${escHtml(e.ip)}</span>
            <span class="hosts-host">${escHtml(e.host)}</span>
            ${e.comment ? `<span class="hosts-comment">${escHtml(e.comment)}</span>` : ""}
            ${e.managed ? `<span class="hosts-tag">ThazzDraco</span>` : ""}
            <div class="hosts-actions">
              <button class="abtn" data-hosts-toggle data-ip="${escHtml(e.ip)}" data-host="${escHtml(e.host)}">${e.disabled ? "Ativar" : "Desativar"}</button>
              <button class="abtn btn-danger2" data-hosts-rm data-ip="${escHtml(e.ip)}" data-host="${escHtml(e.host)}">Remover</button>
            </div>
          </div>`).join("") : `<p class="bench-note" style="padding:12px">Nenhuma entrada customizada encontrada.</p>`}
      </div>`;

    $("#btnHostsAdd").onclick = async () => {
      const ip = ($("#hostsIP").value || "").trim();
      const host = ($("#hostsHost").value || "").trim();
      const comment = ($("#hostsComment").value || "").trim();
      if (!ip || !host) { toast("warn", "Preencha IP e domínio", ""); return; }
      busy(true, "Adicionando entrada…");
      try {
        const r = await api("/api/hosts/adicionar", { ip, host, comment });
        if (r.ok) { toast("ok", "Entrada adicionada", `${ip} → ${host}`); await loadHostsEntries(); }
        else toast("err", "Falha", r.erro || "");
      } catch (e) { toast("err", "Falha", e.message); }
      finally { busy(false); }
    };

    $$("#mBody [data-hosts-toggle]").forEach(btn => {
      btn.onclick = async () => {
        busy(true, "Alternando entrada…");
        try {
          const r = await api("/api/hosts/toggle", { ip: btn.dataset.ip, host: btn.dataset.host });
          if (r.ok) await loadHostsEntries(); else toast("err", "Falha", r.erro || "");
        } catch (e) { toast("err", "Falha", e.message); }
        finally { busy(false); }
      };
    });

    $$("#mBody [data-hosts-rm]").forEach(btn => {
      btn.onclick = () => {
        const ip = btn.dataset.ip, host = btn.dataset.host;
        confirmModal({ title: "Remover entrada HOSTS?", body: `<p>Remove <b>${escHtml(ip)} ${escHtml(host)}</b> do arquivo hosts.</p>`, okLabel: "Remover", danger: true,
          onOk: async () => {
            busy(true, "Removendo…");
            try {
              const r = await api("/api/hosts/remover", { ip, host });
              if (r.ok) { toast("ok", "Entrada removida", ""); await loadHostsEntries(); }
              else toast("err", "Falha", r.erro || "");
            } catch (e) { toast("err", "Falha", e.message); }
            finally { busy(false); }
          }
        });
      };
    });
  } catch (e) {
    body.innerHTML = `<div class="ping-row ping-err">Erro ao ler o arquivo hosts: ${escHtml(e.message)}</div>`;
  }
}

/* ---- Windows Update ------------------------------------------------------- */
async function openWUModal() {
  $("#mTitle").textContent = "Windows Update";
  $("#mBody").innerHTML = `<div class="hosts-loading">Verificando…</div>`;
  $("#mFoot").innerHTML = `<button class="mbtn" data-close>Fechar</button>`;
  $("#modal").classList.add("show");
  await loadWUStatus();
}

async function loadWUStatus() {
  const body = $("#mBody"); if (!body) return;
  try {
    const r = await api("/api/ferramentas/wu/status");
    const isPaused = r.desativado || false;
    const statusLabel = isPaused ? "⏸ Pausado" : "▶ Ativo";
    const statusCls = isPaused ? "wu-paused" : "wu-active";
    body.innerHTML = `
      <div class="wu-status ${statusCls}">
        <div class="wu-status-label">${statusLabel}</div>
        <p class="wu-desc">${isPaused
          ? "O Windows Update está pausado. O sistema não buscará nem instalará atualizações automaticamente."
          : "O Windows Update está ativo. O sistema instala atualizações automaticamente em segundo plano."}</p>
        <div class="wu-actions">
          ${isPaused
            ? `<button class="btn-hero" id="btnWURetomar">▶ Retomar atualizações</button>`
            : `<button class="btn-danger2" id="btnWUPausar">⏸ Pausar atualizações</button>`}
        </div>
        <p class="bench-note" style="margin-top:12px">
          ${isPaused
            ? "Para atualizar manualmente: Configurações → Windows Update → Verificar atualizações."
            : "Pausar é recomendado antes de jogos competitivos ou gravações — atualizações podem causar drops de FPS."}
        </p>
      </div>`;

    const btnAct = $("#btnWUPausar") || $("#btnWURetomar");
    if (btnAct) {
      btnAct.onclick = async () => {
        const pausar = !!$("#btnWUPausar");
        const endpoint = pausar ? "/api/ferramentas/wu/pausar" : "/api/ferramentas/wu/retomar";
        busy(true, pausar ? "Pausando Windows Update…" : "Reativando Windows Update…");
        try {
          const r = await api(endpoint, {});
          toast(r.ok ? "ok" : "err", pausar ? "Windows Update pausado" : "Windows Update reativado", r.mensagem || r.erro || "");
          if (r.ok) await loadWUStatus();
        } catch (e) { toast("err", "Falha", e.message); }
        finally { busy(false); }
      };
    }
  } catch (e) {
    body.innerHTML = `<div class="ping-row ping-err">Erro: ${escHtml(e.message)}</div>`;
  }
}

/* ---- DPC Latency ---------------------------------------------------------- */
async function openDPCModal() {
  $("#mTitle").textContent = "Latência DPC · Diagnóstico";
  $("#mBody").innerHTML = `<div class="hosts-loading">Medindo latência DPC (aguarde ~2s)…</div>`;
  $("#mFoot").innerHTML = `<button class="mbtn" data-close>Fechar</button>`;
  $("#modal").classList.add("show");
  try {
    const r = await api("/api/ferramentas/dpc");
    const cls = r.avaliacao === "ok" ? "dpc-ok" : r.avaliacao === "moderada" ? "dpc-warn" : "dpc-bad";
    const icon = r.avaliacao === "ok" ? "✅" : r.avaliacao === "moderada" ? "⚠️" : "🔴";
    const dicas = (r.dicas || []).map(d => `<li>${escHtml(d)}</li>`).join("");
    $("#mBody").innerHTML = `
      <div class="dpc-result ${cls}">
        <div class="dpc-big">${icon} <span>${r.avaliacao === "ok" ? "Normal" : r.avaliacao === "moderada" ? "Moderada" : "Alta"}</span></div>
        <div class="dpc-stats">
          <div class="dpc-stat"><span>% DPC Time</span><b>${r.dpc_time_pct.toFixed(2)}%</b></div>
          ${r.dpc_rate_per_sec > 0 ? `<div class="dpc-stat"><span>DPCs/s</span><b>${Math.round(r.dpc_rate_per_sec)}</b></div>` : ""}
        </div>
        <p class="dpc-msg">${escHtml(r.mensagem)}</p>
        ${dicas ? `<div class="dpc-dicas"><b>Como resolver:</b><ul>${dicas}</ul></div>` : ""}
      </div>
      <p class="bench-note" style="margin-top:10px">Use <b>LatencyMon</b> (gratuito) para identificar qual driver específico está causando o problema.</p>`;
  } catch (e) {
    $("#mBody").innerHTML = `<div class="ping-row ping-err">Erro: ${escHtml(e.message)}</div>`;
  }
}

/* ---- Process Affinity Manager --------------------------------------------- */
async function openAffinityModal() {
  $("#mTitle").textContent = "Afinidade de CPU";
  $("#mBody").innerHTML = `<div class="hosts-loading">Carregando processos…</div>`;
  $("#mFoot").innerHTML = `<button class="mbtn" data-close>Fechar</button>`;
  $("#modal").classList.add("show");
  await loadAffinityProcesses();
}

async function loadAffinityProcesses() {
  const body = $("#mBody"); if (!body) return;
  try {
    const r = await api("/api/ferramentas/afinidade");
    const procs = (r.processos || []).sort((a, b) => a.nome.localeCompare(b.nome));
    const totalCores = procs[0]?.total_cores || 0;
    const allMask = (BigInt(1) << BigInt(totalCores)) - BigInt(1);

    body.innerHTML = `
      <div class="aff-intro">
        Define em quais <b>núcleos de CPU</b> cada processo pode rodar. Útil para forçar jogos a usar
        apenas núcleos físicos (evitar E-cores em Intel) ou isolar processos pesados.
        <span class="bench-note"> Sistema tem ${totalCores} núcleos lógicos.</span>
      </div>
      <div class="aff-search-row">
        <input id="affSearch" class="game-add-input" placeholder="Filtrar por nome…" autocomplete="off" style="flex:1">
        <span id="affCount" class="bench-note"></span>
      </div>
      <div class="aff-list" id="affList">
        ${procs.map(p => {
          const allCores = BigInt(p.mask) === allMask;
          return `<div class="aff-row${allCores ? "" : " aff-custom"}" data-pid="${p.pid}" data-name="${escHtml(p.nome).toLowerCase()}">
            <span class="aff-name" title="PID ${p.pid}">${escHtml(p.nome)}</span>
            <span class="aff-cores">${allCores ? "todos os núcleos" : `${p.cores_ativos}/${p.total_cores} núcleos`}</span>
            <button class="aff-btn" data-aff-pid="${p.pid}" data-aff-mask="${p.mask}" data-aff-total="${p.total_cores}">Editar</button>
          </div>`;
        }).join("")}
      </div>`;

    const countEl = $("#affCount");
    if (countEl) countEl.textContent = `${procs.length} processos`;

    const searchEl = $("#affSearch");
    if (searchEl) {
      searchEl.oninput = () => {
        const q = searchEl.value.toLowerCase();
        $$("#affList .aff-row").forEach(row => {
          row.style.display = row.dataset.name.includes(q) ? "" : "none";
        });
      };
    }

    $$("#mBody [data-aff-pid]").forEach(btn => {
      btn.onclick = () => openAffinityEditor(
        parseInt(btn.dataset.affPid),
        BigInt(btn.dataset.affMask),
        parseInt(btn.dataset.affTotal)
      );
    });
  } catch (e) {
    body.innerHTML = `<div class="ping-row ping-err">Erro: ${escHtml(e.message)}</div>`;
  }
}

function openAffinityEditor(pid, currentMask, totalCores) {
  const proc = $(`#affList [data-pid="${pid}"]`);
  const name = proc ? proc.querySelector(".aff-name")?.textContent : `PID ${pid}`;

  const cores = [];
  for (let i = 0; i < totalCores; i++) {
    const active = (currentMask >> BigInt(i)) & BigInt(1);
    cores.push(`<label class="aff-core-chk">
      <input type="checkbox" data-bit="${i}" ${active ? "checked" : ""}>
      <span>C${i}</span>
    </label>`);
  }

  confirmModal({
    title: `Afinidade: ${name}`,
    body: `<p style="margin:0 0 10px;font-size:13px">Selecione os <b>núcleos</b> que este processo pode usar. <span class="bench-note">Ao menos 1 deve estar ativo.</span></p>
      <div class="aff-core-grid">${cores.join("")}</div>
      <div style="margin-top:8px;display:flex;gap:8px">
        <button class="aff-btn" id="affSelAll">Todos</button>
        <button class="aff-btn" id="affSelPhysical">Só físicos (pares)</button>
      </div>`,
    okLabel: "Aplicar",
    onOk: async () => {
      const checkboxes = $$("#mBody .aff-core-chk input");
      let newMask = BigInt(0);
      checkboxes.forEach(chk => { if (chk.checked) newMask |= (BigInt(1) << BigInt(chk.dataset.bit)); });
      if (newMask === BigInt(0)) { toast("warn", "Selecione ao menos 1 núcleo", ""); return; }
      busy(true, "Aplicando afinidade…");
      try {
        const r = await api("/api/ferramentas/afinidade/set", { pid, mask: Number(newMask) });
        if (r.ok) { toast("ok", "Afinidade definida", `PID ${pid}: ${popcount(newMask)} núcleos ativos.`); await loadAffinityProcesses(); }
        else toast("err", "Falha", r.erro || "");
      } catch (e) { toast("err", "Falha", e.message); }
      finally { busy(false); }
    }
  });
  // Botões de seleção rápida
  setTimeout(() => {
    const selAll = $("#affSelAll");
    const selPhys = $("#affSelPhysical");
    if (selAll) selAll.onclick = () => $$("#mBody .aff-core-chk input").forEach(c => c.checked = true);
    if (selPhys) selPhys.onclick = () => $$("#mBody .aff-core-chk input").forEach((c, i) => c.checked = (i % 2 === 0));
  }, 0);
}

function popcount(n) {
  let count = 0;
  while (n) { count += Number(n & BigInt(1)); n >>= BigInt(1); }
  return count;
}

/* ---- Custom Presets ------------------------------------------------------- */
async function renderCustomPresets() {
  const box = $("#presets"); if (!box) return;
  // Os built-in presets são renderizados por renderPresets() — nós adicionamos os custom depois
  try {
    const r = await api("/api/presets/custom");
    const custom = r.presets || [];
    const existing = $("#customPresetsRow");
    if (existing) existing.remove();

    const wrap = document.createElement("div");
    wrap.id = "customPresetsRow";
    wrap.className = "custom-presets-row";
    wrap.innerHTML = `
      <div class="cp-header">
        <span class="cp-label">Meus Presets</span>
        <button class="btnG cp-new" id="btnNewPreset">+ Criar preset</button>
      </div>
      ${custom.length ? `<div class="cp-list">${custom.map(p => `
        <div class="cp-card">
          <div class="cp-card-top">
            <b>${escHtml(p.nome)}</b>
            <span class="cp-rule-count">${p.ids?.length || 0} regras</span>
          </div>
          ${p.descricao ? `<p class="cp-desc">${escHtml(p.descricao)}</p>` : ""}
          <div class="cp-actions">
            <button class="btnG" data-cp-apply="${escHtml(p.id)}">⚡ Aplicar</button>
            <button class="abtn btn-danger2" data-cp-del="${escHtml(p.id)}">Excluir</button>
          </div>
        </div>`).join("")}</div>` : `<p class="bench-note cp-empty">Nenhum preset criado ainda. Clique em "+ Criar preset" para salvar sua seleção atual.</p>`}`;
    box.after(wrap);

    $("#btnNewPreset").onclick = () => openCreatePresetModal();

    wrap.querySelectorAll("[data-cp-apply]").forEach(btn => {
      btn.onclick = async () => {
        confirmModal({ title: `Aplicar "${custom.find(p=>p.id===btn.dataset.cpApply)?.nome}"?`,
          body: "<p>Vai aplicar todas as regras deste preset que ainda não estiverem ativas. Totalmente reversível.</p>",
          okLabel: "Aplicar",
          onOk: async () => {
            busy(true, "Aplicando preset…");
            try {
              const r = await api("/api/presets/custom/aplicar", { id: btn.dataset.cpApply });
              if (r.relatorio) {
                const n = r.relatorio.aplicados || 0;
                toast("ok", "Preset aplicado", `${n} regra${n!==1?"s":""} ativada${n!==1?"s":""}.`);
                if (r.scan) { ingest(r.scan); }
              } else toast("err", "Falha", r.erro || "");
            } catch (e) { toast("err", "Falha", e.message); }
            finally { busy(false); }
          }
        });
      };
    });

    wrap.querySelectorAll("[data-cp-del]").forEach(btn => {
      btn.onclick = () => {
        const id = btn.dataset.cpDel, nome = custom.find(p=>p.id===id)?.nome || id;
        confirmModal({ title: "Excluir preset?", body: `<p>Remove o preset <b>${escHtml(nome)}</b>. Esta ação não pode ser desfeita.</p>`, okLabel: "Excluir", danger: true,
          onOk: async () => {
            busy(true, "Excluindo…");
            try {
              const r = await api("/api/presets/custom/excluir", { id });
              if (r.ok) { toast("ok", "Preset excluído", ""); await renderCustomPresets(); }
              else toast("err", "Falha", r.erro || "");
            } catch (e) { toast("err", "Falha", e.message); }
            finally { busy(false); }
          }
        });
      };
    });
  } catch (e) { /* custom presets são opcionais — falha silenciosa */ }
}

function openCreatePresetModal() {
  const applied = (state.rules || []).filter(r => r.estado === "aplicado" && r.modo === "acionavel");
  const groups = {};
  applied.forEach(r => {
    const sec = sectionOf(r.categoria) || "outros";
    if (!groups[sec]) groups[sec] = [];
    groups[sec].push(r);
  });
  const rulesList = Object.entries(groups).map(([sec, rules]) =>
    `<div class="cp-group"><b>${sec}</b>${rules.map(r =>
      `<label class="cp-rule-chk"><input type="checkbox" value="${escHtml(r.id)}" checked> ${escHtml(r.titulo)}</label>`
    ).join("")}</div>`
  ).join("") || `<p class="bench-note">Nenhuma regra aplicada encontrada. Execute a varredura primeiro.</p>`;

  $("#mTitle").textContent = "Criar Preset Personalizado";
  $("#mBody").innerHTML = `
    <div class="cp-create">
      <input id="cpNome" class="game-add-input" placeholder="Nome do preset (ex: Meu Config de FPS)" style="width:100%;box-sizing:border-box" autocomplete="off">
      <input id="cpDesc" class="game-add-input" placeholder="Descrição (opcional)" style="width:100%;box-sizing:border-box;margin-top:6px" autocomplete="off">
      <div style="margin-top:12px;font-size:13px;color:var(--ink-2)">Regras a incluir (baseadas no estado atual):</div>
      <div class="cp-rules-scroll">${rulesList}</div>
    </div>`;
  $("#mFoot").innerHTML = `
    <button class="mbtn" data-close>Cancelar</button>
    <button class="mbtn primary" id="btnSavePreset">Salvar preset</button>`;
  $("#modal").classList.add("show");

  $("#btnSavePreset").onclick = async () => {
    const nome = ($("#cpNome")?.value || "").trim();
    if (!nome) { toast("warn", "Nome obrigatório", ""); return; }
    const ids = [...$$("#mBody .cp-rule-chk input:checked")].map(c => c.value);
    if (!ids.length) { toast("warn", "Selecione ao menos 1 regra", ""); return; }
    const desc = ($("#cpDesc")?.value || "").trim();
    closeModal();
    busy(true, "Salvando preset…");
    try {
      const r = await api("/api/presets/custom/salvar", { nome, descricao: desc, ids });
      if (r.ok) { toast("ok", "Preset salvo", `"${nome}" com ${ids.length} regras.`); await renderCustomPresets(); }
      else toast("err", "Falha", r.erro || "");
    } catch (e) { toast("err", "Falha", e.message); }
    finally { busy(false); }
  };
}

/* ---- F5: Histórico de Performance ----------------------------------------- */
const PERF_MAX = 300; // ~10 min a 2s por amostra
const PERF_KEY = "tz_perf_hist";

/* ---- F1: Monitor de Temperatura ao vivo ----------------------------------- */
const TEMP_MAX = 90; // ~3min a 2s por amostra
let tempHist = [];
let _tempAlerted = false;

function perfHistoryPush(m) {
  const cpu = m.cpu_pct || 0;
  const ram = m.ram?.pct || 0;
  const gpus = m.gpus || [];
  const gpu = gpus.length ? Math.max(0, gpus[0].uso_pct ?? 0) : 0;
  const temp = m.cpu_temp_c ?? -1;
  const t = Date.now();
  // carrega do localStorage + append
  let hist = [];
  try { hist = JSON.parse(localStorage.getItem(PERF_KEY) || "[]"); } catch (e) {}
  if (!Array.isArray(hist)) hist = [];
  hist.push({ t, cpu, ram, gpu, temp });
  if (hist.length > PERF_MAX) hist = hist.slice(-PERF_MAX);
  try { localStorage.setItem(PERF_KEY, JSON.stringify(hist)); } catch (e) {}
  // atualiza o chart se a aba histórico estiver visível
  if (state.page === "historico") renderPerfChart();
  // F1: histórico de temperatura em memória
  const gpuT = (gpus.length && gpus[0].temp_c >= 0) ? gpus[0].temp_c : -1;
  const cpuT = m.cpu_temp_c ?? -1;
  tempHist.push({ t, cpu: cpuT, gpu: gpuT });
  if (tempHist.length > TEMP_MAX) tempHist = tempHist.slice(-TEMP_MAX);
  const maxT = Math.max(cpuT, gpuT);
  if (!_tempAlerted && maxT >= 90) { _tempAlerted = true; toast("err", "Temperatura crítica!", `${maxT}°C — risco de throttling térmico.`); }
  else if (maxT < 85) _tempAlerted = false;
  if (state.page === "medicao") renderTempChart();
}

function perfHistoryLoad() {
  try { return JSON.parse(localStorage.getItem(PERF_KEY) || "[]"); } catch (e) { return []; }
}

function renderTempChart() {
  const canvas = document.getElementById("tempChartCanvas");
  if (!canvas) return;
  const W = canvas.offsetWidth || 600, H = 130;
  canvas.width = W; canvas.height = H;
  const ctx = canvas.getContext("2d");
  ctx.clearRect(0, 0, W, H);
  const TMIN = 20, TMAX = 100;
  const ty = (t) => H - ((Math.min(TMAX, Math.max(TMIN, t)) - TMIN) / (TMAX - TMIN)) * H;
  // grade horizontal
  ctx.lineWidth = 1;
  [40, 60, 80].forEach((v) => {
    ctx.strokeStyle = "#0f1f30"; ctx.setLineDash([]);
    ctx.beginPath(); ctx.moveTo(0, ty(v)); ctx.lineTo(W, ty(v)); ctx.stroke();
  });
  // linha 90°C (alerta)
  const y90 = ty(90);
  ctx.strokeStyle = "rgba(255,80,60,0.45)"; ctx.setLineDash([4, 5]); ctx.lineWidth = 1;
  ctx.beginPath(); ctx.moveTo(0, y90); ctx.lineTo(W, y90); ctx.stroke();
  ctx.setLineDash([]);
  ctx.fillStyle = "rgba(255,80,60,0.6)"; ctx.font = "10px monospace"; ctx.textAlign = "right";
  ctx.fillText("90°C", W - 4, y90 - 3); ctx.textAlign = "left";
  if (tempHist.length < 2) {
    ctx.fillStyle = "#2a4a6a"; ctx.font = "12px monospace"; ctx.textAlign = "center";
    ctx.fillText("Aguardando amostras de temperatura…", W / 2, H / 2); return;
  }
  [{ key: "cpu", color: "#ff6030" }, { key: "gpu", color: "#b060ff" }].forEach(({ key, color }) => {
    ctx.beginPath(); ctx.strokeStyle = color; ctx.lineWidth = 2;
    let first = true;
    tempHist.forEach((pt, i) => {
      if (pt[key] < 0) { first = true; return; }
      const x = (i / (tempHist.length - 1)) * W, y = ty(pt[key]);
      if (first) { ctx.moveTo(x, y); first = false; } else ctx.lineTo(x, y);
    });
    ctx.stroke();
  });
  // atualiza labels
  const last = tempHist[tempHist.length - 1];
  const tempColor = (v) => v >= 90 ? "var(--red)" : v >= 75 ? "var(--amber)" : "var(--cyan)";
  const cpuEl = document.getElementById("tcCpu"), gpuEl = document.getElementById("tcGpu");
  if (cpuEl) { cpuEl.textContent = last.cpu >= 0 ? last.cpu + "°C" : "N/D"; cpuEl.style.color = last.cpu >= 0 ? tempColor(last.cpu) : ""; }
  if (gpuEl) { gpuEl.textContent = last.gpu >= 0 ? last.gpu + "°C" : "N/D"; gpuEl.style.color = last.gpu >= 0 ? tempColor(last.gpu) : ""; }
}

function renderPerfChart() {
  const canvas = document.getElementById("perfChartCanvas");
  if (!canvas) return;
  const hist = perfHistoryLoad();
  const W = canvas.offsetWidth || 600, H = 140;
  canvas.width = W; canvas.height = H;
  const ctx = canvas.getContext("2d");
  ctx.clearRect(0, 0, W, H);
  if (hist.length < 2) {
    ctx.fillStyle = "#2a4a6a";
    ctx.font = "12px monospace";
    ctx.textAlign = "center";
    ctx.fillText("Aguardando amostras…", W / 2, H / 2);
    return;
  }
  // grade horizontal
  ctx.strokeStyle = "#0f1f30";
  ctx.lineWidth = 1;
  [25, 50, 75].forEach((p) => {
    const y = H - (p / 100) * H;
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(W, y); ctx.stroke();
  });
  // linhas de dados
  const lines = [
    { key: "cpu", color: "#41c7ff" },
    { key: "ram", color: "#ffa030" },
    { key: "gpu", color: "#00c47a" },
  ];
  lines.forEach(({ key, color }) => {
    ctx.beginPath();
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    hist.forEach((pt, i) => {
      const x = (i / (hist.length - 1)) * W;
      const y = H - (Math.max(0, Math.min(100, pt[key] || 0)) / 100) * H;
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    });
    ctx.stroke();
  });
  // stats texto
  const last = hist[hist.length - 1];
  const cpuEl = document.getElementById("phCpu");
  const ramEl = document.getElementById("phRam");
  const gpuEl = document.getElementById("phGpu");
  const tempEl = document.getElementById("phTemp");
  if (cpuEl) cpuEl.textContent = last.cpu + "%";
  if (ramEl) ramEl.textContent = last.ram + "%";
  if (gpuEl) gpuEl.textContent = last.gpu >= 0 ? last.gpu + "%" : "N/A";
  if (tempEl) tempEl.textContent = last.temp >= 0 ? last.temp + "°C" : "N/A";
}

/* ---- F11: Scanner de Serviços Pesados ------------------------------------- */
async function runServiceScan() {
  const btn = $("#btnSvcScan"), box = $("#svcList"), sum = $("#svcSummary");
  if (!box) return;
  if (btn) btn.disabled = true;
  box.innerHTML = `<div class="empty">${IC("ring")}<div>Verificando serviços…</div></div>`;
  try {
    const svcs = await api("/api/servicos");
    const rodando = svcs.filter((s) => s.rodando);
    if (sum) sum.textContent = rodando.length ? `${rodando.length} rodando (${svcs.length} verificados)` : `Nenhum em execução (${svcs.length} verificados)`;
    if (!svcs.length) { box.innerHTML = `<div class="empty">${IC("ok")}<div>Nenhum serviço detectado.</div></div>`; return; }
    const IMP_COL = { Alto: "var(--red)", Médio: "var(--amber)", Baixo: "var(--cyan)" };
    box.innerHTML = svcs.map((s) => `
      <div class="svc-row" data-svc="${escHtml(s.nome)}">
        <div class="svc-meta">
          <b>${escHtml(s.exibir)}</b>
          <span class="svc-cat" style="color:${IMP_COL[s.impacto] || "var(--ink-2)"}">${escHtml(s.cat)} · ${escHtml(s.impacto)}</span>
          <span class="svc-desc">${escHtml(s.desc)}</span>
        </div>
        <div class="svc-act">
          <span class="svc-badge ${s.rodando ? "on" : "off"}">${s.rodando ? "Rodando" : "Parado"}</span>
          ${s.rodando ? `<button class="abtn svc-stop-btn" data-svcstop="${escHtml(s.nome)}" title="Parar temporariamente">Parar</button>` : ""}
        </div>
      </div>`).join("");
  } catch (e) { box.innerHTML = `<div class="empty">${IC("err")}<div>Falha: ${escHtml(e.message)}</div></div>`; }
  finally { if (btn) btn.disabled = false; }
}

document.addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-svcstop]"); if (!btn) return;
  const nome = btn.dataset.svcstop; if (!nome) return;
  btn.disabled = true;
  try {
    const r = await api("/api/servicos/parar", { nome });
    if (r.ok) {
      const badge = btn.closest(".svc-row")?.querySelector(".svc-badge");
      if (badge) { badge.textContent = "Parado"; badge.className = "svc-badge off"; }
      btn.remove();
      toast("ok", "Serviço parado", nome);
    } else { toast("err", "Falha ao parar", r.erro || nome); btn.disabled = false; }
  } catch (e2) { toast("err", "Erro", e2.message); btn.disabled = false; }
});

/* ---- F12: Auditoria de Drivers ------------------------------------------- */
async function runDriverAudit() {
  const btn = $("#btnDriverAudit"), box = $("#drvList"), sum = $("#drvSummary");
  if (!box) return;
  if (btn) btn.disabled = true;
  box.innerHTML = `<div class="empty">${IC("ring")}<div>Verificando drivers…</div></div>`;
  try {
    const drivers = await api("/api/drivers/verificar");
    const antigos = drivers.filter((d) => d.status === "antigo" || d.status === "muito_antigo");
    if (sum) sum.textContent = antigos.length ? `${antigos.length} driver(s) antigo(s)` : "Todos os drivers em dia";
    if (!drivers.length) { box.innerHTML = `<div class="empty">${IC("ok")}<div>Nenhum driver encontrado.</div></div>`; return; }
    const ST = { ok: { col: "var(--green)", lbl: "OK" }, antigo: { col: "var(--amber)", lbl: "Antigo" }, muito_antigo: { col: "var(--red)", lbl: "Muito antigo" }, desconhecido: { col: "var(--ink-3)", lbl: "Desconhecido" } };
    box.innerHTML = drivers.map((d) => {
      const st = ST[d.status] || ST.desconhecido;
      const age = d.idade_meses >= 0 ? `${d.idade_meses}m atrás` : "data desconhecida";
      return `<div class="drv-row">
        <div class="drv-meta">
          <b>${escHtml(d.nome)}</b>
          <span class="drv-cat">${escHtml(d.cat)}</span>
          <span class="drv-ver">${escHtml(d.versao || "—")} · ${escHtml(d.data || "—")} (${age})</span>
        </div>
        <span class="drv-badge" style="color:${st.col}">${st.lbl}</span>
      </div>`;
    }).join("");
    if (antigos.length) toast("warn", `${antigos.length} driver(s) antigo(s)`, "Atualize pelo Windows Update ou site do fabricante.");
  } catch (e) { box.innerHTML = `<div class="empty">${IC("err")}<div>Falha: ${escHtml(e.message)}</div></div>`; }
  finally { if (btn) btn.disabled = false; }
}

/* ---- F1: Modo Game ao Vivo ------------------------------------------------- */
let _liveGameTimer = null;

function startLiveGamePoll() {
  if (_liveGameTimer) return;
  _liveGameTimer = setInterval(async () => {
    try {
      const st = await api("/api/modo-game/status");
      updateLiveGameBar(st.ativo, st.rodando || []);
    } catch (e) {}
  }, 3000);
}

function stopLiveGamePoll() {
  if (_liveGameTimer) { clearInterval(_liveGameTimer); _liveGameTimer = null; }
}

function updateLiveGameBar(ativo, rodando) {
  const dot = $("#lgbDot"), status = $("#lgbStatus"), running = $("#lgbRunning"), toggle = $("#lgbToggle");
  if (!dot) return;
  if (ativo) {
    toggle?.classList.add("on");
    if (rodando.length) {
      dot.className = "lgb-dot live";
      status.textContent = "Jogando agora";
      running.innerHTML = rodando.map((g) => `<span class="lgb-game">${IC("gamepad")} ${escHtml(g.nome)}</span>`).join("");
    } else {
      dot.className = "lgb-dot active";
      status.textContent = "Monitorando…";
      running.innerHTML = "";
    }
  } else {
    toggle?.classList.remove("on");
    dot.className = "lgb-dot";
    status.textContent = "Desativado";
    running.innerHTML = "";
  }
}

async function initLiveGameBar() {
  try {
    const st = await api("/api/modo-game/status");
    updateLiveGameBar(st.ativo, st.rodando || []);
    if (st.ativo) startLiveGamePoll();
  } catch (e) {}
}


/* ---- Perfis (presets) ---------------------------------------------------- */
const PRESET_TAG = { competitivo: "Latência mínima", equilibrado: "Seguro p/ a maioria", streaming: "Estável p/ transmitir" };
async function renderPresets() {
  const box = $("#presets"); if (!box) return;
  let ps = state.presets;
  if (!ps) { try { ps = await api("/api/presets"); state.presets = ps; } catch (e) { box.innerHTML = ""; return; } }
  box.innerHTML = (ps || []).map((p) => `
    <div class="preset-card${p.id === "competitivo" ? " hot" : ""}">
      <div class="preset-h">${escHtml(p.nome)}${PRESET_TAG[p.id] ? `<span class="preset-tag">${PRESET_TAG[p.id]}</span>` : ""}</div>
      <p>${escHtml(p.descricao)}</p>
      <div class="preset-foot"><span>${(p.ids || []).length} ajustes</span>
        <button class="btnG" data-preset="${p.id}"><span data-icon="bolt"></span> Aplicar</button></div>
    </div>`).join("");
  $$("#presets [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
}

function applyPreset(id) {
  const p = (state.presets || []).find((x) => x.id === id); if (!p) return;
  confirmModal({
    title: `Aplicar perfil ${p.nome}?`,
    body: `<p>${escHtml(p.descricao)}</p><p style="color:var(--ink-3);font-size:12.5px">Aplica até <b>${(p.ids || []).length} ajustes</b> de uma vez (pula o que já está aplicado ou não cabe no seu PC). Tudo <b>reversível</b> pelo Histórico.</p>`,
    okLabel: "Aplicar perfil",
    onOk: async () => {
      busy(true, `Aplicando ${p.nome}…`);
      try {
        const data = await api("/api/aplicar-preset", { preset_id: id, confirmar: true });
        afterMutation(data);
        const rep = data.relatorio || {}, n = (rep.aplicadas || []).length;
        const negado = Object.values(rep.erros || {}).some((m) => /denied|negad|acesso/i.test(m || ""));
        toast(n ? "ok" : "info", `Perfil ${p.nome}`, n ? `${n} ajuste(s) aplicados.` : (negado ? "Precisa de administrador." : "Já estava tudo aplicado."));
        if (negado) adminWarn();
      } catch (e) { toast("err", "Falha ao aplicar perfil", e.message); } finally { busy(false); }
    },
  });
}

/* ---- Boot ---------------------------------------------------------------- */
/* ---- Atualização automática ------------------------------------------------ */
async function checkForUpdate() {
  try {
    const r = await api("/api/update/check");
    if (!r.available) return;
    const banner = $("#updateBanner"); if (!banner) return;
    const vEl = $("#updateVersion"), nEl = $("#updateNotes");
    if (vEl) vEl.textContent = r.version || "";
    if (nEl && r.notes) {
      // pega só a primeira linha das notas de release
      const firstLine = r.notes.split("\n").find(l => l.trim()) || "";
      nEl.textContent = firstLine.replace(/^#+\s*/, "");
    }
    banner.classList.remove("hidden");
    const btn = $("#btnUpdateDownload");
    if (btn) btn.onclick = () => {
      if (r.download_url) window.open(r.download_url, "_blank");
    };
    const dismiss = $("#btnUpdateDismiss");
    if (dismiss) dismiss.onclick = () => banner.classList.add("hidden");
  } catch {}
}

// Checa na abertura (45s de delay no backend) e a cada 15 min na UI
function scheduleUpdateCheck() {
  // primeira checagem: pergunta ao backend após 60s (ele já iniciou a busca 45s antes)
  setTimeout(async () => {
    await checkForUpdate();
    setInterval(checkForUpdate, 15 * 60 * 1000);
  }, 60_000);
}

async function boot() {
  // #5 modo performance: respeita a escolha salva; senão auto-detecta PC fraco.
  let perfSaved = null; try { perfSaved = localStorage.getItem("tz_perf"); } catch (e) {}
  const perfAuto = (navigator.hardwareConcurrency || 8) <= 2 || matchMedia("(prefers-reduced-motion: reduce)").matches;
  setPerf(perfSaved !== null ? perfSaved === "1" : perfAuto);

  buildGauge(); spawnParticles(); wire();
  // C4: SSE heartbeat — substitui polling /api/ping; sobrevive a throttling de aba
  // inativa porque é uma conexão persistente, não um timer. Reconecta se cair.
  (function hb() {
    const es = new EventSource("/api/heartbeat");
    es.onerror = () => { es.close(); setTimeout(hb, 3000); };
  })();
  try { const info = await api("/api/info"); const b = $("#adminBadge"); state.admin = !!info.admin;
    if (info.admin) b.innerHTML = `<span class="dot"></span> Administrador · v${info.versao}`;
    else { b.classList.add("warn"); b.innerHTML = `<span class="dot"></span> Sem admin · v${info.versao}`; $("#noadmin").classList.add("show"); }
    const bv = $("#buildVer");
    if (bv) {
      bv.textContent = "v" + (info.versao || "—");
      bv.title = `ThazzDraco v${info.versao} — Clique para reconstruir (CONSTRUIR.ps1)`;
      bv.onclick = async () => {
        if (bv.classList.contains("building")) return;
        bv.classList.add("building"); bv.textContent = "BUILDING…";
        try {
          const r = await api("/api/rebuild");
          if (r.ok) toast("ok", "Build iniciado", "Terminal aberto — aguarde concluir e relance o app.");
          else toast("err", "Build falhou", r.erro || "Verifique CONSTRUIR.ps1");
        } catch (e) { toast("err", "Build falhou", e.message); }
        finally { setTimeout(() => { bv.classList.remove("building"); bv.textContent = "v" + (info.versao || "—"); }, 3000); }
      };
    }
  } catch (e) {}
  setIdle(); // SEM auto-scan: o usuário clica em "Escanear PC"
  renderPresets(); // perfis disponíveis mesmo antes de escanear
  renderCustomPresets(); // presets do usuário carregam em paralelo
  scheduleUpdateCheck(); // verifica atualização em background
}
// Estado inicial (antes de escanear): convida a clicar, sem dados ainda.
function setIdle() {
  $("#kicker").textContent = "núcleo · em espera";
  const n = $("#gNum"); n.textContent = "—"; n.style.color = "var(--ink-3)";
  $("#statusTitle").textContent = "Hora de acordar a máquina";
  $("#statusSub").innerHTML = "Uma varredura profunda revela cada FPS preso no sistema. Clique em <b>Escanear PC</b>.";
  const hs = $("#hudStatus"); if (hs) hs.textContent = "Núcleo online · aguardando varredura";
}
document.addEventListener("DOMContentLoaded", boot);
