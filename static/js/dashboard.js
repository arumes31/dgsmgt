// Dashboard core: server list, cards, actions, detail head, tab switching.
const { esc, t, toast, fmtBytes, relTime } = App;
const $ = (id) => document.getElementById(id);
let ME = null, SERVERS = [], current = null, selected = new Set();
let viewMode = localStorage.getItem('viewmode') || 'grid';
let activeFolder = localStorage.getItem('folder') || '';
let refreshTimer = null;
let sockets = [];

const closeSockets = () => { sockets.forEach(s => { try { s.close(); } catch {} }); sockets = []; };

// ---------- boot ----------
(async () => {
  ME = await App.requireAuth();
  if (!ME) return;
  if (ME.must_change_password) { location.href = '/profile.html#must-change'; return; }
  $('nav').innerHTML = `
    <a class="btn btn-outline" style="justify-content:flex-start" href="/dashboard.html">🖥️ ${t('Servers')}</a>
    ${ME.is_admin ? `<a class="btn btn-outline" style="justify-content:flex-start" href="/admin.html">🛠️ ${t('Admin')}</a>` : ''}
    <a class="btn btn-outline" style="justify-content:flex-start" href="/profile.html">👤 ${t('Profile')}</a>`;
  $('btn-logout').onclick = () => App.logout();
  $('btn-theme').onclick = App.toggleTheme;
  $('btn-lang').onclick = () => App.setLang(App.lang === 'de' ? 'en' : 'de');
  App.initBell($('bell-mount'));
  App.initPalette();
  App.initBanners(ME, $('main'));
  await loadServers();
  refreshTimer = setInterval(() => { if (!$('view-list').classList.contains('hidden')) loadServers(true); }, 15000);
  $('search').addEventListener('input', renderCards);
  $('sort').addEventListener('change', renderCards);
  $('btn-viewmode').onclick = () => { viewMode = viewMode === 'grid' ? 'list' : 'grid'; localStorage.setItem('viewmode', viewMode); renderCards(); };
  document.querySelectorAll('[data-bulk]').forEach(b => b.onclick = () => bulkAction(b.dataset.bulk));
  $('bulk-clear').onclick = () => { selected.clear(); renderCards(); };
  $('crumb-back').onclick = showList;
})();

async function loadServers(silent) {
  try {
    SERVERS = await App.get('/api/my-servers') || [];
    renderCards();
    App.setPaletteItems([
      ...SERVERS.map(s => ({ label: `Open: ${s.name}`, hint: s.status, run: () => openDetail(s.ID) })),
      ...SERVERS.filter(s => s.can_restart).map(s => ({ label: `Restart: ${s.name}`, hint: 'action', run: () => runAction(s, 'restart') })),
      { label: 'Go to Admin', hint: 'g a', run: () => location.href = '/admin.html' },
      { label: 'Go to Profile', hint: 'g p', run: () => location.href = '/profile.html' },
    ]);
    renderFolders();
  } catch (e) { if (!silent) toast(e.message, 'error'); }
}

function renderFolders() {
  const folders = [...new Set(SERVERS.map(s => s.folder).filter(Boolean))].sort();
  $('folders').innerHTML = folders.length ? `<div class="form-label">Folders</div>` +
    [``, ...folders].map(f => `<div class="chip" style="display:flex;margin-bottom:6px;${f===activeFolder?'border-color:var(--accent)':''}" data-f="${esc(f)}">${f ? '📁 ' + esc(f) : '🗂️ All'}</div>`).join('') : '';
  $('folders').querySelectorAll('[data-f]').forEach(n => n.onclick = () => {
    activeFolder = n.dataset.f; localStorage.setItem('folder', activeFolder); renderCards(); renderFolders();
  });
}

function statusClass(s) {
  if (s === 'running') return 'status-online';
  if (s === 'paused') return 'status-paused';
  if (s === 'restarting') return 'status-warn';
  return 'status-offline';
}

