/* Ícones SVG line-style (stroke), consistentes em 24x24. Sem dependências. */
(function () {
  const S = (p, extra) =>
    `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="var(--icon-stroke, 1.8)" ` +
    `stroke-linecap="round" stroke-linejoin="round" ${extra || ""}>${p}</svg>`;

  const ICONS = {
    scan: S('<path d="M3 7V5a2 2 0 0 1 2-2h2M17 3h2a2 2 0 0 1 2 2v2M21 17v2a2 2 0 0 1-2 2h-2M7 21H5a2 2 0 0 1-2-2v-2"/><path d="M3 12h18"/>'),
    bolt: S('<path d="M13 2 4.5 13.5H11l-1 8.5L19.5 10H13z" fill="currentColor" stroke="none"/>'),
    check: S('<path d="M20 6 9 17l-5-5"/>'),
    undo: S('<path d="M3 7v6h6"/><path d="M3 13a9 9 0 1 0 3-7.7L3 8"/>'),
    warn: S('<path d="M12 9v4M12 17h.01"/><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/>'),
    search: S('<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>'),
    ring: S('<path d="M21 12a9 9 0 1 1-6.2-8.5" class="spin"/>', 'class="spin"'),
    cpu: S('<rect x="6" y="6" width="12" height="12" rx="2"/><path d="M9 2v2M15 2v2M9 20v2M15 20v2M2 9h2M2 15h2M20 9h2M20 15h2"/><rect x="9.5" y="9.5" width="5" height="5" rx="1"/>'),
    ram: S('<rect x="2" y="7" width="20" height="10" rx="2"/><path d="M6 17v3M10 17v3M14 17v3M18 17v3M7 10v3M11 10v3M15 10v3"/>'),
    disk: S('<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="12" cy="12" r="3.4"/><circle cx="12" cy="12" r=".6" fill="currentColor"/><path d="M17.5 17.5 19 19"/>'),
    display: S('<rect x="2.5" y="4" width="19" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>'),
    network: S('<path d="M12 20v-6"/><circle cx="12" cy="4" r="2"/><circle cx="5" cy="20" r="2"/><circle cx="19" cy="20" r="2"/><path d="M12 6v3a3 3 0 0 1-3 3H7M12 9a3 3 0 0 0 3 3h2"/>'),
    battery: S('<rect x="2" y="7" width="17" height="10" rx="2.5"/><path d="M22 10v4"/><path d="M6 10v4M9.5 10v4"/>'),
    plug: S('<path d="M9 2v6M15 2v6M7 8h10v3a5 5 0 0 1-10 0V8zM12 16v6"/>'),
    history: S('<path d="M3 3v6h6"/><path d="M3 9a9 9 0 1 1-1 5"/><path d="M12 8v4l3 2"/>'),
    guide: S('<circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 1 1 3.5 2.3c-.8.4-1 .8-1 1.7M12 17h.01"/>'),
    ok: S('<circle cx="12" cy="12" r="9"/><path d="m8.5 12 2.3 2.3 4.7-4.6"/>'),
    err: S('<circle cx="12" cy="12" r="9"/><path d="M15 9l-6 6M9 9l6 6"/>'),
    info: S('<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>'),
    empty: S('<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18M8 14h8"/>'),
    home: S('<path d="M3 11.5 12 4l9 7.5"/><path d="M5 10v9a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-9"/><path d="M9.5 20v-6h5v6"/>'),
    broom: S('<path d="M19 4 11 12"/><path d="M15.5 7.5 9 14l-3.5 5.5L4 21M9 14l2.5 6"/><path d="M3 21c2-1.5 3.5-1.5 5.5-3"/>'),
    shield: S('<path d="M12 3 5 6v5c0 4.4 2.9 7.8 7 9 4.1-1.2 7-4.6 7-9V6l-7-3z"/><path d="m9.5 12 1.8 1.8 3.3-3.5"/>'),
    /* energy removido — idêntico a bolt; usar data-icon="bolt" */
    game: S('<rect x="2" y="7" width="20" height="10" rx="4"/><path d="M7 11v2M6 12h2M16 11.5h.01M18 13.5h.01"/>'),
    gpu: S('<rect x="3" y="5" width="18" height="12" rx="2"/><circle cx="9" cy="11" r="2.4"/><path d="M14 9h4M14 13h4M7 17v3"/>'),
    mem: S('<rect x="2" y="7" width="20" height="10" rx="2"/><path d="M6 17v3M10 17v3M14 17v3M18 17v3M7 10v3M11 10v3M15 10v3"/>'),
    netcat: S('<rect x="3" y="9" width="18" height="6" rx="2"/><path d="M7 9V6M17 9V6M7 18v-3M17 18v-3"/>'),
    svc: S('<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2"/>'),
    sys: S('<rect x="4" y="4" width="16" height="16" rx="2"/><path d="M9 9h6v6H9z"/><path d="M9 2v2M15 2v2M9 20v2M15 20v2M2 9h2M2 15h2M20 9h2M20 15h2"/>'),
    perf: S('<path d="M12 14a6 6 0 0 1 6-6M12 14l4-4"/><path d="M4.2 17a9 9 0 1 1 15.6 0"/><circle cx="12" cy="14" r="1.4" fill="currentColor"/>'),
    rocket: S('<path d="M5 14c-1.5 1-2 4-2 4s3-.5 4-2c.6-.9.5-2-.3-2.8A2 2 0 0 0 5 14z"/><path d="M9 13a14 14 0 0 1 6-9c2.5-1 5-1 5-1s0 2.5-1 5a14 14 0 0 1-9 6z"/><path d="M9 13 7 11M11 15l2 2"/><circle cx="15" cy="9" r="1.4"/>'),
    activity: S('<path d="M3 12h4l2.5-7 5 14L17 12h4"/>'),
    gauge: S('<path d="M4.2 17a9 9 0 1 1 15.6 0"/><path d="M12 13l4.5-3.5"/><circle cx="12" cy="13" r="1.3" fill="currentColor"/><path d="M6 13h0M18 13h0M8 8h0M16 8h0"/>'),
    diag: S('<circle cx="11" cy="11" r="7"/><path d="m20.5 20.5-3.6-3.6"/><path d="M7.5 11H9l1-2.2 2 4.4 1-2.2h1.5"/>'),
    wrench: S('<path d="M14.5 5.5a3.8 3.8 0 0 0-5 5L4 16v4h4l5.5-5.5a3.8 3.8 0 0 0 5-5l-2.5 2.5-2.5-.5-.5-2.5z"/>'),
    trash: S('<path d="M4 7h16M9 7V5h6v2M6 7l1 13h10l1-13"/>'),
    doc: S('<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M8 13h8M8 17h6"/>'),
    print: S('<path d="M6 9V3h12v6"/><rect x="5" y="9" width="14" height="8" rx="1"/><path d="M7 17h10v4H7z"/>'),
    backup: S('<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v6c0 1.7 3.6 3 8 3s8-1.3 8-3V6"/><path d="M4 12v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>'),
    gamepad: S('<rect x="2" y="7" width="20" height="10" rx="4"/><path d="M7 11v2M6 12h2"/><circle cx="15.5" cy="11.5" r=".6" fill="currentColor"/><circle cx="17.5" cy="13.5" r=".6" fill="currentColor"/>'),
    refresh: S('<path d="M1 4v6h6"/><path d="M23 20v-6h-6"/><path d="M20.5 9A9 9 0 0 0 5.2 5.2L1 10M23 14l-4.2 4.8A9 9 0 0 1 3.5 15"/>'),
    overlay: S('<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M9 20h6M12 16v4"/><path d="M8 9h2M10 9v2"/><path d="M14 10h4M14 12h3"/>'),
    update: S('<path d="M12 2v13M7 10l5 6 5-6"/><path d="M3 20h18"/>'),
  };

  window.ICONS = ICONS;

  // Popula elementos com data-icon (substitui conteudo).
  document.querySelectorAll("[data-icon]").forEach((el) => {
    const ic = ICONS[el.dataset.icon];
    if (ic) el.innerHTML = ic;
  });
  // data-icon-left: insere um icone antes do texto (nav).
  document.querySelectorAll("[data-icon-left]").forEach((el) => {
    const ic = ICONS[el.dataset.iconLeft];
    if (ic) el.insertAdjacentHTML("afterbegin", `<span class="nav-ic">${ic}</span>`);
  });
})();
