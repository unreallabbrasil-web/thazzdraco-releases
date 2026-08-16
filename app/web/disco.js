/* ============================================================================
   Disco — onde o espaço foi parar, e o que fazer com ele.

   A tela responde três perguntas, nesta ordem, porque é a ordem em que o
   técnico pensa:
     1. onde está o espaço?      -> árvore de pastas, do maior para o menor
     2. o que é isso?            -> por tipo de conteúdo, e o que está parado
     3. o que eu faço com isso?  -> cesta -> Lixeira

   Depende de app.js (carregado antes): $, $$, api, toast, escHtml, IC, busy,
   confirmModal, navTo.
   ========================================================================== */
(function () {
  "use strict";

  const D = {
    unidades: null, alvo: null, rodando: false, progresso: null,
    resumo: null, atual: null, maiores: null, antigos: null, familias: null,
    achados: null, marcados: new Set(), limpeza: null,
    cesta: new Map(),      // caminho -> {nome, bytes, pasta}
    aba: "pastas", poll: null, modoExclusao: "lixeira", exclusao: null,
    sumidos: new Set(),    // o que já foi apagado nesta sessão de tela
  };

  function gb(bytes) {
    const g = bytes / (1024 ** 3);
    if (g >= 1) return g.toFixed(g >= 100 ? 0 : 1) + " GB";
    const m = bytes / (1024 ** 2);
    if (m >= 1) return m.toFixed(0) + " MB";
    return Math.max(0, Math.round(bytes / 1024)) + " KB";
  }
  function milhares(n) { return (n || 0).toLocaleString("pt-BR"); }
  function nomeDe(caminho) { return String(caminho || "").split("\\").filter(Boolean).pop() || caminho; }

  // idade: "há 3 anos" diz mais que "1274 dias" na hora de decidir se apaga.
  function idade(dias) {
    if (dias == null) return "";
    if (dias < 45) return `há ${dias} dia${dias === 1 ? "" : "s"}`;
    if (dias < 365) return `há ${Math.round(dias / 30)} meses`;
    const a = dias / 365;
    return a < 2 ? "há mais de 1 ano" : `há ${Math.floor(a)} anos`;
  }

  /* ---- Render --------------------------------------------------------------- */

  async function render() {
    const box = $("#discoBox"); if (!box) return;
    if (!D.unidades) {
      box.innerHTML = `<div class="empty">${IC("ring")}<div>Lendo unidades…</div></div>`;
      try { D.unidades = (await api("/api/disco/unidades")).unidades || []; }
      catch (e) { D.unidades = []; }
      if (!D.alvo) {
        const padrao = D.unidades.find((u) => u.sistema && u.pronta) || D.unidades.find((u) => u.pronta);
        D.alvo = padrao ? padrao.letra + "\\" : "";
      }
    }
    box.innerHTML = topoHTML() + corpoHTML() + cestaHTML();
    $$("#discoBox [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
  }

  /* ---- Escolha do que varrer ------------------------------------------------ */

  const TIPOS = {
    fixo: { rot: "Disco interno" },
    removivel: { rot: "Removível" },
    rede: { rot: "Rede" },
    cd: { rot: "Disco óptico" },
    ram: { rot: "Disco em RAM" },
  };

  function topoHTML() {
    const us = D.unidades || [];
    const chips = us.map((u) => {
      const alvoU = (D.alvo || "").toUpperCase().startsWith(u.letra.toUpperCase());
      const t = TIPOS[u.tipo] || TIPOS.fixo;
      const critico = u.pronta && u.usado_pct >= 90;
      const nome = u.rotulo ? `${u.rotulo} (${u.letra})` : u.letra;
      if (!u.pronta) {
        return `<div class="dk-chip vazia" title="${escHtml(u.aviso || "")}">
          <b>${escHtml(nome)}</b><span>${escHtml(u.aviso || "sem mídia")}</span></div>`;
      }
      return `<button class="dk-chip ${alvoU ? "on" : ""} ${critico ? "cheio" : ""}"
          data-act="unidade" data-raiz="${escHtml(u.letra)}\\" title="${escHtml(t.rot)}${u.fs ? " · " + u.fs : ""}">
        <b>${escHtml(nome)}</b>
        <span>${u.livre_gb} de ${u.total_gb} GB livres</span>
        <i style="width:${Math.min(100, u.usado_pct)}%"></i>
        <em class="dk-tipo">${escHtml(t.rot)}${u.sistema ? " · Windows" : ""}</em>
      </button>`;
    }).join("");

    const custom = !!(D.alvo && D.alvo.length > 3);
    return `<div class="ptitle">Disco · onde o espaço foi parar</div>
      <p class="page-lead">Varre e soma o tamanho de cada pasta, para achar em segundos os GB que somem.
        A varredura <b>só lê</b>. Apagar só acontece quando você escolhe item por item e confirma.</p>
      <div class="dk-chips">${chips || "<span class='ses-note'>Nenhuma unidade encontrada.</span>"}</div>
      <div class="dk-pasta ${custom ? "on" : ""}">
        <span data-icon="disk"></span>
        <input id="dkPasta" placeholder="…ou varra só uma pasta:  E:\\Jogos" value="${custom ? escHtml(D.alvo) : ""}"
               spellcheck="false" autocomplete="off">
        <button class="mbtn" data-act="usar-pasta">Usar esta pasta</button>
      </div>`;
  }

  function corpoHTML() {
    if (D.rodando) return progressoHTML();
    if (!D.resumo) {
      return `<div class="dk-acoes">
        <button class="btn-hero" data-act="varrer" ${D.alvo ? "" : "disabled"}>
          <span data-icon="scan"></span> Varrer ${escHtml(D.alvo || "")}</button>
        <span class="ses-note">Um disco cheio leva de alguns segundos a alguns minutos. Dá para cancelar.</span>
      </div>`;
    }
    return resumoHTML() + abasHTML() + abaAtualHTML() + rodapeHTML();
  }

  function progressoHTML() {
    const p = D.progresso || {};
    return `<div class="ex-run" style="margin:16px 4px 0">
      <div class="ex-run-top"><span class="ses-kicker">Varrendo ${escHtml(D.alvo || "")}</span>
        <b>${gb(p.bytes || 0)}</b></div>
      <div class="ex-atual">${milhares(p.arquivos)} arquivos · ${p.decorrido_s || 0}s</div>
      <div class="sp-acts" style="margin-top:12px"><button class="mbtn" data-act="cancelar">Cancelar</button></div>
    </div>`;
  }

  function resumoHTML() {
    const r = D.resumo || {};
    const lib = D.achados ? D.achados.liberavel_ja : 0;
    const antigosBytes = (D.antigos || []).reduce((s, f) => s + f.bytes, 0);
    // Cartão que leva a uma aba é botão de verdade; o que só informa é <div>.
    // Um deles navega, o outro não — a diferença tem que existir para o teclado
    // também, não só para o mouse.
    const cartao = (rot, val, sub, act) => act
      ? `<button type="button" class="dk-card" data-act="aba" data-aba="${act}">
           <span>${rot}</span><b>${val}</b><em>${sub}</em></button>`
      : `<div class="dk-card"><span>${rot}</span><b>${val}</b><em>${sub}</em></div>`;
    return `<div class="dk-cards">
      ${cartao("Varrido", gb(r.bytes), `${milhares(r.arquivos)} arquivos`, "")}
      ${cartao("Parado há +1 ano", gb(antigosBytes), `${(D.antigos || []).length} arquivos grandes`, "parados")}
      ${cartao("Maior tipo", (D.familias || [])[0] ? gb(D.familias[0].bytes) : "—",
              (D.familias || [])[0] ? D.familias[0].nome : "sem dados", "tipos")}
      ${cartao("Cache limpável", gb(lib || 0), "sem perder nada", "limpeza")}
    </div>`;
  }

  const ABAS = [
    { id: "pastas", rot: "Pastas" },
    { id: "tipos", rot: "Por tipo" },
    { id: "parados", rot: "Parados" },
    { id: "maiores", rot: "Maiores arquivos" },
    { id: "limpeza", rot: "Cache limpável" },
  ];

  function abasHTML() {
    return `<div class="dk-abas" role="tablist" aria-label="Visões da varredura">${ABAS.map((a) =>
      `<button type="button" role="tab" aria-selected="${D.aba === a.id}"
        class="dk-aba ${D.aba === a.id ? "on" : ""}" data-act="aba" data-aba="${a.id}">${a.rot}</button>`
    ).join("")}</div>`;
  }

  function abaAtualHTML() {
    switch (D.aba) {
      case "tipos": return tiposHTML();
      case "parados": return listaArquivosHTML(D.antigos, "Grandes e parados",
        "Arquivos com mais de 100 MB que ninguém abre há mais de um ano. É o lugar mais provável de sobrar espaço sem fazer falta — mas confira: backup e coleção também ficam parados de propósito.");
      case "maiores": return listaArquivosHTML(D.maiores, "Maiores arquivos da varredura",
        "Os maiores, independente de idade.");
      case "limpeza": return achadosHTML();
      default: return navegadorHTML();
    }
  }

  /* ---- Aba: árvore de pastas ------------------------------------------------- */

  function navegadorHTML() {
    const a = D.atual;
    if (!a) return `<div class="empty">${IC("empty")}<div>Sem pasta aberta.</div></div>`;
    const maior = Math.max(1, ...(a.filhos || []).map((f) => f.bytes), a.proprios || 0);
    const linhas = (a.filhos || []).map((f) => {
      if (D.sumidos.has(f.caminho.toLowerCase())) return linhaSumida(f.nome);
      return `<div class="dk-linha ${f.entravel ? "entra" : ""} ${naCesta(f.caminho) ? "sel" : ""} ${f.protegido ? "prot" : ""}">
        ${marcaHTML(f, f.nome)}
        <div class="dk-barra" ${f.entravel ? `data-act="entrar" data-caminho="${escHtml(f.caminho)}"` : ""}>
          <i style="width:${Math.round((f.bytes / maior) * 100)}%"></i></div>
        <span class="dk-nome" ${f.entravel ? `data-act="entrar" data-caminho="${escHtml(f.caminho)}"` : ""}>${escHtml(f.nome)}</span>
        <span class="dk-qtd">${milhares(f.arquivos)} arq.</span>
        <b class="dk-tam">${gb(f.bytes)}</b>
        ${abrirBtn(f.caminho, f.nome)}
      </div>`;
    }).join("");

    const soltos = (a.proprios || 0) > 0
      ? `<div class="dk-linha solto">
           <span class="dk-check-vazio"></span>
           <div class="dk-barra"><i style="width:${Math.round((a.proprios / maior) * 100)}%"></i></div>
           <span class="dk-nome">arquivos direto nesta pasta</span><span class="dk-qtd"></span>
           <b class="dk-tam">${gb(a.proprios)}</b><span class="dk-abrir"></span>
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

  function linhaSumida(nome) {
    return `<div class="dk-linha sumida"><span class="dk-check-vazio"></span><div class="dk-barra"></div>
      <span class="dk-nome">${escHtml(nome)}</span><span class="dk-qtd">apagado agora</span>
      <b class="dk-tam">—</b><span class="dk-abrir"></span></div>`;
  }

  // caixaHTML: a marca de seleção. Nada vem marcado, nunca — para algo que
  // apaga arquivo, marcado por padrão não seria consentimento, seria armadilha.
  //
  // É <button role="checkbox">, não <label>: esta caixa é o controle que decide
  // o que vai ser apagado, e um <div>/<label> sem input não recebe foco, não
  // responde a teclado e não anuncia estado nenhum para leitor de tela.
  function caixaHTML(caminho, nome, bytes, pasta) {
    const on = naCesta(caminho);
    return `<button type="button" class="pl-check dk-check ${on ? "on" : ""}" role="checkbox"
      aria-checked="${on}" aria-label="Selecionar ${escHtml(nome)} para apagar" data-act="cesta"
      data-caminho="${escHtml(caminho)}" data-nome="${escHtml(nome)}" data-bytes="${bytes}"
      data-pasta="${pasta ? 1 : 0}">${on ? IC("check") : ""}</button>`;
  }

  function naCesta(caminho) { return D.cesta.has(String(caminho).toLowerCase()); }

  // marcaHTML: caixa de seleção OU cadeado, decidido pelo backend.
  //
  // O que o portão de exclusão recusa não ganha caixa — antes dava para marcar
  // C:\Windows, ver o total subir, confirmar, e só então receber "recusado".
  // Oferecer uma escolha que nunca vai ser aceita é pior que não oferecer.
  function marcaHTML(it, nome) {
    if (it.protegido) {
      return `<span class="pl-check trava" role="img" aria-label="Protegido: ${escHtml(it.motivo || "o app não apaga isto")}"
        title="${escHtml(it.motivo || "o app não apaga isto")}"></span>`;
    }
    return caixaHTML(it.caminho, nome || nomeDe(it.caminho), it.bytes, !!it.entravel);
  }

  // Botão só de ícone precisa de nome acessível — o `title` aparece no hover do
  // mouse e não existe para quem navega por teclado ou leitor de tela.
  function abrirBtn(caminho, nome) {
    return `<button type="button" class="dk-abrir" data-act="abrir" data-caminho="${escHtml(caminho)}"
      aria-label="Abrir ${escHtml(nome)} no Explorer" title="Abrir no Explorer">${IC("ext")}</button>`;
  }

  /* ---- Aba: por tipo --------------------------------------------------------- */

  function tiposHTML() {
    const fs = D.familias || [];
    if (!fs.length) return `<div class="empty">${IC("empty")}<div>Sem dados de tipo nesta varredura.</div></div>`;
    const maior = Math.max(1, ...fs.map((f) => f.bytes));
    return `<div class="ptitle">O que está ocupando, por tipo</div>
      <p class="page-lead">A árvore diz <b>onde</b>. Isto diz <b>o quê</b> — que é como se decide o que apagar.
        O tipo vem da extensão do arquivo, então é uma pista muito boa e não um veredito.</p>
      <div class="dk-fams">${fs.map((f) => `
        <div class="dk-fam">
          <div class="dk-fam-top">
            <b>${escHtml(f.nome)}</b>
            <span>${milhares(f.arquivos)} arquivos</span>
            <i>${gb(f.bytes)}</i>
          </div>
          <div class="dk-fbar"><i style="width:${Math.round((f.bytes / maior) * 100)}%"></i></div>
          ${(f.exemplos || []).length ? `<div class="dk-fam-ex">${f.exemplos.map((e) => `
            <div class="dk-ex">
              ${marcaHTML(e)}
              <b>${gb(e.bytes)}</b>
              <code title="${escHtml(e.caminho)}">${escHtml(e.caminho)}</code>
              <span>${e.protegido ? escHtml(e.motivo || "protegido") : idade(e.dias)}</span>
            </div>`).join("")}</div>` : ""}
        </div>`).join("")}</div>`;
  }

  /* ---- Abas: listas de arquivos ---------------------------------------------- */

  function listaArquivosHTML(itens, titulo, lead) {
    const lista = (itens || []).filter((f) => !D.sumidos.has(f.caminho.toLowerCase()));
    if (!lista.length) {
      return `<div class="ptitle">${escHtml(titulo)}</div>
        <div class="empty">${IC("empty")}<div>Nada aqui — o que já é uma boa notícia.</div></div>`;
    }
    const total = lista.reduce((s, f) => s + f.bytes, 0);
    return `<div class="ptitle">${escHtml(titulo)}</div>
      <p class="page-lead">${escHtml(lead)}</p>
      <div class="dk-sel-tudo">
        <b>${gb(total)}</b> em ${lista.length} arquivos
        <button class="mbtn" data-act="marcar-lista">Selecionar todos</button>
        <button class="mbtn" data-act="limpar-cesta">Limpar seleção</button>
      </div>
      <div class="dk-lista">${lista.slice(0, 200).map((f) => `
        <div class="dk-linha arq ${naCesta(f.caminho) ? "sel" : ""} ${f.protegido ? "prot" : ""}">
          ${marcaHTML(f)}
          <b class="dk-tam">${gb(f.bytes)}</b>
          <span class="dk-cam-arq" title="${escHtml(f.caminho)}">${escHtml(f.caminho)}</span>
          <span class="dk-idade">${f.protegido ? escHtml(f.motivo || "protegido") : idade(f.dias)}</span>
          ${abrirBtn(f.caminho, nomeDe(f.caminho))}
        </div>`).join("")}</div>`;
  }

  /* ---- Aba: cache limpável (classificação por categoria) ---------------------- */

  const GRUPOS = [
    { classe: "limpavel", titulo: "Dá para limpar", sub: "cache que some e volta sozinho — ninguém perde nada" },
    { classe: "coberto", titulo: "Já existe tela para isso", sub: "a Limpeza profunda cuida — não faço em dobro" },
    { classe: "relatorio", titulo: "Decisão sua", sub: "dado que você baixou ou guardou. O app mostra e não apaga" },
    { classe: "intocavel", titulo: "Não mexo nisto", sub: "parece lixo e não é — apagar quebra o sistema" },
  ];

  function achadosHTML() {
    const a = D.achados;
    if (!a || !a.achados || !a.achados.length) {
      return `<div class="empty">${IC("empty")}<div>Sem cache classificado nesta varredura.</div></div>`;
    }
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
                 ${caixaHTML(c, nomeDe(c), d.bytes, false)}
                 <code title="${escHtml(c)}">${escHtml(c)}</code>
                 <button class="mbtn" data-act="abrir" data-caminho="${escHtml(c)}">Abrir</button>
               </div>`).join("")}
              <span class="ac-aviso">Conferido: ${escHtml(d.conferido)}. Não é comparação byte a byte —
                antes de apagar, confirme que nenhum programa está usando a cópia que você escolher.</span>
            </div>
          </div>`).join("")}</div>
      </div>` : "";

    const inst = (a.instaladores || []).length ? `
      <div class="ac-grupo relatorio">
        <div class="ac-head"><b>Instaladores grandes em Downloads</b>
          <span>já instalados, quase sempre não servem mais para nada</span>
          <i class="ac-soma">${gb(a.instaladores.reduce((s, f) => s + f.bytes, 0))}</i></div>
        <div class="ac-itens">${a.instaladores.map((f) => `
          <div class="ac-item ${naCesta(f.caminho) ? "sel" : ""}">
            ${caixaHTML(f.caminho, nomeDe(f.caminho), f.bytes, false)}
            <b class="ac-tam">${gb(f.bytes)}</b>
            <div class="ac-corpo"><b>${escHtml(nomeDe(f.caminho))}</b>
              <span>parado ${idade(f.idade_dias)}</span></div>
            <button class="mbtn" data-act="abrir" data-caminho="${escHtml(f.caminho)}">Abrir pasta</button>
          </div>`).join("")}</div>
      </div>` : "";

    return `<div class="ptitle">Cache que dá para limpar</div>
      ${a.liberavel_ja ? `<div class="ac-topo">
        <div><span>Cache regenerável</span><b>${gb(a.liberavel_ja)}</b></div>
        <p class="ses-note">Estas categorias o app apaga de vez, porque se refazem sozinhas. Nada vem
          marcado. Para apagar <b>arquivo seu</b>, use a seleção das outras abas — aquilo vai para a Lixeira.</p>
      </div>` : ""}
      ${resultadoHTML()}${grupos}${dups}${inst}`;
  }

  function achadoHTML(x, classe) {
    const acao = classe === "coberto" && x.atalho
      ? `<button class="mbtn" data-act="ir" data-alvo="${escHtml(x.atalho)}">Abrir</button>`
      : (classe === "relatorio" && x.caminhos && x.caminhos.length)
        ? `<button class="mbtn" data-act="abrir" data-caminho="${escHtml(x.caminhos[0])}">Abrir pasta</button>` : "";
    const on = D.marcados.has(x.id);
    const marca = classe === "limpavel"
      ? `<button type="button" class="pl-check" role="checkbox" aria-checked="${on}"
          aria-label="Selecionar ${escHtml(x.nome)} para limpar" data-act="marcar"
          data-id="${escHtml(x.id)}">${on ? IC("check") : ""}</button>`
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

  function barraLimpezaHTML() {
    const a = D.achados; if (!a) return "";
    const escolhidos = (a.achados || []).filter((x) => x.classe === "limpavel" && D.marcados.has(x.id));
    const total = escolhidos.reduce((s, x) => s + x.bytes, 0);
    return `<div class="ac-barra ${escolhidos.length ? "ativa" : ""}">
      <div class="ac-barra-num"><span>Escolhido</span><b>${gb(total)}</b></div>
      <span class="ses-note" style="margin:0;flex:1">Cache <b>não vai para a Lixeira</b>: é apagado de vez,
        porque se refaz sozinho. Confira a lista antes.</span>
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
      <span class="ses-note" style="margin:0">Registrado em operacoes.log.</span>
    </div>`;
  }

  /* ---- A cesta: o que vai ser apagado ---------------------------------------- */

  function cestaHTML() {
    if (!D.cesta.size) return exclusaoHTML();
    let total = 0; D.cesta.forEach((v) => { total += v.bytes || 0; });
    return exclusaoHTML() + `<div class="dk-cesta">
      <div class="dk-cesta-num"><span>Selecionado</span><b>${gb(total)}</b></div>
      <span class="dk-cesta-txt">${D.cesta.size} ${D.cesta.size === 1 ? "item" : "itens"} · vai para a
        <b>Lixeira</b>, dá para restaurar pelo Windows</span>
      <button class="mbtn" data-act="limpar-cesta">Limpar</button>
      <button class="btnG danger" data-act="excluir"><span data-icon="trash"></span> Apagar ${gb(total)}</button>
    </div>`;
  }

  function exclusaoHTML() {
    const r = D.exclusao; if (!r) return "";
    const ok = (r.itens || []).filter((i) => i.ok);
    const falhou = (r.itens || []).filter((i) => !i.ok);
    return `<div class="dk-exres">
      <div class="dk-exres-num"><span>${r.para_lixeira ? "Para a Lixeira" : "Apagado de vez"}</span><b>${gb(r.liberado)}</b></div>
      <div class="dk-exres-lista">
        ${ok.length ? `<div>${ok.length} ${ok.length === 1 ? "item foi" : "itens foram"} removidos${r.para_lixeira ? " — restaure pela Lixeira do Windows se precisar" : ""}.</div>` : ""}
        ${falhou.map((i) => `<div class="ac-res-recusa">${escHtml(i.nome)}: ${escHtml(i.erro || "não saiu")}</div>`).join("")}
        ${(r.recusados || []).map((m) => `<div class="ac-res-recusa">recusado: ${escHtml(m)}</div>`).join("")}
      </div>
      <span class="ses-note" style="margin:0">Registrado em operacoes.log. Os números da tela ainda são da
        varredura anterior — varra de novo para atualizar.</span>
      <button class="mbtn" data-act="fechar-exres">Ok</button>
    </div>`;
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
    D.atual = D.maiores = D.resumo = D.antigos = D.familias = D.achados = null;
    D.limpeza = D.exclusao = null; D.marcados.clear(); D.cesta.clear(); D.sumidos.clear();
    D.rodando = true; D.progresso = {}; render();
    try {
      await api("/api/disco/varrer", { raiz: D.alvo });
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
          D.antigos = D.resumo.antigos || [];
          D.familias = D.resumo.familias || [];
          try { D.achados = await api("/api/disco/achados"); } catch (e) { D.achados = null; }
          await entrar(D.alvo);
          toast("ok", "Varredura pronta", `${gb(D.resumo.bytes)} em ${milhares(D.resumo.arquivos)} arquivos.`);
        } else { render(); }
      } catch (e) { /* servidor ocupado; tenta de novo */ }
    }, 900);
  }
  function pararPoll() { if (D.poll) { clearInterval(D.poll); D.poll = null; } }

  async function entrar(caminho) {
    try {
      D.atual = await api("/api/disco/arvore?caminho=" + encodeURIComponent(caminho) + "&limite=40");
      D.aba = "pastas";
      render();
      const l = $("#discoBox .dk-lista"); if (l) l.scrollTop = 0;
    } catch (e) { toast("err", "Não abriu a pasta", e.message); }
  }

  function alternarCesta(el) {
    const caminho = el.dataset.caminho;
    const chave = caminho.toLowerCase();
    if (D.cesta.has(chave)) D.cesta.delete(chave);
    else D.cesta.set(chave, {
      caminho, nome: el.dataset.nome || nomeDe(caminho),
      bytes: Number(el.dataset.bytes) || 0, pasta: el.dataset.pasta === "1",
    });
    render();
  }

  function marcarListaVisivel() {
    const lista = D.aba === "parados" ? D.antigos : D.maiores;
    (lista || []).filter((f) => !f.protegido && !D.sumidos.has(f.caminho.toLowerCase())).slice(0, 200).forEach((f) => {
      D.cesta.set(f.caminho.toLowerCase(), { caminho: f.caminho, nome: nomeDe(f.caminho), bytes: f.bytes, pasta: false });
    });
    render();
  }

  // pedirExclusao: a última chance de perceber que marcou errado. Mostra item por
  // item, com tamanho, e deixa a escolha Lixeira/definitivo explícita — porque um
  // botão que às vezes tem desfazer e às vezes não seria pior que nenhum.
  function pedirExclusao() {
    if (!D.cesta.size) return;
    const itens = [...D.cesta.values()].sort((a, b) => b.bytes - a.bytes);
    const total = itens.reduce((s, i) => s + i.bytes, 0);
    D.modoExclusao = "lixeira";

    confirmModal({
      title: `Apagar ${itens.length} ${itens.length === 1 ? "item" : "itens"} · ${gb(total)}?`,
      body: `<ul class="dk-conf">
          ${itens.slice(0, 40).map((i) => `<li>
            <b>${gb(i.bytes)}</b>
            <div><span>${escHtml(i.nome)}</span><code>${escHtml(i.caminho)}</code></div>
            ${i.pasta ? `<em>pasta inteira</em>` : ""}
          </li>`).join("")}
          ${itens.length > 40 ? `<li class="dk-conf-mais">…e mais ${itens.length - 40} itens</li>` : ""}
        </ul>
        <div class="dk-modo">
          <label class="dk-modo-op on">
            <input type="radio" name="modo" value="lixeira" checked>
            <div><b>Mandar para a Lixeira</b>
              <span>Dá para restaurar pelo Windows. É o padrão, e é o que você quer em 99% dos casos.</span></div>
          </label>
          <label class="dk-modo-op">
            <input type="radio" name="modo" value="definitivo">
            <div><b>Apagar de vez</b>
              <span>Não passa pela Lixeira e <b>não tem desfazer</b>. Útil só para arquivo gigante, que não
                caberia na Lixeira de qualquer jeito.</span></div>
          </label>
        </div>
        <div class="warn-box">${IC("warn")}<div>O app <b>não apaga</b> pasta do Windows, Programas,
          recuperação, nem perfil inteiro — mesmo que esteja na lista. Item em uso por um programa aberto
          também fica, e aparece no resultado.</div></div>`,
      okLabel: rotuloBotao(total, "lixeira"), danger: true,
      onOk: () => excluir(itens.map((i) => i.caminho)),
    });

    const mb = $("#mBody");
    if (mb) mb.onchange = (ev) => {
      if (ev.target.name !== "modo") return;
      D.modoExclusao = ev.target.value;
      $$("#mBody .dk-modo-op").forEach((l) => l.classList.toggle("on", l.contains(ev.target)));
      const ok = $("#mOk");
      if (ok) ok.textContent = rotuloBotao(total, D.modoExclusao);
    };
  }

  // O botão diz o tamanho E o destino, nas duas opções. Antes ele começava como
  // "Apagar 41 GB" (sem dizer para onde ia) e ao trocar de modo perdia o
  // tamanho — o rótulo mudava de assunto justo no clique que não tem volta.
  function rotuloBotao(total, modo) {
    return modo === "lixeira"
      ? "Mandar " + gb(total) + " para a Lixeira"
      : "Apagar " + gb(total) + " de vez";
  }

  async function excluir(caminhos) {
    busy(true, D.modoExclusao === "lixeira" ? "Mandando para a Lixeira…" : "Apagando…");
    try {
      const r = await api("/api/disco/excluir", {
        caminhos, para_lixeira: D.modoExclusao === "lixeira", confirmar: true,
      });
      D.exclusao = r;
      (r.itens || []).forEach((i) => { if (i.ok) D.sumidos.add(i.caminho.toLowerCase()); });
      D.cesta.clear();
      const falhas = (r.itens || []).filter((i) => !i.ok).length + (r.recusados || []).length;
      toast(r.liberado ? "ok" : "info", gb(r.liberado) + " liberados",
        falhas ? `${falhas} não saíram — veja o detalhe na tela.` : (r.para_lixeira ? "Estão na Lixeira do Windows." : "Registrado no log."));
      render();
    } catch (e) { toast("err", "Falha ao apagar", e.message); }
    finally { busy(false); }
  }

  function pedirLimpeza() {
    const escolhidos = (D.achados.achados || []).filter((x) => x.classe === "limpavel" && D.marcados.has(x.id));
    if (!escolhidos.length) return;
    const total = escolhidos.reduce((s, x) => s + x.bytes, 0);
    confirmModal({
      title: "Apagar " + gb(total) + " de cache?",
      body: `<p>Vai apagar o conteúdo destas pastas:</p>
        <ul style="margin:12px 0 0 18px;display:flex;flex-direction:column;gap:8px">
          ${escolhidos.map((x) => `<li><b>${escHtml(x.nome)}</b> — ${gb(x.bytes)}<br>
             <span style="color:var(--ink-3);font-size:12px">${escHtml(x.descricao)}</span>
             ${(x.caminhos || []).map((c) => `<br><code style="font-size:10.5px;color:var(--ink-4)">${escHtml(c)}</code>`).join("")}
           </li>`).join("")}
        </ul>
        <div class="warn-box">${IC("warn")}<div>Cache é apagado <b>de vez</b>, sem passar pela Lixeira —
          mandar gigabytes de cache para lá só mudaria o lixo de lugar. Estes caches se refazem sozinhos.
          Arquivo em uso é mantido, não forçado.</div></div>`,
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
      await varrer();
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
      if (act === "unidade") {
        D.alvo = el.dataset.raiz;
        D.resumo = D.atual = D.achados = D.limpeza = D.exclusao = null;
        D.marcados.clear(); D.cesta.clear(); D.sumidos.clear();
        render(); return;
      }
      if (act === "usar-pasta") {
        const v = ($("#dkPasta") || {}).value || "";
        if (!v.trim()) { toast("info", "Digite o caminho da pasta", "Exemplo: E:\\Jogos"); return; }
        D.alvo = v.trim();
        D.resumo = D.atual = D.achados = D.limpeza = D.exclusao = null;
        D.cesta.clear(); D.sumidos.clear();
        return varrer();
      }
      if (act === "aba") { D.aba = el.dataset.aba; render(); return; }
      if (act === "varrer") return varrer();
      if (act === "cancelar") return cancelar();
      if (act === "entrar") return entrar(el.dataset.caminho);
      if (act === "cesta") return alternarCesta(el);
      if (act === "marcar-lista") return marcarListaVisivel();
      if (act === "limpar-cesta") { D.cesta.clear(); render(); return; }
      if (act === "excluir") return pedirExclusao();
      if (act === "fechar-exres") { D.exclusao = null; render(); return; }
      if (act === "dobra") { D.abrirIntocavel = !D.abrirIntocavel; render(); return; }
      if (act === "marcar") { const id = el.dataset.id; D.marcados.has(id) ? D.marcados.delete(id) : D.marcados.add(id); render(); return; }
      if (act === "limpar") return pedirLimpeza();
      if (act === "ir") return navTo(el.dataset.alvo);
      if (act === "abrir") return abrirPasta(el.dataset.caminho);
    };
    // Enter no campo de pasta faz o óbvio.
    box.onkeydown = (ev) => {
      if (ev.key !== "Enter" || ev.target.id !== "dkPasta") return;
      ev.preventDefault();
      const b = $("#discoBox [data-act='usar-pasta']"); if (b) b.click();
    };
  }

  window.DISCO = {
    abrir() { wire(); render(); },
    parar: pararPoll,
  };
})();