function renderCards() {
  const q = $('search').value.toLowerCase();
  const sort = $('sort').value;
  let list = SERVERS.filter(s =>
    (!q || s.name.toLowerCase().includes(q) || (s.image || '').toLowerCase().includes(q) || (s.status || '').includes(q)) &&
    (!activeFolder || s.folder === activeFolder));
  list.sort((a, b) => sort === 'status' ? (a.status || '').localeCompare(b.status || '') || a.name.localeCompare(b.name)
    : sort === 'folder' ? (a.folder || '').localeCompare(b.folder || '') || a.name.localeCompare(b.name)
    : a.name.localeCompare(b.name));

  const c = $('cards');
  c.classList.toggle('list-view', viewMode === 'list');
  if (!SERVERS.length) {
    c.innerHTML = `<div class="empty-state glass" style="grid-column:1/-1;border-radius:var(--radius-md)">
      <h3>${t('No servers yet')}</h3><p>${ME.is_admin ? 'Create your first server from a game template in the admin area.' : 'Ask an admin to assign servers to you.'}</p>
      ${ME.is_admin ? '<a class="btn btn-primary mt" href="/admin.html#servers">+ Create server</a>' : ''}</div>`;
    return;
  }
  c.innerHTML = list.map(s => {
    const health = s.health && s.health.status !== 'none' ?
      `<span class="badge ${s.health.status === 'healthy' ? 'badge-success' : s.health.status === 'starting' ? 'badge-warn' : 'badge-danger'}">${esc(s.health.status)}</span>` : '';
    const ports = (s.ports || []).slice(0, 3).map(p => `<span class="chip" data-copy="${esc(p)}" title="Copy">${esc(p)}</span>`).join(' ');
    return `<div class="glass glass-card" style="border-radius:var(--radius-md);padding:22px;position:relative;cursor:pointer" data-open="${s.ID}">
      <input type="checkbox" class="card-select" data-sel="${s.ID}" ${selected.has(s.ID) ? 'checked' : ''}>
      <div style="display:flex;gap:14px;align-items:flex-start;margin-left:26px">
        <div class="icon-emoji" style="width:44px;height:44px;font-size:22px">${App.gameIcon(s.icon, s.image)}</div>
        <div class="grow" style="min-width:0">
          <div style="display:flex;justify-content:space-between;gap:8px;align-items:center">
            <b style="font-size:16px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(s.name)}</b>
            <span class="status-indicator ${statusClass(s.status)}"><span class="status-dot"></span>${t(esc(s.status || 'offline'))}</span>
          </div>
          <div class="small" style="margin:2px 0 8px">${esc(s.image || '')} ${s.folder ? '· 📁 ' + esc(s.folder) : ''}</div>
          <div class="flex flex-wrap" style="gap:6px">${ports} ${health} ${s.uptime ? `<span class="small">⏱ ${esc(s.uptime)}</span>` : ''}</div>
        </div>
      </div>
      <div class="flex" style="margin-top:16px;margin-left:26px" data-noopen>
        ${s.can_start ? `<button class="btn btn-outline" data-act="start" data-id="${s.ID}">▶</button>` : ''}
        ${s.can_restart ? `<button class="btn btn-outline" data-act="restart" data-id="${s.ID}">⟳</button>` : ''}
        ${s.can_stop ? `<button class="btn btn-danger" data-act="stop" data-id="${s.ID}">■</button>` : ''}
        ${s.can_view_logs ? `<button class="btn btn-outline" data-act="console" data-id="${s.ID}">🖳 ${t('Console')}</button>` : ''}
      </div>
    </div>`;
  }).join('');

  c.querySelectorAll('[data-open]').forEach(n => n.onclick = (e) => {
    if (e.target.closest('[data-noopen]') || e.target.closest('[data-sel]') || e.target.closest('[data-copy]')) return;
    openDetail(+n.dataset.open);
  });
  c.querySelectorAll('[data-copy]').forEach(n => n.onclick = (e) => { e.stopPropagation(); App.copy(n.dataset.copy); });
  c.querySelectorAll('[data-sel]').forEach(n => n.onchange = () => {
    n.checked ? selected.add(+n.dataset.sel) : selected.delete(+n.dataset.sel);
    updateBulkbar();
  });
  c.querySelectorAll('[data-act]').forEach(n => n.onclick = async (e) => {
    e.stopPropagation();
    const s = SERVERS.find(x => x.ID === +n.dataset.id);
    if (n.dataset.act === 'console') { openDetail(s.ID, 'console'); return; }
    runAction(s, n.dataset.act, n);
  });
  updateBulkbar();
}

function updateBulkbar() {
  $('bulkbar').classList.toggle('hidden', selected.size === 0);
  $('bulk-count').textContent = `${selected.size} selected`;
}

async function bulkAction(action) {
  const res = await App.post('/api/bulk-action', { server_ids: [...selected], action }).catch(e => { toast(e.message, 'error'); });
  if (res) {
    const fails = res.filter(r => !r.ok);
    toast(fails.length ? `${action}: ${fails.length} failed` : `${action}: all OK`, fails.length ? 'error' : 'success');
  }
  selected.clear();
  loadServers();
}

