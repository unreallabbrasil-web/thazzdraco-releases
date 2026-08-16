/* ============================================================================
   Disco — onde o espaço foi parar.
   O diagnóstico já dizia "9% livre em C:" e não oferecia nada. Esta tela é a
   resposta: varre a unidade e mostra, do maior para o menor, o que está
   ocupando — com navegação por pasta e a lista dos maiores arquivos.
   Depende de app.js (carregado antes): $, $$, api, toast, escHtml, IC, busy.
   ========================================================================== */
(function () {
  "use strict";

  const D = { unidade: null, discos: null, poll: null, atual: null, maiores: null, resumo: null,
              achados: null, marcados: new Set(), limpeza: null };

  function gb(bytes) {
    const g = bytes / (1024 ** 3);
    if (g >= 1) return g.toFixed(g >= 100 ? 0 : 1) + " GB";
    const m = bytes / (1024 ** 2);
    if (m >= 1) return m.toFixed(0) + " MB";
    return Math.max(0, Math.round(bytes / 1024)) + " KB";
  }
  function milhares(n) { return (n || 0).toLocaleString("pt-BR"); }

  /* ---- Render --------------------------------------------------------------- */

  async function render() {
    const box = $("#discoBox"); if (!box) return;
    if (!D.discos) {
      box.innerHTML = `<div class="empty">${IC("ring")}<div>Lendo unidades…</div></div>`;
      try { D.discos = (await api("/api/saude")).discos || []; } catch (e) { D.discos = []; }
      if (!D.unidade && D.discos.length) D.unidade = D.discos[0].letra;
    }
    box.innerHTML = topoHTML() + corpoHTML();
    $$("#discoBox [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
  }

  function topoHTML() {
    const chips = (D.discos || []).map((d) => {
      const on = d.letra === D.unidade;
      const critico = d.usado_pct >= 90;
      return `<button class="dk-chip ${on ? "on" : ""} ${critico ? "cheio" : ""}" data-act="unidade" data-letra="${escHtml(d.letra)}">
        <b>${escHtml(d.letra)}</b>
        <span>${d.livre_gb} GB livres de ${d.total_gb}</span>
        <i style="width:${Math.min(100, d.usado_pct)}%"></i>
      </button>`;
    }).join("");
    return `<div class="ptitle">Disco · onde o espaço foi parar</div>
      <p class="page-lead">Varre a unidade e soma o tamanho de cada pasta. Serve para achar em segundos
        os GB que somem — instaladores velhos, cache de jogo, máquinas virtuais, downloads esquecidos.
        <b>Só lê</b>: nada é apagado aqui.</p>
      <div class="dk-chips">${chips || "<span class='ses-note'>Nenhuma unidade fixa encontrada.</span>"}</div>`;
  }

  function corpoHTML() {
    if (D.rodando) return progressoHTML();
    if (!D.atual) {
      return `<div class="dk-acoes">
        <button class="btn-hero" data-act="varrer"><span data-icon="scan"></span> Varrer ${escHtml(D.unidade || "")}</button>
        <span class="ses-note">Um disco cheio leva de alguns segundos a um minuto. Dá para cancelar.</span>
      </div>`;
    }
    return achadosHTML() + navegadorHTML() + maioresHTML() + rodapeHTML();
  }

  /* ---- O que dá para liberar ------------------------------------------------ */

  const GRUPOS = [
    { classe: "limpavel", titulo: "Dá para limpar", sub: "cache que some e volta sozinho — ninguém perde nada" },
    { classe: "coberto", titulo: "Já existe tela para isso", sub: "a Limpeza profunda cuida — não faço em dobro" },
    { classe: "relatorio", titulo: "Decisão sua", sub: "dado que você baixou ou guardou. O app mostra e não apaga" },
    { classe: "intocavel", titulo: "Não mexo nisto", sub: "parece lixo e não é — apagar quebra o sistema" },
  ];

  function achadosHTML() {
    const a = D.achados;
    if (!a || !a.achados || !a.achados.length) return "";
    const grupos = GRUPOS.map((g) => {
      const itens = a.achados.filter((x) => x.classe === g.classe);
      if (!itens.length) return "";
      const dobra = g.classe === "intocavel";
      return `<div class="ac-grupo ${g.classe} ${dobra && !D.abrirIntocavel ? "dobrado" : ""}">
        <div class="ac-head" ${dobra ? `data-act="dobra"` : ""}>
          <b>${g.titulo}</b><span>${g.sub}</span>
          <i class="ac-soma">${gb(itens.reduce((s, x) => s + x.bytes, 0))}</i>
          ${dobra ? `<span class="ac-seta">${D.abrirIntocavel ? "▾" : "▸"}</span>` : ""}
        </div>
        <div class="ac-itens">${itens.map((x) => achadoHTML(x, g.classe)).join("")}</div>
        ${g.classe === "limpavel" ? barraLimpezaHTML() : ""}
      </div>`;
    }).join("");

    const inst = (a.instaladores || []).length ? `
      <div class="ac-grupo relatorio">
        <div class="ac-head"><b>Instaladores grandes em Downloads</b>
          <span>pasta pessoal — o app lista, quem apaga é você</span>
          <i class="ac-soma">${gb(a.instaladores.reduce((s, f) => s + f.bytes, 0))}</i></div>
        <div class="ac-itens">${a.instaladores.map((f) => `
          <div class="ac-item">
            <b class="ac-tam">${gb(f.bytes)}</b>
            <div class="ac-corpo"><b>${escHtml(f.caminho.split("\\").pop())}</b>
              <span>parado há ${f.idade_dias} dias</span></div>
            <button class="mbtn" data-act="abrir" data-caminho="${escHtml(f.caminho)}">Abrir pasta</button>
          </div>`).join("")}</div>
      </div>` : "";

    const dups = (a.duplicados || []).length ? `
      <div class="ac-grupo relatorio">
        <div class="ac-head"><b>Arquivos grandes duplicados</b>
          <span>o app aponta as cópias — qual delas fica é decisão sua</span>
          <i class="ac-soma">${gb(a.duplicados.reduce((s, d) => s + d.desperdicio, 0))} em cópias</i></div>
        <div class="ac-nota">Cuidado: muita cópia é <b>de propósito</b>. Mods de servidor de jogo, por
          exemplo, existem no workshop da Steam e na pasta do servidor ao mesmo tempo — apagar uma
          delas quebra o servidor. O app só aponta; conferir para que serve cada cópia é com você.</div>
        <div class="ac-itens">${a.duplicados.map((d) => `
          <div class="ac-item dup">
            <b class="ac-tam">${gb(d.bytes)}</b>
            <div class="ac-corpo">
              <b>${d.caminhos.length} cópias — sobra ${gb(d.desperdicio)} se ficar uma</b>
              ${d.caminhos.map((c) => `<div class="dup-cam">
                 <code title="${escHtml(c)}">${escHtml(c)}</code>
                 <button class="mbtn" data-act="abrir" data-caminho="${escHtml(c)}">Abrir</button>
               </div>`).join("")}
              <span class="ac-aviso">Conferido: ${escHtml(d.conferido)}. Não é comparação byte a byte —
                antes de apagar, confirme que nenhum programa está usando a cópia que você escolher.</span>
            </div>
          </div>`).join("")}</div>
      </div>` : "";

    return `<div class="ptitle">O que dá para liberar</div>
      ${a.liberavel_ja ? `<div class="ac-topo">
        <div><span>Cache que dá para limpar</span><b>${gb(a.liberavel_ja)}</b></div>
        <p class="ses-note">Marque o que quiser apagar. Nada vem marcado: exclusão é a única coisa no
          app que não tem desfazer, então a escolha é sempre sua.</p>
      </div>` : ""}
      ${resultadoHTML()}
      ${grupos}${dups}${inst}`;
  }

  function achadoHTML(x, classe) {
    const acao = classe === "coberto" && x.atalho
      ? `<button class="mbtn" data-act="ir" data-alvo="${escHtml(x.atalho)}">Abrir</button>`
      : (classe === "relatorio" && x.caminhos && x.caminhos.length)
        ? `<button class="mbtn" data-act="abrir" data-caminho="${escHtml(x.caminhos[0])}">Abrir pasta</button>` : "";
    // Só o que é limpável ganha caixa de seleção — e nada vem marcado: para algo
    // que não tem undo, marcado por padrão não seria consentimento.
    const marca = classe === "limpavel"
      ? `<label class="pl-check ${D.marcados.has(x.id) ? "on" : ""}" data-act="marcar" data-id="${escHtml(x.id)}">${D.marcados.has(x.id) ? IC("check") : ""}</label>`
      : "";
    return `<div class="ac-item ${D.marcados.has(x.id) ? "sel" : ""}">
      ${marca}
      <b class="ac-tam">${gb(x.bytes)}</b>
      <div class="ac-corpo">
        <b>${escHtml(x.nome)}</b>
        <span>${escHtml(x.descricao)}</span>
        ${x.aviso ? `<span class="ac-aviso">${escHtml(x.aviso)}</span>` : ""}
      </div>
      ${acao}
    </div>`;
  }

  // barraLimpezaHTML: o rodapé que soma o escolhido e dispara a exclusão.
  function barraLimpezaHTML() {
    const a = D.achados; if (!a) return "";
    const escolhidos = (a.achados || []).filter((x) => x.classe === "limpavel" && D.marcados.has(x.id));
    const total = escolhidos.reduce((s, x) => s + x.bytes, 0);
    return `<div class="ac-barra ${escolhidos.length ? "ativa" : ""}">
      <div class="ac-barra-num"><span>Escolhido</span><b>${gb(total)}</b></div>
      <span class="ses-note" style="margin:0;flex:1">Apagar <b>não tem desfazer</b> — é a única coisa no
        app sem volta. Estes caches se refazem sozinhos, mas confira a lista antes.</span>
      <button class="btnG" data-act="limpar" ${escolhidos.length ? "" : "disabled"}>
        <span data-icon="broom"></span> Liberar ${gb(total)}</button>
    </div>`;
  }

  function resultadoHTML() {
    const r = D.limpeza; if (!r) return "";
    return `<div class="ac-resultado">
      <div class="ac-res-num"><span>Liberado</span><b>${gb(r.liberado)}</b></div>
      <div class="ac-res-lista">
        ${(r.categorias || []).map((c) => `<div>${escHtml(c.nome)} — <b>${gb(c.bytes)}</b> em ${c.arquivos} arquivos${c.travados ? ` · ${c.travados} em uso, mantidos` : ""}</div>`).join("")}
        ${(r.recusadas || []).map((m) => `<div class="ac-res-recusa">recusado: ${escHtml(m)}</div>`).join("")}
      </div>
      <span class="ses-note" style="margin:0">Registrado em operacoes.log. Varrendo de novo para os
        números não ficarem velhos…</span>
    </div>`;
  }

  function progressoHTML() {
    const p = D.progresso || {};
    return `<div class="ex-run" style="margin:16px 4px 0">
      <div class="ex-run-top"><span class="ses-kicker">Varrendo ${escHtml(D.unidade || "")}</span>
        <b>${gb(p.bytes || 0)}</b></div>
      <div class="ex-atual">${milhares(p.arquivos)} arquivos · ${p.decorrido_s || 0}s</div>
      <div class="sp-acts" style="margin-top:12px"><button class="mbtn" data-act="cancelar">Cancelar</button></div>
    </div>`;
  }

  function navegadorHTML() {
    const a = D.atual;
    const maior = Math.max(1, ...(a.filhos || []).map((f) => f.bytes), a.proprios || 0);
    const linhas = (a.filhos || []).map((f) => `
      <div class="dk-linha ${f.entravel ? "entra" : ""}" ${f.entravel ? `data-act="entrar" data-caminho="${escHtml(f.caminho)}"` : ""}>
        <div class="dk-barra"><i style="width:${Math.round((f.bytes / maior) * 100)}%"></i></div>
        <span class="dk-nome">${escHtml(f.nome)}</span>
        <span class="dk-qtd">${milhares(f.arquivos)} arq.</span>
        <b class="dk-tam">${gb(f.bytes)}</b>
      </div>`).join("");
    const soltos = (a.proprios || 0) > 0
      ? `<div class="dk-linha solto">
           <div class="dk-barra"><i style="width:${Math.round((a.proprios / maior) * 100)}%"></i></div>
           <span class="dk-nome">arquivos direto nesta pasta</span><span class="dk-qtd"></span>
           <b class="dk-tam">${gb(a.proprios)}</b>
         </div>` : "";

    return `<div class="dk-nav">
      <div class="dk-cam">
        ${a.pai ? `<button class="mbtn" data-act="entrar" data-caminho="${escHtml(a.pai)}">↑ subir</button>` : ""}
        <b>${escHtml(a.caminho)}</b>
        <span class="ses-note" style="margin:0">${gb(a.bytes)} · ${milhares(a.arquivos)} arquivos</span>
      </div>
      <div class="dk-lista">${linhas || `<div class="empty">${IC("empty")}<div>Sem subpastas aqui.</div></div>`}${soltos}</div>
      ${a.ocultos ? `<div class="ses-note">+${a.ocultos} pastas menores não listadas</div>` : ""}
    </div>`;
  }

  function maioresHTML() {
    const m = (D.maiores || []).slice(0, 12);
    if (!m.length) return "";
    return `<div class="ptitle">Maiores arquivos da unidade</div>
      <div class="dk-lista">${m.map((f) => `
        <div class="dk-linha arq">
          <b class="dk-tam">${gb(f.bytes)}</b>
          <span class="dk-cam-arq" title="${escHtml(f.caminho)}">${escHtml(f.caminho)}</span>
        </div>`).join("")}</div>`;
  }

  function rodapeHTML() {
    const r = D.resumo || {};
    return `<div class="dk-rodape">
      <span>Varredura de ${escHtml(r.quando || "")} · ${(r.duracao_ms / 1000 || 0).toFixed(1)}s</span>
      <span>${milhares(r.arquivos)} arquivos · ${milhares(r.pastas)} pastas${r.sem_acesso ? ` · ${r.sem_acesso} pastas sem permissão de leitura` : ""}</span>
      <span class="dk-nota">Tamanho lógico dos arquivos. Junções e links não são seguidos — senão a mesma
        pasta seria contada duas vezes.</span>
      <button class="mbtn" data-act="varrer">Varrer de novo</button>
    </div>`;
  }

  /* ---- Ações ---------------------------------------------------------------- */

  async function varrer() {
    D.atual = D.maiores = D.resumo = null;
    D.rodando = true; D.progresso = {}; render();
    try {
      await api("/api/disco/varrer", { raiz: D.unidade + "\\" });
      poll();
    } catch (e) { D.rodando = false; render(); toast("err", "Não deu para varrer", e.message); }
  }

  function poll() {
    if (D.poll) return;
    D.poll = setInterval(async () => {
      try {
        const st = await api("/api/disco/status");
        if (st.estado === "varrendo") { D.progresso = st; render(); return; }
        pararPoll();
        D.rodando = false;
        if (st.estado === "pronto") {
          D.resumo = st.resumo || {};
          D.maiores = D.resumo.maiores || [];
          try { D.achados = await api("/api/disco/achados"); } catch (e) { D.achados = null; }
          await entrar(D.unidade + "\\");
          toast("ok", "Varredura pronta", `${gb(D.resumo.bytes)} em ${milhares(D.resumo.arquivos)} arquivos.`);
        } else { render(); }
      } catch (e) { /* servidor ocupado; tenta de novo */ }
    }, 900);
  }
  function pararPoll() { if (D.poll) { clearInterval(D.poll); D.poll = null; } }

  async function entrar(caminho) {
    try {
      D.atual = await api("/api/disco/arvore?caminho=" + encodeURIComponent(caminho) + "&limite=40");
      render();
      const l = $("#discoBox .dk-lista"); if (l) l.scrollTop = 0;
    } catch (e) { toast("err", "Não abriu a pasta", e.message); }
  }

  // pedirLimpeza mostra EXATAMENTE o que vai sumir antes de apagar. Sem undo, a
  // confirmação é a única chance de o técnico perceber que marcou errado.
  function pedirLimpeza() {
    const escolhidos = (D.achados.achados || []).filter((x) => x.classe === "limpavel" && D.marcados.has(x.id));
    if (!escolhidos.length) return;
    const total = escolhidos.reduce((s, x) => s + x.bytes, 0);
    confirmModal({
      title: "Apagar " + gb(total) + "?",
      body: `<p>Vai apagar o conteúdo destas pastas:</p>
        <ul style="margin:12px 0 0 18px;display:flex;flex-direction:column;gap:8px">
          ${escolhidos.map((x) => `<li><b>${escHtml(x.nome)}</b> — ${gb(x.bytes)}<br>
             <span style="color:var(--ink-3);font-size:12px">${escHtml(x.descricao)}</span>
             ${(x.caminhos || []).map((c) => `<br><code style="font-size:10.5px;color:var(--ink-4)">${escHtml(c)}</code>`).join("")}
           </li>`).join("")}
        </ul>
        <div class="warn-box">${IC("warn")}<div><b>Isto não tem desfazer.</b> O motor de otimização
          guarda snapshot e desfaz; apagar arquivo, não. Arquivo em uso é mantido, não forçado.</div></div>`,
      okLabel: "Apagar " + gb(total), danger: true,
      onOk: () => limpar(escolhidos.map((x) => x.id)),
    });
  }

  async function limpar(ids) {
    busy(true, "Apagando…");
    try {
      D.limpeza = await api("/api/disco/limpar", { ids, confirmar: true });
      D.marcados.clear();
      toast(D.limpeza.liberado ? "ok" : "info", gb(D.limpeza.liberado) + " liberados",
        D.limpeza.total_travado ? `${D.limpeza.total_travado} arquivos em uso foram mantidos.` : "Registrado no log de operações.");
      render();
      await varrer(); // os números da tela não podem ficar velhos depois de apagar
    } catch (e) { toast("err", "Falha ao limpar", e.message); }
    finally { busy(false); }
  }

  async function abrirPasta(caminho) {
    try { await api("/api/disco/abrir", { caminho }); }
    catch (e) { toast("err", "Não abriu", e.message); }
  }

  async function cancelar() {
    try { await api("/api/disco/cancelar", {}); } catch (e) {}
    pararPoll(); D.rodando = false; render();
    toast("info", "Varredura cancelada", "");
  }

  /* ---- Eventos e boot -------------------------------------------------------- */

  function wire() {
    const box = $("#discoBox"); if (!box || box.dataset.wired) return;
    box.dataset.wired = "1";
    box.onclick = (ev) => {
      const el = ev.target.closest("[data-act]"); if (!el) return;
      const act = el.dataset.act;
      if (act === "unidade") { D.unidade = el.dataset.letra; D.atual = D.maiores = D.resumo = D.achados = D.limpeza = null; D.marcados.clear(); render(); return; }
      if (act === "varrer") return varrer();
      if (act === "cancelar") return cancelar();
      if (act === "entrar") return entrar(el.dataset.caminho);
      if (act === "dobra") { D.abrirIntocavel = !D.abrirIntocavel; render(); return; }
      if (act === "marcar") { const id = el.dataset.id; D.marcados.has(id) ? D.marcados.delete(id) : D.marcados.add(id); render(); return; }
      if (act === "limpar") return pedirLimpeza();
      if (act === "ir") return navTo(el.dataset.alvo);
      if (act === "abrir") return abrirPasta(el.dataset.caminho);
    };
  }

  window.DISCO = {
    abrir() { wire(); render(); },
    parar: pararPoll,
  };
})();
