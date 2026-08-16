/* ============================================================================
   Sessão — o trilho do atendimento (docs/SESSAO.md, fatia S1).
   Cria/retoma a sessão, mostra as 7 etapas e conduz para as telas que já
   existem. Não duplica nenhuma tela: cada etapa aponta para o que já funciona.
   Planejar/Aplicar ainda não têm painel próprio (fatias S3/S5) e dizem isso na
   cara em vez de fingir.
   Depende de app.js (carregado antes): $, api, toast, escHtml, IC, busy,
   confirmModal, showPage, navTo, doScan, openReport, state.
   ========================================================================== */
(function () {
  "use strict";

  const SES = { s: null, defs: [], vista: null, sessoes: null, intencao: "equilibrado", poll: null };

  const INTENCOES = [
    ["equilibrado", "Equilibrado", "Ganho seguro, sem efeito colateral"],
    ["competitivo", "Competitivo", "Latência acima de tudo"],
    ["streaming", "Streaming", "Jogo + captura na mesma máquina"],
    ["livre", "Livre", "Sem perfil — você escolhe item a item"],
  ];

  const STATUS_TXT = {
    concluida: "concluída", pulada: "pulada", "em-andamento": "agora",
    disponivel: "a seguir", bloqueada: "bloqueada",
  };

  // O que cada etapa faz HOJE. A navegação é assunto da UI — por isso o mapa
  // vive aqui e não no backend (que manda só id/nome/resumo).
  const ACOES = {
    medir: { painel: () => painelMedida("baseline") },
    ler: { label: "Escanear PC", fn: () => doScan(), dica: "Leitura pura: nada é alterado." },
    entender: { label: "Rodar diagnóstico", fn: () => showPage("diagnostico") },
    planejar: { painel: painelPlanejar },
    aplicar: { painel: painelAplicar },
    provar: { painel: () => painelMedida("depois") },
    entregar: { painel: painelEntregar },
  };

  /* ---- Estado -------------------------------------------------------------- */

  function ingest(resp) {
    // Uma sessão que existe mas não abre precisa aparecer: o técnico tem que
    // saber que perdeu o registro, não achar que nunca houve atendimento.
    if (resp && resp.aviso) toast("err", "Sessão ilegível", resp.aviso);
    SES.s = (resp && resp.sessao) || null;
    SES.cmp = (resp && resp.comparacao) || null; // vem calculada do backend, fonte única
    if (resp && resp.etapas) SES.defs = resp.etapas;
    state.sessao = SES.s; // deixa a sessão visível para o resto do app (relatório etc.)
    if (!SES.s || !SES.s.etapas.some((e) => e.id === SES.vista)) SES.vista = SES.s ? SES.s.etapa : null;
    renderBadge();
    render();
  }

  function idx(id) { return SES.defs.findIndex((d) => d.id === id); }
  function defOf(id) { return SES.defs.find((d) => d.id === id) || { nome: id, resumo: "" }; }
  function stOf(id) { return (SES.s && SES.s.etapas.find((e) => e.id === id)) || null; }

  function renderBadge() {
    const b = $("#navSessao"); if (!b) return;
    if (!SES.s || SES.s.encerrada) { b.textContent = ""; return; }
    b.textContent = (idx(SES.s.etapa) + 1) + "/" + SES.defs.length;
  }

  /* ---- Render -------------------------------------------------------------- */

  function render() {
    const box = $("#sessaoBox"); if (!box) return;
    // O plano tem dezenas de itens: re-renderizar a cada clique não pode jogar
    // o técnico de volta para o topo da lista.
    const pgs = $(".pages"), sc = pgs ? pgs.scrollTop : 0;
    box.innerHTML = SES.s && !SES.s.encerrada ? viewTrilho() : viewAbertura();
    if (pgs) pgs.scrollTop = sc;
    // icons.js só popula os [data-icon] que existiam no carregamento; o que a
    // gente monta em runtime precisa pedir.
    $$("#sessaoBox [data-icon]").forEach((el) => { const ic = IC(el.dataset.icon); if (ic) el.innerHTML = ic; });
  }

  function viewAbertura() {
    const passos = SES.defs.length
      ? SES.defs.map((d, i) => `<li><b>${i + 1}. ${escHtml(d.nome)}</b><span>${escHtml(d.resumo)}</span></li>`).join("")
      : "";
    const ints = INTENCOES.map(([id, nome, sub]) => `<button class="ses-chip ${SES.intencao === id ? "on" : ""}" data-int="${id}">
        <b>${nome}</b><span>${sub}</span></button>`).join("");

    return `
      <div class="ses-intro">
        <div class="ses-kicker">DRACO // SESSÃO</div>
        <h2>Um atendimento, sete etapas</h2>
        <p class="ses-lead">Medir antes de mexer, ler, entender, planejar, aplicar, provar o ganho e entregar.
          A sessão guarda onde você parou: fechou o app no meio, ela volta na mesma etapa.</p>
        <ol class="ses-preview">${passos}</ol>
        <div class="ptitle">Abrir sessão</div>
        <div class="ses-form">
          <input id="sesCliente" class="ses-input" placeholder="Nome do cliente (opcional)" autocomplete="off" maxlength="80">
          <div class="ses-chips">${ints}</div>
          <p class="ses-note">A intenção fica registrada agora e vai semear o plano automático quando ele existir.</p>
          <button class="btn-hero" data-act="iniciar"><span data-icon="bolt"></span> Iniciar sessão</button>
        </div>
      </div>
      <div class="ptitle">Atendimentos anteriores</div>
      <div class="ses-lista" id="sesLista">${listaHTML()}</div>`;
  }

  function listaHTML() {
    if (SES.sessoes === null) return `<div class="empty">${IC("ring")}<div>Lendo…</div></div>`;
    if (!SES.sessoes.length) return `<div class="empty">${IC("empty")}<div>Nenhum atendimento gravado ainda.</div></div>`;
    return SES.sessoes.map((r) => {
      const quem = escHtml(r.cliente && r.cliente.nome ? r.cliente.nome : r.pc || "—");
      const et = defOf(r.etapa).nome || r.etapa;
      return `<div class="ses-item">
        <div class="si-l"><b>${quem}</b><span>${escHtml(r.criada)} · ${escHtml(r.intencao)}</span></div>
        <div class="si-r">
          <span class="ses-pill ${r.encerrada ? "off" : "on"}">${r.encerrada ? "encerrada" : "parou em " + escHtml(et)}</span>
          ${r.encerrada ? "" : `<button class="mbtn" data-act="retomar" data-id="${escHtml(r.id)}">Retomar</button>`}
        </div>
      </div>`;
    }).join("");
  }

  function viewTrilho() {
    const s = SES.s;
    const quem = escHtml(s.cliente && s.cliente.nome ? s.cliente.nome : s.pc || "este PC");
    const passos = SES.defs.map((d, i) => {
      const e = stOf(d.id) || {};
      const on = d.id === SES.vista ? " vendo" : "";
      return `<li class="tr-step st-${e.status || "disponivel"}${on}" data-act="ir" data-etapa="${d.id}">
        <span class="tr-n">${e.status === "concluida" ? IC("check") : i + 1}</span>
        <span class="tr-txt"><b>${escHtml(d.nome)}</b><span>${STATUS_TXT[e.status] || ""}</span></span>
      </li>`;
    }).join("");

    return `
      <div class="ses-bar">
        <div class="sb-l">
          <span class="ses-kicker">SESSÃO ATIVA</span>
          <b>${quem}</b>
          <span class="sb-meta">${escHtml(s.intencao)} · aberta em ${escHtml(s.criada)}</span>
        </div>
        <div class="sb-r">
          ${s.limitada ? `<span class="ses-pill warn">modo limitado · sem admin</span>` : ""}
          <button class="mbtn" data-act="encerrar">Encerrar sessão</button>
        </div>
      </div>
      ${rebootCardHTML()}
      <ol class="trilho">${passos}</ol>
      ${painelHTML()}`;
  }

  function painelHTML() {
    const s = SES.s, id = SES.vista, d = defOf(id), e = stOf(id) || {};
    const i = idx(id), corrente = id === s.etapa;
    const a = ACOES[id] || {};

    let avisos = "";
    if (e.status === "bloqueada") {
      avisos += `<div class="warn-box">${IC("warn")}<div><b>Etapa bloqueada.</b> ${escHtml(e.motivo || "")}.
        Feche o app e abra como administrador para liberar — ou pule a etapa registrando o motivo.</div></div>`;
    }
    if (e.status === "pulada") {
      avisos += `<div class="ses-box skip">${IC("warn")}<div><b>Etapa pulada.</b> Motivo: ${escHtml(e.motivo || "—")}
        <span>Isso entra no relatório — o que não foi feito fica dito.</span></div></div>`;
    }
    if (e.status === "concluida") {
      avisos += `<div class="ses-box ok">${IC("check")}<div><b>Etapa concluída</b> em ${escHtml(e.quando || "—")}.</div></div>`;
    }
    if (a.nota) avisos += `<div class="ses-box info">${IC("info")}<div>${escHtml(a.nota)}</div></div>`;

    // corpo próprio da etapa (Planejar tem o plano; o resto é ação + atalho)
    const corpo = a.painel ? a.painel() : "";

    const acts = a.label
      ? `<div class="sp-acts">
           <button class="btn-hero" data-act="acao"><span data-icon="bolt"></span> ${escHtml(a.label)}</button>
           ${a.alt ? `<button class="mbtn" data-act="acao2">${escHtml(a.alt.label)}</button>` : ""}
           ${a.dica ? `<span class="sp-dica">${escHtml(a.dica)}</span>` : ""}
         </div>`
      : "";

    // com uma fase em andamento, concluir/pular a etapa não faz sentido
    const rodando = SES.s.execucao && SES.s.execucao.estado === "rodando";
    const foot = rodando ? "" : corrente && e.status !== "bloqueada"
      ? `<div class="sp-foot">
           <button class="btnG" data-act="concluir"><span data-icon="check"></span> Concluir etapa</button>
           <button class="mbtn" data-act="pular">Pular etapa</button>
         </div>`
      : corrente
        ? `<div class="sp-foot"><button class="mbtn" data-act="pular">Pular etapa</button></div>`
        : `<div class="sp-foot"><button class="mbtn" data-act="ir" data-etapa="${escHtml(s.etapa)}">Voltar para a etapa atual</button></div>`;

    return `<div class="ses-painel">
      <div class="sp-head">
        <span class="sp-n">Etapa ${i + 1} de ${SES.defs.length}</span>
        <h3>${escHtml(d.nome)}</h3>
        <p>${escHtml(d.resumo)}</p>
      </div>
      ${avisos}${corpo}${acts}${foot}
    </div>`;
  }

  /* ---- Etapa Planejar: o plano ---------------------------------------------- */

  const FASES = [
    { n: 1, titulo: "Aplica agora", sub: "efeito imediato, sem reiniciar" },
    { n: 2, titulo: "Pede reinício", sub: "só vale depois que o Windows reiniciar" },
    { n: 3, titulo: "Na mão", sub: "BIOS, Windows ou decisão sua — o app não faz por você" },
  ];
  const TIER_TXT = { verde: "seguro", amarelo: "médio", vermelho: "avançado" };
  // rules.json é ASCII por convenção (docs/CONVENTIONS.md) — acento é da exibição
  const IMP_TXT = { alto: "alto", medio: "médio", baixo: "baixo" };

  function painelPlanejar() {
    const p = SES.s.plano;
    if (!p) {
      return `<div class="pl-vazio">
        <p>O plano cruza a <b>varredura</b> com o <b>diagnóstico de gargalos</b> e o perfil
           <b>${escHtml(SES.s.intencao)}</b>, e devolve uma fila: o que aplica agora, o que só vale
           depois de reiniciar e o que é na mão. Você revisa e aprova — nada é escrito aqui.</p>
        <p class="ses-note">Leva alguns segundos: é a varredura completa mais as sondas de gargalo.</p>
        <div class="sp-acts"><button class="btn-hero" data-act="gerar"><span data-icon="bolt"></span> Gerar plano</button></div>
      </div>`;
    }

    const r = p.resumo || {};
    const grupos = FASES.map((f) => {
      const itens = p.itens.filter((i) => i.fase === f.n);
      if (!itens.length) return "";
      const marcados = (r.por_fase || {})[f.n] || 0;
      return `<div class="pl-fase">
        <div class="pl-fase-head">
          <span class="pl-fase-n">${f.n}</span>
          <div><b>${f.titulo}</b><span>${f.sub}</span></div>
          <span class="pl-fase-cnt">${itens.length} ${itens.length === 1 ? "item" : "itens"}${marcados ? ` · <b>${marcados}</b> marcados` : ""}</span>
        </div>
        ${itens.map(itemHTML).join("")}
      </div>`;
    }).join("");

    const dif = (r.score_projetado || 0) - (r.score_atual || 0);
    const perfis = INTENCOES.map(([id, nome]) =>
      `<button class="pl-perfil ${SES.s.intencao === id ? "on" : ""}" data-act="perfil" data-int="${id}">${nome}</button>`).join("");

    return `
      <div class="pl-bar">
        <span class="ses-note" style="margin:0">Perfil:</span>
        <div class="pl-perfis">${perfis}</div>
        <span class="ses-note" style="margin:0 0 0 auto">gerado em ${escHtml(p.gerado || "—")}</span>
        <button class="mbtn" data-act="gerar">Gerar de novo</button>
      </div>
      <div class="pl-lista">${grupos}</div>
      <div class="pl-resumo">
        <div class="plr-col"><span>Selecionados</span><b>${r.selecionados || 0}<i>/${r.total || 0}</i></b></div>
        <div class="plr-col"><span>Pedem reinício</span><b>${r.pedem_reboot || 0}</b></div>
        <div class="plr-col"><span>Pedem confirmação</span><b>${r.pedem_consentimento || 0}</b></div>
        ${r.bloqueados ? `<div class="plr-col"><span>Bloqueados</span><b class="trava">${r.bloqueados}</b></div>` : ""}
        <div class="plr-score">
          <span>Boost Score</span>
          <b>${r.score_atual || 0} <i>→</i> <em>${r.score_projetado || 0}</em>${dif > 0 ? ` <u>+${dif}</u>` : ""}</b>
          <span class="plr-nota">projeção do score, não previsão de FPS — quem mede FPS é a etapa Provar</span>
        </div>
      </div>`;
  }

  function itemHTML(i) {
    const marca = i.bloqueado
      ? `<span class="pl-check trava" title="${escHtml(i.bloqueado)}"></span>`
      : i.selecionavel
        ? `<label class="pl-check ${i.selecionado ? "on" : ""}" data-act="marcar" data-id="${escHtml(i.id)}">${i.selecionado ? IC("check") : ""}</label>`
        : `<span class="pl-check mao" title="Você faz — o app não executa">${IC("guide")}</span>`;
    const pills = [
      `<span class="pl-pill t-${i.tier}">${TIER_TXT[i.tier] || i.tier}</span>`,
      i.impacto ? `<span class="pl-pill imp-${i.impacto}">impacto ${IMP_TXT[i.impacto] || i.impacto}</span>` : "",
      i.requer_consentimento ? `<span class="pl-pill consent">pede confirmação</span>` : "",
    ].join("");
    return `<div class="pl-item ${i.selecionado ? "sel" : ""} ${i.bloqueado ? "travado" : ""} ${i.origem_tipo === "gargalo" ? "gargalo" : ""}">
      ${marca}
      <div class="pl-body">
        <div class="pl-top"><b>${escHtml(i.titulo)}</b>${pills}</div>
        ${i.bloqueado
          ? `<div class="pl-trava">${IC("warn")}<span>${escHtml(i.bloqueado)}</span></div>`
          : `<div class="pl-porque">${escHtml(i.porque || "")}</div>`}
        ${i.orientacao ? `<div class="pl-orient">${escHtml(i.orientacao)}</div>` : ""}
      </div>
      <div class="pl-act">
        ${i.tecnico ? `<button class="pl-info" data-act="tecnico" data-id="${escHtml(i.id)}" title="O que isso mexe no sistema">${IC("info")}</button>` : ""}
        ${i.alvo ? `<button class="mbtn" data-act="pagina" data-alvo="${escHtml(i.alvo)}">Abrir</button>` : ""}
      </div>
    </div>`;
  }

  function achaItem(id) { return (SES.s && SES.s.plano ? SES.s.plano.itens : []).find((i) => i.id === id); }

  /* ---- Reinício: a ponte entre as duas metades do atendimento --------------- */

  function rebootCardHTML() {
    const s = SES.s, rb = s.reboot;
    if (rb && rb.avisar) {
      const alvo = defOf(rb.voltar_para || "provar").nome;
      return `<div class="rb-card ok">
        <div class="rb-ic">${IC("check")}</div>
        <div class="rb-body">
          <b>O PC reiniciou — os ajustes da fase 2 estão valendo agora.</b>
          <span>Foi para isso que a sessão ficou guardada. O passo seguinte é medir o resultado.</span>
        </div>
        <div class="rb-act">
          <button class="btn-hero" data-act="pos-reboot"><span data-icon="gauge"></span> Ir para ${escHtml(alvo)}</button>
          <button class="mbtn" data-act="reboot-visto">Depois</button>
        </div>
      </div>`;
    }
    if (rb && rb.pendente) {
      return `<div class="rb-card wait">
        <div class="rb-ic">${IC("warn")}</div>
        <div class="rb-body">
          <b>Reinício pedido, ainda não aconteceu.</b>
          <span>Quando o Windows reiniciar, abra o ThazzDraco de novo — a sessão volta neste ponto.</span>
        </div>
      </div>`;
    }
    if (precisaReiniciar()) {
      return `<div class="rb-card wait">
        <div class="rb-ic">${IC("warn")}</div>
        <div class="rb-body">
          <b>Falta reiniciar.</b>
          <span>Os ajustes da fase 2 só passam a valer depois do reinício — e sem eles a medição do "depois" mede a máquina pela metade.</span>
        </div>
        <div class="rb-act"><button class="btn-hero" data-act="reiniciar"><span data-icon="refresh"></span> Reiniciar agora</button></div>
      </div>`;
    }
    return "";
  }

  function precisaReiniciar() {
    const s = SES.s;
    if (!s.execucao || !s.execucao.requer_reboot) return false;
    return !s.reboot || !s.reboot.confirmado;
  }

  function pedirReinicio() {
    confirmModal({
      title: "Reiniciar agora?",
      body: `<p>Salve o que estiver aberto. O Windows reinicia em alguns segundos.</p>
        <p class="ses-note">A sessão fica guardada: quando você abrir o ThazzDraco de novo, ela volta
        aqui e oferece medir o resultado.</p>`,
      okLabel: "Reiniciar", danger: true,
      onOk: async () => {
        try { await api("/api/sessao/reiniciar", { voltar_para: "provar" });
          toast("info", "Reiniciando…", "Abra o ThazzDraco depois que o Windows subir."); }
        catch (e) { toast("err", "Não deu para reiniciar", e.message); }
      },
    });
  }

  async function posReboot() {
    const alvo = (SES.s.reboot && SES.s.reboot.voltar_para) || "provar";
    try {
      ingest(await api("/api/sessao/reboot-visto", {}));
      // Aplicar acabou (foi o reinício que fechou a fase 2): conclui e o trilho
      // anda sozinho. Mas só se ela estiver liberada — num PC sem admin a etapa
      // está bloqueada, e receber o técnico com um erro no botão de "próximo
      // passo" seria pior que não fazer nada.
      const e = stOf(SES.s.etapa);
      if (SES.s.etapa === "aplicar" && e && e.status === "em-andamento") await etapaAcao("concluir");
    } catch (err) { toast("err", "Falha", err.message); }
    SES.vista = alvo;
    render();
  }

  /* ---- Etapa Entregar: o relatório ------------------------------------------ */

  function painelEntregar() {
    const s = SES.s, p = s.plano;
    const cont = contagens();
    const pulos = (s.etapas || []).filter((e) => e.status === "pulada");
    return `
      <p class="ses-lead">Fecha o atendimento: o relatório traz o que foi feito, o que <b>não</b> foi
        (e por quê) e o antes × depois medido.</p>
      <div class="ent-grid">
        <div class="ent-num"><span>Aplicados</span><b>${cont.aplicados}</b></div>
        <div class="ent-num"><span>Falharam</span><b class="${cont.falhas ? "ruim" : ""}">${cont.falhas}</b></div>
        <div class="ent-num"><span>Na mão</span><b>${cont.mao}/${cont.maoTotal}</b></div>
        <div class="ent-num"><span>Etapas puladas</span><b class="${pulos.length ? "aviso" : ""}">${pulos.length}</b></div>
      </div>
      ${provaHTML()}
      ${pulos.length ? `<div class="ses-box skip">${IC("warn")}<div><b>O que ficou de fora vai no relatório:</b>
        <span>${pulos.map((e) => escHtml(defOf(e.id).nome) + " — " + escHtml(e.motivo || "")).join(" · ")}</span></div></div>` : ""}
      <div class="sp-acts">
        <button class="btn-hero" data-act="relatorio"><span data-icon="doc"></span> Abrir relatório</button>
        <button class="mbtn" data-act="encerrar">Encerrar sessão</button>
      </div>`;
  }

  function provaHTML() {
    const cmp = SES.cmp;
    const temProva = cmp && cmp.fps && cmp.fps.comparavel;
    if (temProva) {
      const d = (cmp.fps.metricas || []).find((m) => m.rotulo === "FPS médio");
      return `<div class="ses-box ok">${IC("check")}<div><b>Ganho medido:</b>
        ${d ? `FPS médio ${num(d.antes)} → ${num(d.depois)} (${d.pct > 0 ? "+" : ""}${num(d.pct)}%)` : "FPS comparado"}
        <span>Mesmo jogo, mesma resolução — é comparação de verdade.</span></div></div>`;
    }
    const motivo = cmp && cmp.fps ? cmp.fps.motivo : "não houve medição de FPS nesta sessão";
    return `<div class="ses-box skip">${IC("warn")}<div><b>Sem prova de FPS.</b> ${escHtml(motivo)}
      <span>O relatório vai dizer isso com todas as letras — número sem cenário não é prova.</span></div></div>`;
  }

  function contagens() {
    const itens = (SES.s.plano && SES.s.plano.itens) || [];
    const mao = itens.filter((i) => !i.selecionavel);
    return {
      aplicados: itens.filter((i) => i.estado === "aplicado").length,
      falhas: itens.filter((i) => i.estado === "falhou").length,
      mao: mao.filter((i) => i.estado === "feito").length,
      maoTotal: mao.length,
    };
  }

  // relatorioHTML é injetado no relatório do cliente (app.js → openReport).
  function relatorioHTML() {
    const s = SES.s; if (!s) return "";
    const cont = contagens(), itens = (s.plano && s.plano.itens) || [];
    const lista = (f) => itens.filter(f).map((i) => `<li>${escHtml(i.titulo)}</li>`).join("");
    const falhados = itens.filter((i) => i.estado === "falhou");
    const pulos = (s.etapas || []).filter((e) => e.status === "pulada");
    const cmp = SES.cmp || {};

    const tab = (nome, b) => {
      if (!b) return "";
      if (!b.comparavel) return `<h2>${nome} — não comparável</h2><p class="r-nao">${escHtml(b.motivo || "")}</p>`;
      return `<h2>${nome}${b.cenario ? " — " + escHtml(b.cenario) : ""}</h2>
        <table class="r-tab"><tr><th></th><th>Antes</th><th>Depois</th><th>Dif.</th></tr>
        ${(b.metricas || []).map((d) => `<tr><td>${escHtml(d.rotulo)}</td><td>${num(d.antes)}</td><td>${num(d.depois)}</td>
          <td class="${d.igual ? "" : (d.melhorou ? "r-ok" : "r-bad")}">${d.igual ? "igual" : (d.pct > 0 ? "+" : "") + num(d.pct) + "%"}</td></tr>`).join("")}
        </table>${b.observacao ? `<p class="r-obs">${escHtml(b.observacao)}</p>` : ""}`;
    };

    return `
      <h2>Sessão de atendimento</h2>
      <div class="r-grid">
        <div><span>Aberta em</span><span>${escHtml(s.criada)}</span></div>
        <div><span>Perfil</span><span>${escHtml(s.intencao)}</span></div>
        <div><span>Itens aplicados</span><span>${cont.aplicados}</span></div>
        <div><span>Itens que falharam</span><span>${cont.falhas}</span></div>
        <div><span>Feitos à mão</span><span>${cont.mao} de ${cont.maoTotal}</span></div>
        <div><span>Reinício</span><span>${s.reboot && s.reboot.confirmado ? "feito em " + escHtml(s.reboot.confirmado) : (precisaReiniciar() ? "PENDENTE" : "não foi necessário")}</span></div>
      </div>
      ${tab("FPS no jogo", cmp.fps)}
      ${tab("Benchmark", cmp.bench)}
      ${cont.aplicados ? `<h2>Aplicado nesta sessão (${cont.aplicados})</h2><ul>${lista((i) => i.estado === "aplicado")}</ul>` : ""}
      ${falhados.length ? `<h2>Não pegou (${falhados.length})</h2><ul>${falhados.map((i) => `<li>${escHtml(i.titulo)} — <i>${escHtml(i.erro || "")}</i></li>`).join("")}</ul>` : ""}
      ${cont.maoTotal ? `<h2>Na mão</h2><ul>${itens.filter((i) => !i.selecionavel).map((i) => `<li>${i.estado === "feito" ? "✔" : "○"} ${escHtml(i.titulo)}${i.orientacao ? " — " + escHtml(i.orientacao) : ""}</li>`).join("")}</ul>` : ""}
      ${pulos.length ? `<h2>Etapas puladas</h2><ul>${pulos.map((e) => `<li>${escHtml(defOf(e.id).nome)} — <i>${escHtml(e.motivo || "")}</i></li>`).join("")}</ul>` : ""}`;
  }

  /* ---- Etapas Medir e Provar: a evidência ----------------------------------- */

  function num(v) {
    if (v === undefined || v === null) return "—";
    const a = Math.abs(v);
    return a >= 100 ? Math.round(v).toString() : (Math.round(v * 10) / 10).toString();
  }

  function painelMedida(lado) {
    const s = SES.s;
    const ev = (lado === "baseline" ? s.baseline : s.depois) || {};
    const outro = (lado === "baseline" ? s.depois : s.baseline) || {};
    const intro = lado === "baseline"
      ? `<p class="ses-lead">Mede a máquina <b>como ela está agora</b>, antes de qualquer mudança.
         Sem essa foto, o ganho no fim é opinião — o relatório vai dizer "sem prova".</p>`
      : `<p class="ses-lead">Mede de novo, <b>no mesmo cenário</b>, e compara. Jogo diferente ou
         resolução diferente não é comparação: o app recusa e diz por quê.</p>`;

    const pronto = SES.fpsPronto;
    const cardFPS = ev.fps
      ? `<div class="med-card feito">
          <div class="med-top"><b>FPS no jogo</b><span class="ses-pill on">medido</span></div>
          <div class="med-nums">
            <div><span>FPS médio</span><b>${num(ev.fps.res.fps_avg)}</b></div>
            <div><span>1% low</span><b>${num(ev.fps.res.low1)}</b></div>
            <div><span>0.1% low</span><b>${num(ev.fps.res.low01)}</b></div>
          </div>
          <div class="med-cen">${escHtml(cenTexto(ev.fps.cenario))} · ${escHtml(ev.fps.quando)}</div>
          <button class="mbtn" data-act="medir" data-lado="${lado}" data-tipo="fps">Substituir pela última captura</button>
        </div>`
      : `<div class="med-card">
          <div class="med-top"><b>FPS no jogo</b><span class="ses-pill">não medido</span></div>
          ${pronto && pronto.pronto
            ? `<p class="ses-note">Há uma captura pronta: <b>${escHtml(pronto.processo || "?")}</b> ·
                 ${num(pronto.fps_avg)} fps médio · ${num(pronto.duracao_s)}s.</p>
               <button class="btn-hero" data-act="medir" data-lado="${lado}" data-tipo="fps"><span data-icon="check"></span> Guardar como ${lado === "baseline" ? "baseline" : "depois"}</button>`
            : `<p class="ses-note">Meça o FPS com o jogo aberto (PresentMon, sem injeção). Depois volte aqui para guardar.</p>
               <button class="btn-hero" data-act="irfps"><span data-icon="game"></span> Medir FPS no jogo</button>
               <button class="mbtn" data-act="verfps">Verificar captura</button>`}
        </div>`;

    const cardBench = ev.bench
      ? `<div class="med-card feito">
          <div class="med-top"><b>Benchmark</b><span class="ses-pill on">medido</span></div>
          <div class="med-nums">
            <div><span>Índice</span><b>${num(ev.bench.res.indice)}</b></div>
            <div><span>CPU 1 núcleo</span><b>${num(ev.bench.res.cpu_single)}</b></div>
            <div><span>Memória</span><b>${num(ev.bench.res.mem_bw)}<i>GB/s</i></b></div>
            <div><span>Disco</span><b>${num(ev.bench.res.disk_write)}<i>MB/s</i></b></div>
          </div>
          <div class="med-cen">${escHtml(ev.bench.quando)}</div>
          <button class="mbtn" data-act="medir" data-lado="${lado}" data-tipo="bench">Medir de novo</button>
        </div>`
      : `<div class="med-card">
          <div class="med-top"><b>Benchmark</b><span class="ses-pill">não medido</span></div>
          <p class="ses-note">CPU, memória e disco. Roda aqui mesmo, leva alguns segundos —
             feche o que estiver pesado antes.</p>
          <button class="btn-hero" data-act="medir" data-lado="${lado}" data-tipo="bench"><span data-icon="gauge"></span> Rodar benchmark</button>
        </div>`;

    let html = intro + `<div class="med-grid">${cardFPS}${cardBench}</div>`;

    if (lado === "depois") html += comparacaoHTML();
    else if (!ev.fps && !ev.bench) {
      html += `<div class="ses-box skip">${IC("warn")}<div><b>Nada medido ainda.</b>
        Dá para pular esta etapa, mas aí o atendimento fica sem prova de ganho — e o relatório vai dizer isso.</div></div>`;
    } else if (!outro.fps && ev.fps) {
      html += `<div class="ses-box info">${IC("info")}<div>Guarde o cenário: <b>${escHtml(cenTexto(ev.fps.cenario))}</b>.
        Na etapa <b>Provar</b> tem que ser o mesmo jogo e a mesma resolução, senão não há comparação.</div></div>`;
    }
    return html;
  }

  function cenTexto(c) {
    if (!c) return "";
    return [c.jogo || c.exe, c.resolucao, c.refresh_hz ? c.refresh_hz + " Hz" : "", c.duracao_s ? c.duracao_s + "s" : ""]
      .filter(Boolean).join(" · ");
  }

  function comparacaoHTML() {
    const cmp = SES.cmp;
    if (!cmp || (!cmp.tem_baseline && !cmp.tem_depois)) return "";
    return `<div class="ptitle">Antes × depois</div>` + [
      ["FPS no jogo", cmp.fps], ["Benchmark", cmp.bench],
    ].map(([nome, b]) => b ? blocoCmpHTML(nome, b) : "").join("");
  }

  function blocoCmpHTML(nome, b) {
    if (!b.comparavel) {
      return `<div class="cmp-bloco">
        <div class="cmp-head"><b>${nome}</b><span class="ses-pill warn">não comparável</span></div>
        <div class="warn-box">${IC("warn")}<div>${escHtml(b.motivo || "")}
          ${b.cenario ? `<div class="med-cen" style="margin-top:6px">${escHtml(b.cenario)}</div>` : ""}</div></div>
      </div>`;
    }
    const linhas = (b.metricas || []).map((d) => {
      const cls = d.igual ? "igual" : (d.melhorou ? "melhor" : "pior");
      const sinal = d.pct > 0 ? "+" : "";
      return `<tr class="${cls}">
        <td>${escHtml(d.rotulo)}${d.unidade ? ` <i>${escHtml(d.unidade)}</i>` : ""}</td>
        <td>${num(d.antes)}</td><td>${num(d.depois)}</td>
        <td class="cmp-dif">${d.igual ? "igual" : sinal + num(d.pct) + "%"}</td>
      </tr>`;
    }).join("");
    return `<div class="cmp-bloco">
      <div class="cmp-head"><b>${nome}</b>${b.cenario ? `<span class="med-cen">${escHtml(b.cenario)}</span>` : ""}</div>
      <table class="cmp-tab"><thead><tr><th></th><th>Antes</th><th>Depois</th><th>Diferença</th></tr></thead>
        <tbody>${linhas}</tbody></table>
      ${b.observacao ? `<div class="cmp-obs">${IC("info")}<span>${escHtml(b.observacao)}</span></div>` : ""}
    </div>`;
  }

  async function medir(lado, tipo) {
    busy(true, tipo === "bench" ? "Medindo CPU, memória e disco…" : "Guardando a captura…");
    try {
      ingest(await api("/api/sessao/medir", { fase: lado, tipo }));
      toast("ok", tipo === "bench" ? "Benchmark guardado" : "Captura guardada",
        lado === "baseline" ? "É esta a foto do antes." : "Agora dá para comparar.");
    } catch (e) { toast("err", "Não deu para medir", e.message); }
    finally { busy(false); }
  }

  async function verFpsPronto(avisar) {
    try {
      SES.fpsPronto = await api("/api/sessao/fps-pronto");
      if (avisar) {
        toast(SES.fpsPronto.pronto ? "ok" : "info",
          SES.fpsPronto.pronto ? "Captura encontrada" : "Nenhuma captura pronta",
          SES.fpsPronto.pronto ? escHtml(SES.fpsPronto.processo || "") : "Meça o FPS com o jogo aberto.");
      }
      render();
    } catch (e) { /* sem captura é estado normal */ }
  }

  /* ---- Etapa Aplicar: a fila ------------------------------------------------ */

  const EST_TXT = {
    pendente: "na fila", aplicando: "aplicando…", aplicado: "aplicado",
    falhou: "falhou", pulado: "pulado", feito: "feito por você", desconhecido: "indefinido",
  };

  function itensDaFase(f) {
    const p = SES.s.plano; if (!p) return [];
    return p.itens.filter((i) => i.fase === f && i.selecionado && i.selecionavel);
  }
  function fasePendente(f) {
    return itensDaFase(f).some((i) => i.estado === "pendente" || i.estado === "aplicando" || i.estado === "desconhecido");
  }

  function painelAplicar() {
    const p = SES.s.plano;
    if (!p) {
      return `<div class="ses-box info">${IC("info")}<div>Nenhum plano ainda. Volte para <b>Planejar</b> e gere o plano — é ele que diz o que executar.</div></div>`;
    }
    const ex = SES.s.execucao;
    if (ex && ex.estado === "rodando") return execRodandoHTML(ex);

    let html = "";
    if (ex && ex.estado === "interrompida") {
      html += `<div class="warn-box">${IC("warn")}<div><b>Execução interrompida.</b> ${escHtml(ex.mensagem || "")}</div></div>`;
    } else if (ex && ex.estado === "concluida") {
      const cls = ex.falhas ? "skip" : "ok";
      html += `<div class="ses-box ${cls}">${IC(ex.falhas ? "warn" : "check")}<div>
        <b>Fase ${ex.fase} concluída</b> — ${escHtml(ex.mensagem || "")}
        <span>${ex.batch_id ? "Lote <b>" + escHtml(ex.batch_id) + "</b> no Histórico: dá para desfazer tudo de uma vez." : "Nada foi escrito."}</span>
      </div></div>`;
      // o aviso de reinício (com o botão) vive no topo do trilho — rebootCardHTML()
    }

    html += [1, 2].map(faseHTML).join("");

    const mao = p.itens.filter((i) => i.fase === 3);
    if (mao.length) {
      html += `<div class="pl-fase" style="margin-top:18px">
        <div class="pl-fase-head"><span class="pl-fase-n">3</span>
          <div><b>Na mão</b><span>o app não executa — marque conforme for fazendo</span></div></div>
        ${mao.map(itemMaoHTML).join("")}</div>`;
    }
    return html;
  }

  function faseHTML(f) {
    const itens = itensDaFase(f);
    if (!itens.length) return "";
    const pend = fasePendente(f);
    const nome = f === 1 ? "Aplica agora" : "Pede reinício";
    return `<div class="pl-fase" style="margin-top:18px">
      <div class="pl-fase-head">
        <span class="pl-fase-n">${f}</span>
        <div><b>${nome}</b><span>${itens.length} ${itens.length === 1 ? "item marcado" : "itens marcados"}</span></div>
        ${pend ? `<button class="btnG" data-act="rodar" data-fase="${f}"><span data-icon="bolt"></span> Executar fase ${f}</button>`
               : `<span class="pl-fase-cnt">fase executada</span>`}
      </div>
      ${itens.map(itemExecHTML).join("")}</div>`;
  }

  function itemExecHTML(i) {
    const est = i.estado || "pendente";
    return `<div class="pl-item ex-${est}">
      <span class="ex-dot"></span>
      <div class="pl-body">
        <div class="pl-top"><b>${escHtml(i.titulo)}</b><span class="pl-pill est-${est}">${EST_TXT[est] || est}</span></div>
        ${i.erro ? `<div class="ex-erro">${escHtml(i.erro)}</div>` : `<div class="pl-porque">${escHtml(i.porque || "")}</div>`}
      </div>
      <div class="pl-act">${i.tecnico ? `<button class="pl-info" data-act="tecnico" data-id="${escHtml(i.id)}" title="O que isso mexe no sistema">${IC("info")}</button>` : ""}</div>
    </div>`;
  }

  function itemMaoHTML(i) {
    const feito = i.estado === "feito";
    return `<div class="pl-item ${feito ? "ex-feito" : ""}">
      <label class="pl-check ${feito ? "on" : ""}" data-act="feito" data-id="${escHtml(i.id)}">${feito ? IC("check") : ""}</label>
      <div class="pl-body">
        <div class="pl-top"><b>${escHtml(i.titulo)}</b>${feito ? `<span class="pl-pill est-feito">feito por você</span>` : ""}</div>
        <div class="pl-porque">${escHtml(i.porque || "")}</div>
        ${i.orientacao ? `<div class="pl-orient">${escHtml(i.orientacao)}</div>` : ""}
      </div>
      <div class="pl-act">${i.alvo ? `<button class="mbtn" data-act="pagina" data-alvo="${escHtml(i.alvo)}">Abrir</button>` : ""}</div>
    </div>`;
  }

  function execRodandoHTML(ex) {
    const pct = ex.total ? Math.round((ex.feitos / ex.total) * 100) : 0;
    return `<div class="ex-run">
      <div class="ex-run-top"><span class="ses-kicker">Executando fase ${ex.fase}</span>
        <b>${ex.feitos}<i>/${ex.total}</i></b></div>
      <div class="ex-bar"><i style="width:${pct}%"></i></div>
      <div class="ex-atual">${escHtml(ex.atual || "…")}</div>
      <p class="ses-note">Ponto de restauração e snapshot são feitos antes de qualquer escrita.
        Pode fechar a janela — a sessão sabe reconhecer que a execução foi interrompida.</p>
    </div>`;
  }

  async function rodarFase(fase, confirmar) {
    const pedem = itensDaFase(fase).filter((i) => i.requer_consentimento);
    if (pedem.length && !confirmar) {
      return confirmModal({
        title: "Confirmar " + pedem.length + (pedem.length === 1 ? " ajuste" : " ajustes"),
        body: `<p>Estes itens pedem confirmação explícita:</p>
          <ul style="margin:12px 0 0 18px;display:flex;flex-direction:column;gap:8px">
            ${pedem.map((i) => `<li><b>${escHtml(i.titulo)}</b> — ${escHtml(i.descricao || "")}</li>`).join("")}</ul>
          <div class="warn-box">${IC("shield")}<div>Tudo é reversível pelo Histórico, menos a limpeza de temporários.</div></div>`,
        okLabel: "Executar", danger: pedem.some((i) => i.tier === "vermelho"),
        onOk: () => rodarFase(fase, true),
      });
    }
    busy(true, "Iniciando a fase " + fase + "…");
    try {
      ingest(await api("/api/sessao/aplicar", { fase, confirmar: !!confirmar }));
      pollExec();
    } catch (e) { toast("err", "Não deu para executar", e.message); }
    finally { busy(false); }
  }

  function pollExec() {
    if (SES.poll) return;
    SES.poll = setInterval(async () => {
      try {
        const r = await api("/api/sessao/status");
        if (!r.sessao) return pararPoll();
        SES.s = r.sessao; state.sessao = r.sessao;
        if (SES.vista === "aplicar") render();
        const ex = r.sessao.execucao;
        if (!ex || ex.estado !== "rodando") {
          pararPoll(); renderBadge(); render();
          if (ex && ex.estado === "concluida") {
            toast(ex.falhas ? "warn" : "ok", `Fase ${ex.fase} concluída`, ex.mensagem || "");
          }
        }
      } catch (e) { /* o servidor pode estar ocupado na escrita; tenta de novo */ }
    }, 1200);
  }
  function pararPoll() { if (SES.poll) { clearInterval(SES.poll); SES.poll = null; } }

  async function marcarFeito(id) {
    const it = achaItem(id); if (!it) return;
    try { ingest(await api("/api/sessao/plano/feito", { id, feito: it.estado !== "feito" })); }
    catch (e) { toast("err", "Não gravou", e.message); }
  }

  // Regerar joga fora o que foi marcado à mão — então pergunta antes, se já
  // existe plano. Perder meia hora de revisão por um clique não é aceitável.
  function pedirPlano(intencao) {
    const alvo = intencao || SES.s.intencao;
    if (!SES.s.plano) return gerarPlano(alvo);
    const troca = alvo !== SES.s.intencao;
    confirmModal({
      title: troca ? "Trocar para o perfil " + alvo + "?" : "Gerar o plano de novo?",
      body: `<p>O plano é montado do zero${troca ? " com o perfil <b>" + escHtml(alvo) + "</b>" : ""} —
             <b>as marcações que você fez à mão se perdem</b>.</p>
             <p class="ses-note">A varredura roda de novo, então leva alguns segundos.</p>`,
      okLabel: "Gerar", onOk: () => gerarPlano(alvo),
    });
  }

  async function gerarPlano(intencao) {
    busy(true, "Montando o plano — varrendo e diagnosticando…");
    try {
      ingest(await api("/api/sessao/plano/gerar", { intencao: intencao || SES.s.intencao }));
      const r = (SES.s.plano || {}).resumo || {};
      toast("ok", "Plano pronto", `${r.total || 0} itens · ${r.selecionados || 0} já marcados pelo perfil.`);
    } catch (e) { toast("err", "Não deu para montar o plano", e.message); }
    finally { busy(false); }
  }

  async function marcarItem(id) {
    const it = achaItem(id); if (!it || !it.selecionavel) return;
    if (it.bloqueado) return toast("info", "Item bloqueado", it.bloqueado);
    const novo = !it.selecionado;
    it.selecionado = novo; render(); // resposta imediata; o servidor confirma logo atrás
    try { ingest(await api("/api/sessao/plano/selecionar", { itens: [{ id, selecionado: novo }] })); }
    catch (e) { it.selecionado = !novo; render(); toast("err", "Não gravou a seleção", e.message); }
  }

  function verTecnico(id) {
    const it = achaItem(id); if (!it) return;
    infoModal(it.titulo, `<p>${escHtml(it.descricao || "")}</p>
      ${it.detectado ? `<p class="ses-note">Detectado: ${escHtml(it.detectado)}</p>` : ""}
      <p style="margin-top:12px"><b>O que isso altera:</b></p>
      <p class="pl-tec">${escHtml(it.tecnico)}</p>`);
  }

  /* ---- Ações --------------------------------------------------------------- */

  async function iniciar() {
    const nome = ($("#sesCliente") && $("#sesCliente").value) || "";
    busy(true, "Abrindo sessão…");
    try {
      ingest(await api("/api/sessao/nova", { cliente: { nome }, intencao: SES.intencao }));
      toast("ok", "Sessão aberta", SES.s && SES.s.limitada ? "Sem admin: só as etapas de leitura." : "Comece medindo o PC como ele está.");
    } catch (err) { toast("err", "Não abriu a sessão", err.message); }
    finally { busy(false); }
  }

  async function retomar(id) {
    busy(true, "Retomando…");
    SES.vista = null; // outra sessão: nunca herdar a etapa que estava sendo vista
    try { ingest(await api("/api/sessao/retomar", { id })); toast("ok", "Sessão retomada", ""); }
    catch (err) { toast("err", "Não retomou", err.message); }
    finally { busy(false); }
  }

  async function etapaAcao(acao, motivo) {
    busy(true, acao === "pular" ? "Pulando etapa…" : "Concluindo etapa…");
    try {
      const r = await api("/api/sessao/etapa", { etapa: SES.s.etapa, acao, motivo: motivo || "" });
      const encerrou = r.sessao && r.sessao.encerrada;
      SES.vista = null; // andou o trilho: a vista acompanha (só fica presa quando o usuário volta de propósito)
      ingest(r);
      if (encerrou) { carregaLista(); toast("ok", "Sessão concluída", "O trilho chegou ao fim."); }
      else toast("ok", acao === "pular" ? "Etapa pulada" : "Etapa concluída", "Próxima: " + defOf(SES.s.etapa).nome + ".");
    } catch (err) { toast("err", "Não deu para avançar", err.message); }
    finally { busy(false); }
  }

  function pedePular() {
    const nome = defOf(SES.s.etapa).nome;
    confirmModal({
      title: "Pular " + nome + "?",
      body: `<p>Pular é permitido; esquecer não. O motivo fica gravado na sessão e <b>aparece no relatório</b> —
             é assim que o cliente sabe o que não foi feito.</p>
             <input id="sesMotivo" class="ses-input" placeholder="Por que está pulando esta etapa?" maxlength="200" autocomplete="off">`,
      okLabel: "Pular etapa",
      onOk: () => {
        const m = (($("#sesMotivo") && $("#sesMotivo").value) || "").trim();
        if (!m) { toast("err", "Falta o motivo", "Escreva por que está pulando."); return pedePular(); }
        etapaAcao("pular", m);
      },
    });
  }

  function pedeEncerrar() {
    confirmModal({
      title: "Encerrar a sessão?",
      body: `<p>A sessão fecha no ponto em que está. O que ficou por fazer continua registrado como não feito.</p>
             <p style="color:var(--ink-3);font-size:12.5px">Nada é desfeito: o que já foi aplicado continua aplicado, e o undo segue no Histórico.</p>`,
      okLabel: "Encerrar",
      onOk: async () => {
        busy(true, "Encerrando…");
        try { ingest(await api("/api/sessao/encerrar", {})); carregaLista(); toast("ok", "Sessão encerrada", ""); }
        catch (err) { toast("err", "Não encerrou", err.message); }
        finally { busy(false); }
      },
    });
  }

  async function carregaLista() {
    try { const r = await api("/api/sessoes"); SES.sessoes = r.sessoes || []; }
    catch (e) { SES.sessoes = []; }
    const el = $("#sesLista"); if (el) el.innerHTML = listaHTML();
  }

  /* ---- Eventos ------------------------------------------------------------- */

  function wireBox() {
    const box = $("#sessaoBox"); if (!box) return;
    box.onclick = (ev) => {
      // só os chips da tela de abertura; os do plano são [data-act="perfil"]
      const chip = ev.target.closest(".ses-chip[data-int]");
      if (chip) {
        // Troca a marcação na mão em vez de re-renderizar: um render aqui
        // reconstrói o box inteiro e apaga o nome do cliente já digitado.
        SES.intencao = chip.dataset.int;
        $$("#sessaoBox .ses-chip").forEach((c) => c.classList.toggle("on", c === chip));
        return;
      }
      const el = ev.target.closest("[data-act]"); if (!el) return;
      const act = el.dataset.act;
      if (act === "iniciar") return iniciar();
      if (act === "retomar") return retomar(el.dataset.id);
      if (act === "encerrar") return pedeEncerrar();
      if (act === "concluir") return etapaAcao("concluir");
      if (act === "pular") return pedePular();
      if (act === "ir") {
        SES.vista = el.dataset.etapa; render();
        // ao entrar em Medir/Provar, checa se há captura de FPS esperando
        if (SES.vista === "medir" || SES.vista === "provar") verFpsPronto(false);
        return;
      }
      if (act === "gerar") return pedirPlano();
      if (act === "perfil") return pedirPlano(el.dataset.int);
      if (act === "marcar") return marcarItem(el.dataset.id);
      if (act === "tecnico") return verTecnico(el.dataset.id);
      if (act === "pagina") return navTo(el.dataset.alvo);
      if (act === "rodar") return rodarFase(+el.dataset.fase, false);
      if (act === "feito") return marcarFeito(el.dataset.id);
      if (act === "medir") return medir(el.dataset.lado, el.dataset.tipo);
      if (act === "irfps") return navTo("fps");
      if (act === "verfps") return verFpsPronto(true);
      if (act === "reiniciar") return pedirReinicio();
      if (act === "pos-reboot") return posReboot();
      if (act === "reboot-visto") return api("/api/sessao/reboot-visto", {}).then(ingest).catch(() => {});
      if (act === "relatorio") return openReport();
      const a = ACOES[SES.vista] || {};
      if (act === "acao" && a.fn) return a.fn();
      if (act === "acao2" && a.alt) return a.alt.fn();
    };
  }

  /* ---- Boot ---------------------------------------------------------------- */

  async function boot() {
    wireBox();
    try { ingest(await api("/api/sessao")); }
    catch (e) { render(); }
    carregaLista();
    // Retomar de verdade: se ficou sessão aberta (o app foi fechado no meio, ou o
    // PC reiniciou), o app abre nela em vez de no Núcleo.
    if (SES.s && !SES.s.encerrada) {
      showPage("sessao");
      toast("info", "Sessão em andamento", "Você parou em " + defOf(SES.s.etapa).nome + ".");
      // Uma fase pode ter ficado "rodando" no arquivo (o app fechou no meio): o
      // status reconcilia isso; aqui só religamos o poll se ainda estiver viva.
      if (SES.s.execucao && SES.s.execucao.estado === "rodando") pollExec();
      if (SES.vista === "medir" || SES.vista === "provar") verFpsPronto(false);
    }
  }

  window.SESSAO = { render, boot, atual: () => SES.s, relatorioHTML, cliente: () => (SES.s && SES.s.cliente && SES.s.cliente.nome) || "" };
  document.addEventListener("DOMContentLoaded", boot);
})();