async function runAction(s, action, btn) {
  if (action === 'stop' && !(await App.confirmDialog('Stop server', `Stop <b>${esc(s.name)}</b>?`))) return;
  const orig = btn ? btn.textContent : '';
  if (btn) { btn.disabled = true; btn.textContent = '…'; }
  // optimistic status
  s.status = action === 'stop' ? 'stopping…' : action === 'start' ? 'starting…' : 'restarting…';
  renderCards();
  try {
    await App.post(`/api/action/${s.container_id}/${action}`);
    toast(`${s.name}: ${action} OK`, 'success');
  } catch (e) { toast(e.message, 'error'); }
  if (btn) { btn.disabled = false; btn.textContent = orig; }
  setTimeout(() => loadServers(true), 1200);
  if (current && current.ID === s.ID) setTimeout(() => refreshDetailHead(), 1500);
}

// ================= Detail view =================
function showList() {
  closeSockets();
  current = null;
  $('view-detail').classList.add('hidden');
  $('view-list').classList.remove('hidden');
  loadServers(true);
}

async function openDetail(id, tab) {
  current = SERVERS.find(s => s.ID === id);
  if (!current) return;
  $('view-list').classList.add('hidden');
  $('view-detail').classList.remove('hidden');
  $('d-icon').textContent = App.gameIcon(current.icon, current.image);
  $('d-name').textContent = current.name;
  refreshDetailHead();

  const tabs = [];
  if (current.can_view_logs) tabs.push(['console', t('Console')]);
  tabs.push(['metrics', t('Metrics')]);
  if (current.can_access_files) tabs.push(['files', t('Files')]);
  if (current.can_manage_backups) tabs.push(['backups', t('Backups')]);
  tabs.push(['activity', t('Activity')]);
  if (current.can_edit_config) tabs.push(['settings', t('Settings')]);
  $('d-tabs').innerHTML = tabs.map(([id, label]) => `<button class="tab" data-tab="${id}">${label}</button>`).join('');
  $('d-tabs').querySelectorAll('.tab').forEach(n => n.onclick = () => switchTab(n.dataset.tab));
  switchTab(tab || tabs[0][0]);
}

async function refreshDetailHead() {
  if (!current) return;
  try {
    const info = await App.get(`/api/status/${current.container_id}`);
    const st = $('d-status');
    st.className = 'status-indicator ' + statusClass(info.state);
    st.innerHTML = `<span class="status-dot"></span>${t(esc(info.state))}` +
      (info.oom_killed ? ' <span class="badge badge-danger">OOM</span>' : '') +
      (info.restart_count > 0 ? ` <span class="badge badge-warn">${info.restart_count} restarts</span>` : '') +
      (info.health && info.health.status !== 'none' ? ` <span class="badge ${info.health.status==='healthy'?'badge-success':'badge-danger'}">${esc(info.health.status)}</span>` : '');
    $('d-uptime').textContent = info.uptime ? `⏱ ${info.uptime}` : '';
    $('d-connect').innerHTML = (info.ports || []).map(p => `<span class="chip" title="Copy connect info">${esc(p)}</span>`).join('');
    $('d-connect').querySelectorAll('.chip').forEach(n => n.onclick = () => App.copy(n.textContent));
    App.get(`/api/metrics/${current.container_id}/availability`).then(a =>
      $('d-avail').textContent = `📈 ${a.availability_percent.toFixed(1)}% uptime (${a.days}d)`).catch(() => {});
  } catch { $('d-status').innerHTML = `<span class="status-dot"></span>offline`; $('d-status').className = 'status-indicator status-offline'; }

  const A = [];
  if (current.can_start) A.push(['start', '▶ ' + t('Start'), 'btn-outline']);
  if (current.can_restart) A.push(['restart', '⟳ ' + t('Restart'), 'btn-outline']);
  if (current.can_stop) { A.push(['stop', '■ ' + t('Stop'), 'btn-danger']); A.push(['kill', '☠ Kill', 'btn-danger']); A.push(['pause', '⏸', 'btn-outline']); A.push(['unpause', '⏵', 'btn-outline']); }
  $('d-actions').innerHTML = A.map(([a, l, c]) => `<button class="btn ${c}" data-dact="${a}">${l}</button>`).join('');
  $('d-actions').querySelectorAll('[data-dact]').forEach(n => n.onclick = () => runAction(current, n.dataset.dact, n));
}

function switchTab(tab) {
  closeSockets();
  $('d-tabs').querySelectorAll('.tab').forEach(n => n.classList.toggle('active', n.dataset.tab === tab));
  const c = $('tab-content');
  ({ console: tabConsole, metrics: tabMetrics, files: tabFiles, backups: tabBackups, activity: tabActivity, settings: tabSettings })[tab](c);
}
