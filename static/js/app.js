/**
 * DGSMgt v2 core: API client with refresh-token handling, i18n (en/de),
 * theme, toasts, modals, command palette, notifications bell, shortcuts.
 */
const App = (() => {
  // ---------- i18n ----------
  const dict = {
    de: {
      'Dashboard': 'Übersicht', 'Servers': 'Server', 'Users': 'Benutzer', 'Groups': 'Gruppen',
      'Settings': 'Einstellungen', 'Logout': 'Abmelden', 'Login': 'Anmelden', 'Search': 'Suchen',
      'Start': 'Starten', 'Stop': 'Stoppen', 'Restart': 'Neustarten', 'Kill': 'Beenden (Kill)',
      'Pause': 'Pausieren', 'Resume': 'Fortsetzen', 'Console': 'Konsole', 'Files': 'Dateien',
      'Backups': 'Backups', 'Activity': 'Aktivität', 'Metrics': 'Metriken', 'Delete': 'Löschen',
      'Create': 'Erstellen', 'Save': 'Speichern', 'Cancel': 'Abbrechen', 'Upload': 'Hochladen',
      'Download': 'Herunterladen', 'Restore': 'Wiederherstellen', 'Name': 'Name', 'Image': 'Image',
      'running': 'läuft', 'exited': 'gestoppt', 'offline': 'offline', 'paused': 'pausiert',
      'Password': 'Passwort', 'Username': 'Benutzername', 'Profile': 'Profil',
      'Notifications': 'Benachrichtigungen', 'Mark all read': 'Alle gelesen',
      'No servers yet': 'Noch keine Server', 'Copied!': 'Kopiert!', 'Uptime': 'Laufzeit',
      'Admin': 'Verwaltung', 'Trash': 'Papierkorb', 'Audit log': 'Audit-Log', 'Nodes': 'Nodes',
      'Confirm': 'Bestätigen', 'Loading...': 'Lädt...', 'Never': 'Nie',
    }
  };
  let lang = localStorage.getItem('lang') || (navigator.language || 'en').slice(0, 2);
  const t = (s) => (dict[lang] && dict[lang][s]) || s;
  const setLang = (l) => { lang = l; localStorage.setItem('lang', l); location.reload(); };

  // ---------- Theme ----------
  const applyTheme = () => {
    const saved = localStorage.getItem('theme');
    const theme = saved || (window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
    document.documentElement.dataset.theme = theme;
  };
  const toggleTheme = () => {
    const cur = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
    localStorage.setItem('theme', cur);
    applyTheme();
  };
  applyTheme();

  // ---------- Helpers ----------
  const esc = (s) => (s ?? '').toString().replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const fmtBytes = (b) => {
    if (b == null || isNaN(b)) return '–';
    const u = ['B', 'KB', 'MB', 'GB', 'TB']; let i = 0; b = Number(b);
    while (b >= 1024 && i < u.length - 1) { b /= 1024; i++; }
    return b.toFixed(i ? 1 : 0) + ' ' + u[i];
  };
  const relTime = (iso) => {
    if (!iso) return t('Never');
    const d = new Date(iso); const s = (Date.now() - d.getTime()) / 1000;
    const abs = d.toLocaleString();
    let rel;
    if (s < 60) rel = 'just now';
    else if (s < 3600) rel = Math.floor(s / 60) + 'm ago';
    else if (s < 86400) rel = Math.floor(s / 3600) + 'h ago';
    else rel = Math.floor(s / 86400) + 'd ago';
    return `<span title="${esc(abs)}">${rel}</span>`;
  };
  const copy = async (text) => {
    try { await navigator.clipboard.writeText(text); toast(t('Copied!'), 'success'); }
    catch { toast('Copy failed', 'error'); }
  };

  // ---------- Toasts ----------
  const toast = (msg, type = 'info') => {
    let c = document.getElementById('toast-container');
    if (!c) { c = document.createElement('div'); c.id = 'toast-container'; document.body.appendChild(c); }
    const el = document.createElement('div');
    el.className = `toast toast-${type}`;
    el.textContent = msg;
    c.appendChild(el);
    requestAnimationFrame(() => el.classList.add('show'));
    setTimeout(() => { el.classList.remove('show'); setTimeout(() => el.remove(), 500); }, 4200);
  };

  // ---------- Modal helpers ----------
  const confirmDialog = (title, body, { typed = null, danger = true } = {}) => new Promise((resolve) => {
    const dlg = document.createElement('dialog');
    dlg.className = 'modal-dialog';
    dlg.innerHTML = `<div class="modal-content" style="max-width:480px;width:92vw">
      <div class="modal-header"><h3>${esc(title)}</h3></div>
      <div class="modal-body"><p class="text-secondary">${body}</p>
        ${typed ? `<p class="small mt">Type <b>${esc(typed)}</b> to confirm:</p>
        <input class="form-input mt" id="cf-typed" autocomplete="off">` : ''}</div>
      <div class="modal-footer">
        <button class="btn btn-outline" id="cf-no">${t('Cancel')}</button>
        <button class="btn ${danger ? 'btn-danger' : 'btn-primary'}" id="cf-yes" ${typed ? 'disabled' : ''}>${t('Confirm')}</button>
      </div></div>`;
    document.body.appendChild(dlg);
    dlg.showModal();
    const yes = dlg.querySelector('#cf-yes');
    if (typed) dlg.querySelector('#cf-typed').addEventListener('input', (e) => { yes.disabled = e.target.value !== typed; });
    const done = (v) => { dlg.close(); dlg.remove(); resolve(v); };
    yes.onclick = () => done(true);
    dlg.querySelector('#cf-no').onclick = () => done(false);
    dlg.addEventListener('cancel', () => done(false));
  });

  const modal = (title, bodyHTML, footHTML = '') => {
    const dlg = document.createElement('dialog');
    dlg.className = 'modal-dialog';
    dlg.innerHTML = `<div class="modal-content" style="max-width:720px;width:94vw">
      <div class="modal-header"><h3>${esc(title)}</h3>
        <button class="btn btn-outline btn-icon" data-close>✕</button></div>
      <div class="modal-body">${bodyHTML}</div>
      ${footHTML ? `<div class="modal-footer">${footHTML}</div>` : ''}</div>`;
    document.body.appendChild(dlg);
    dlg.querySelector('[data-close]').onclick = () => { dlg.close(); dlg.remove(); };
    dlg.addEventListener('cancel', () => dlg.remove());
    dlg.showModal();
    return dlg;
  };

  // ---------- Auth / API ----------
  const tokens = {
    get access() { return localStorage.getItem('token'); },
    set access(v) { v ? localStorage.setItem('token', v) : localStorage.removeItem('token'); },
    get refresh() { return localStorage.getItem('refresh_token'); },
    set refresh(v) { v ? localStorage.setItem('refresh_token', v) : localStorage.removeItem('refresh_token'); },
  };

  let refreshing = null;
  const tryRefresh = async () => {
    if (!tokens.refresh) return false;
    if (refreshing) return refreshing;
    refreshing = (async () => {
      try {
        const res = await fetch('/api/refresh', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: tokens.refresh })
        });
        if (!res.ok) return false;
        const j = await res.json();
        tokens.access = j.data.token;
        tokens.refresh = j.data.refresh_token;
        return true;
      } catch { return false; } finally { setTimeout(() => refreshing = null, 100); }
    })();
    return refreshing;
  };

  const logout = async (redirect = true) => {
    try {
      if (tokens.refresh) await fetch('/api/logout', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + tokens.access },
        body: JSON.stringify({ refresh_token: tokens.refresh })
      });
    } catch { /* ignore */ }
    tokens.access = null; tokens.refresh = null;
    if (redirect) location.href = '/login.html';
  };

  const api = async (url, options = {}, retry = true) => {
    const headers = { 'Authorization': 'Bearer ' + tokens.access, ...(options.headers || {}) };
    if (options.body && typeof options.body === 'string') headers['Content-Type'] = 'application/json';
    let res;
    try { res = await fetch(url, { ...options, headers }); }
    catch { throw new Error('Network error: cannot reach the server'); }
    if (res.status === 401 && retry) {
      if (await tryRefresh()) return api(url, options, false);
      logout();
      throw new Error('Session expired');
    }
    if (res.status === 204) return true;
    let j = null;
    try { j = await res.json(); } catch { /* non-json */ }
    if (!res.ok) throw new Error((j && j.error) || ('Request failed: ' + res.status));
    return j ? { data: j.data, meta: j.meta } : {};
  };
  const get = async (u) => (await api(u)).data;
  const getFull = (u) => api(u);
  const post = async (u, d) => (await api(u, { method: 'POST', body: JSON.stringify(d ?? {}) })).data;
  const put = async (u, d) => (await api(u, { method: 'PUT', body: JSON.stringify(d ?? {}) })).data;
  const del = (u) => api(u, { method: 'DELETE' });
  const upload = async (u, formData) => (await api(u, { method: 'POST', body: formData })).data;

  // Authenticated file download (fetch + blob, keeps Authorization header)
  const download = async (url, filename) => {
    const res = await fetch(url, { headers: { 'Authorization': 'Bearer ' + tokens.access } });
    if (!res.ok) { toast('Download failed', 'error'); return; }
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = filename || 'download';
    a.click();
    URL.revokeObjectURL(a.href);
  };

  const ws = (path) => {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const sep = path.includes('?') ? '&' : '?';
    return new WebSocket(`${proto}//${location.host}${path}${sep}token=${encodeURIComponent(tokens.access)}`);
  };

  const requireAuth = async () => {
    if (!tokens.access && !(await tryRefresh())) { location.href = '/login.html'; return null; }
    try { return await get('/api/me'); }
    catch { location.href = '/login.html'; return null; }
  };

  // ---------- Notification bell ----------
  let bellTimer = null;
  const initBell = (mount) => {
    mount.innerHTML = `<div class="bell-wrap">
      <button class="btn btn-outline btn-icon" id="bell-btn" title="${t('Notifications')}">🔔<span class="bell-dot hidden" id="bell-dot"></span></button>
      <div class="notif-panel glass" id="notif-panel">
        <div style="display:flex;justify-content:space-between;align-items:center;padding:14px 18px;border-bottom:1px solid var(--glass-border)">
          <b>${t('Notifications')}</b>
          <button class="btn btn-outline" style="padding:4px 12px;font-size:12px" id="bell-read">${t('Mark all read')}</button>
        </div><div id="notif-list"></div></div></div>`;
    const panel = mount.querySelector('#notif-panel');
    mount.querySelector('#bell-btn').onclick = () => panel.classList.toggle('open');
    mount.querySelector('#bell-read').onclick = async () => { await post('/api/notifications/read'); pollBell(); };
    document.addEventListener('click', (e) => { if (!mount.contains(e.target)) panel.classList.remove('open'); });
    const pollBell = async () => {
      try {
        const { data, meta } = await getFull('/api/notifications');
        const dot = mount.querySelector('#bell-dot');
        const unread = meta ? meta.unread : 0;
        dot.classList.toggle('hidden', !unread);
        dot.textContent = unread > 9 ? '9+' : unread;
        setFaviconBadge(unread > 0);
        mount.querySelector('#notif-list').innerHTML = (data || []).map(n =>
          `<div class="notif-item ${n.read_at ? '' : 'unread'}">
            <div style="display:flex;justify-content:space-between"><b>${esc(n.title)}</b>
            <span class="small">${relTime(n.CreatedAt || n.created_at)}</span></div>
            <div class="text-secondary">${esc(n.body)}</div></div>`).join('') ||
          `<div class="notif-item text-muted">No notifications</div>`;
      } catch { /* ignore */ }
    };
    pollBell();
    bellTimer = setInterval(pollBell, 30000);
  };

  // Favicon badge (red dot when something needs attention)
  let baseFavicon = null;
  const setFaviconBadge = (on) => {
    const link = document.querySelector('link[rel="icon"]');
    if (!link) return;
    if (!baseFavicon) baseFavicon = link.href;
    if (!on) { link.href = baseFavicon; return; }
    const img = new Image();
    img.onload = () => {
      const c = document.createElement('canvas'); c.width = c.height = 32;
      const ctx = c.getContext('2d');
      ctx.drawImage(img, 0, 0, 32, 32);
      ctx.fillStyle = '#ef4444'; ctx.beginPath(); ctx.arc(25, 7, 6, 0, 7); ctx.fill();
      link.href = c.toDataURL('image/png');
    };
    img.src = baseFavicon;
  };

  // ---------- Command palette + shortcuts ----------
  let palItems = [];
  const setPaletteItems = (items) => { palItems = items; };
  const initPalette = () => {
    if (document.getElementById('palette')) return;
    const el = document.createElement('div');
    el.id = 'palette';
    el.innerHTML = `<div class="pal-box glass"><input placeholder="${t('Search')}… (servers, actions)" id="pal-input">
      <div class="pal-list" id="pal-list"></div></div>`;
    document.body.appendChild(el);
    const input = el.querySelector('#pal-input');
    const list = el.querySelector('#pal-list');
    let sel = 0, shown = [];
    const render = () => {
      const q = input.value.toLowerCase();
      shown = palItems.filter(i => i.label.toLowerCase().includes(q)).slice(0, 12);
      sel = Math.min(sel, Math.max(0, shown.length - 1));
      list.innerHTML = shown.map((i, idx) =>
        `<div class="pal-item ${idx === sel ? 'sel' : ''}" data-i="${idx}"><span>${esc(i.label)}</span><span class="small">${esc(i.hint || '')}</span></div>`).join('');
      list.querySelectorAll('.pal-item').forEach(n => n.onclick = () => { close(); shown[+n.dataset.i].run(); });
    };
    const close = () => el.classList.remove('open');
    const open = () => { el.classList.add('open'); input.value = ''; sel = 0; render(); input.focus(); };
    input.addEventListener('input', render);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowDown') { sel++; render(); e.preventDefault(); }
      else if (e.key === 'ArrowUp') { sel = Math.max(0, sel - 1); render(); e.preventDefault(); }
      else if (e.key === 'Enter' && shown[sel]) { close(); shown[sel].run(); }
      else if (e.key === 'Escape') close();
    });
    el.addEventListener('click', (e) => { if (e.target === el) close(); });

    let gPressed = false;
    document.addEventListener('keydown', (e) => {
      const inField = ['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName);
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); open(); return; }
      if (inField) return;
      if (e.key === '/') { e.preventDefault(); document.getElementById('search')?.focus(); }
      else if (e.key === '?') { showHelp(); }
      else if (e.key.toLowerCase() === 'g') { gPressed = true; setTimeout(() => gPressed = false, 800); }
      else if (gPressed && e.key.toLowerCase() === 'd') { location.href = '/dashboard.html'; }
      else if (gPressed && e.key.toLowerCase() === 'a') { location.href = '/admin.html'; }
      else if (gPressed && e.key.toLowerCase() === 'p') { location.href = '/profile.html'; }
    });
  };
  const showHelp = () => modal('Keyboard shortcuts', `
    <div class="help-row"><span>Command palette</span><span><kbd>Ctrl</kbd>+<kbd>K</kbd></span></div>
    <div class="help-row"><span>${t('Search')}</span><span><kbd>/</kbd></span></div>
    <div class="help-row"><span>${t('Dashboard')}</span><span><kbd>g</kbd> <kbd>d</kbd></span></div>
    <div class="help-row"><span>${t('Admin')}</span><span><kbd>g</kbd> <kbd>a</kbd></span></div>
    <div class="help-row"><span>${t('Profile')}</span><span><kbd>g</kbd> <kbd>p</kbd></span></div>
    <div class="help-row"><span>This help</span><span><kbd>?</kbd></span></div>`);

  // ---------- Maintenance banner + push ----------
  const initBanners = async (me, mount) => {
    try {
      const s = await get('/api/settings/public');
      if (s.maintenance_message) {
        mount.insertAdjacentHTML('afterbegin',
          `<div class="banner banner-warn">🛠️ ${esc(s.maintenance_message)}</div>`);
      }
      window.__vapid = s.vapid_public_key;
      if (me && me.is_admin) {
        const u = await get('/api/admin/update-check').catch(() => null);
        if (u && u.update_available) {
          mount.insertAdjacentHTML('afterbegin',
            `<div class="banner banner-info">⬆️ Update available: <b>${esc(u.latest)}</b> (current ${esc(u.current)}) — <a href="${esc(u.url)}" target="_blank" rel="noopener" style="color:inherit">release notes</a></div>`);
        }
      }
    } catch { /* ignore */ }
  };

  const enablePush = async () => {
    if (!('serviceWorker' in navigator) || !window.__vapid) { toast('Push not available', 'error'); return false; }
    const reg = await navigator.serviceWorker.register('/sw.js');
    const perm = await Notification.requestPermission();
    if (perm !== 'granted') { toast('Permission denied', 'error'); return false; }
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlB64(window.__vapid)
    });
    const j = sub.toJSON();
    await post('/api/push/subscribe', { endpoint: j.endpoint, p256dh: j.keys.p256dh, auth: j.keys.auth });
    toast('Browser notifications enabled', 'success');
    return true;
  };
  const urlB64 = (s) => {
    const pad = '='.repeat((4 - s.length % 4) % 4);
    const b64 = (s + pad).replace(/-/g, '+').replace(/_/g, '/');
    const raw = atob(b64); const arr = new Uint8Array(raw.length);
    for (let i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
    return arr;
  };

  // Cron helper: human-friendly presets used by cron builder selects
  const cronPresets = [
    { label: '— none —', value: '' },
    { label: 'Every day at 04:00', value: '0 4 * * *' },
    { label: 'Every day at 06:00', value: '0 6 * * *' },
    { label: 'Every 6 hours', value: '0 */6 * * *' },
    { label: 'Every hour', value: '0 * * * *' },
    { label: 'Every Sunday 05:00', value: '0 5 * * 0' },
    { label: 'Custom…', value: 'custom' },
  ];

  const gameIcon = (icon, image) => {
    const map = {
      minecraft: '⛏️', valheim: '🛶', ark: '🦖', rust: '🔧', palworld: '🐣', terraria: '🌳',
      factorio: '🏭', satisfactory: '⚙️', zomboid: '🧟', '7dtd': '☠️', dst: '🍖', vrising: '🧛',
      enshrouded: '🌫️', starbound: '🌌', spaceengineers: '🚀', cs2: '🔫', csgo: '🔫', css: '🔫',
      cs16: '🔫', tf2: '🎯', l4d2: '🧟', gmod: '🔨', arma3: '🎖️', dayz: '🩸', insurgency: '💥',
      sandstorm: '🏜️', mordhau: '⚔️', quake: '👹', ut: '🛸', assetto: '🏎️', openttd: '🚂',
      mindustry: '🗼', wesnoth: '🛡️', teeworlds: '🎯', veloren: '🏔️', linuxgsm: '🐧',
    };
    if (icon && map[icon]) return map[icon];
    const img = (image || '').toLowerCase();
    for (const k of Object.keys(map)) if (img.includes(k)) return map[k];
    return '🎮';
  };

  return {
    t, setLang, lang, esc, fmtBytes, relTime, copy, toast, confirmDialog, modal,
    tokens, api, get, getFull, post, put, del, upload, download, ws, requireAuth, logout,
    initBell, initPalette, setPaletteItems, initBanners, enablePush, cronPresets, gameIcon,
    toggleTheme, showHelp,
  };
})();
