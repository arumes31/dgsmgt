// Admin core: boot, navigation, overview, users.
const { esc, t, toast, fmtBytes, relTime } = App;
const $ = (id) => document.getElementById(id);
const C = () => $('content');
let ME, USERS = [], SERVERS = [], GROUPS = [];

(async () => {
  ME = await App.requireAuth();
  if (!ME) return;
  if (!ME.is_admin) { location.href = '/dashboard.html'; return; }
  if (ME.is_root) { $('nav-nodes').classList.remove('hidden'); $('nav-settings').classList.remove('hidden'); }
  App.initBell($('bell-mount'));
  App.initPalette();
  App.initBanners(ME, $('main'));
  document.querySelectorAll('[data-sec]').forEach(b => b.onclick = () => show(b.dataset.sec));
  App.setPaletteItems([
    ...['overview','servers','users','groups','access','audit','trash'].map(s => ({ label: 'Admin: ' + s, run: () => show(s) })),
    { label: 'Deploy from game template', hint: 'servers', run: () => { show('servers'); setTimeout(openTemplateWizard, 200); } },
  ]);
  show(location.hash.replace('#', '') || 'overview');
})();

async function refreshBase() {
  [USERS, SERVERS, GROUPS] = await Promise.all([
    App.get('/api/admin/users').catch(() => []),
    App.get('/api/admin/servers').catch(() => []),
    App.get('/api/admin/groups').catch(() => []),
  ]);
}

function show(sec) {
  const sections = { overview, servers: serversSec, users: usersSec, groups: groupsSec, access: accessSec, audit: auditSec, trash: trashSec, nodes: nodesSec, settings: settingsSec };
  if (!sections[sec]) sec = 'overview'; // unknown/malformed hash → safe default
  location.hash = sec;
  document.querySelectorAll('[data-sec]').forEach(b => b.classList.toggle('btn-primary', b.dataset.sec === sec));
  sections[sec]();
}

// ================= Overview =================
async function overview() {
  C().innerHTML = `<h1 class="mb">Overview</h1><div class="stat-grid" id="ov-stats"><div class="skeleton"></div><div class="skeleton"></div><div class="skeleton"></div><div class="skeleton"></div></div>
    <div class="glass" style="border-radius:var(--radius-md);padding:24px"><b>Diagnostics</b><div id="ov-diag" class="mt small"></div></div>`;
  await refreshBase();
  const my = await App.get('/api/my-servers').catch(() => []);
  const up = my.filter(s => s.status === 'running').length;
  const diag = await App.get('/api/admin/diagnostics').catch(() => ({}));
  $('ov-stats').innerHTML = `
    <div class="stat-tile glass"><div class="stat-value" style="color:var(--success)">${up}</div><div class="stat-label">Servers running</div></div>
    <div class="stat-tile glass"><div class="stat-value" style="color:var(--danger)">${my.length - up}</div><div class="stat-label">Servers down</div></div>
    <div class="stat-tile glass"><div class="stat-value">${USERS.length}</div><div class="stat-label">Users</div></div>
    <div class="stat-tile glass"><div class="stat-value">${diag.cron_jobs ?? '–'}</div><div class="stat-label">Scheduled jobs</div></div>`;
  $('ov-diag').innerHTML = `Version: <b>${esc(diag.version || '?')}</b> · Audit entries: ${diag.audit_entries ?? '–'} · Nodes: ${esc(JSON.stringify(diag.nodes || {}))}`;
}

// ================= Users =================
async function usersSec() {
  await refreshBase();
  const lockouts = await App.get('/api/admin/lockouts').catch(() => []);
  const locked = lockouts.filter(l => l.locked);
  C().innerHTML = `<div class="topbar"><h1>Users</h1>
      <button class="btn btn-primary" id="u-new">＋ User</button>
      <button class="btn btn-outline" id="u-invite">✉ Invitation link</button>
      <label class="btn btn-outline">📥 CSV import<input type="file" id="u-csv" accept=".csv" hidden></label></div>
    ${locked.length ? `<div class="banner banner-warn">🔒 ${locked.length} locked-out login(s): ${locked.map(l => `<span class="chip" data-unlock="${esc(l.key)}">${esc(l.key)} ✕</span>`).join(' ')}</div>` : ''}
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
      <thead><tr><th>User</th><th>Email</th><th>Role</th><th>2FA</th><th>Last login</th><th></th></tr></thead>
      <tbody>${USERS.map(u => `<tr>
        <td style="cursor:pointer" data-detail="${u.ID}"><b>${esc(u.username)}</b>${u.disabled ? ' <span class="badge badge-danger">disabled</span>' : ''}${u.discord_username ? ` <span class="badge">🎮 ${esc(u.discord_username)}</span>` : ''}</td>
        <td class="small">${esc(u.email || '–')}</td>
        <td>${u.is_root ? '<span class="badge badge-warn">root</span>' : u.is_admin ? '<span class="badge badge-success">admin</span>' : '<span class="badge">user</span>'}</td>
        <td>${u.totp_enabled ? '✅' : '–'}</td>
        <td class="small">${relTime(u.last_login_at)}</td>
        <td><button class="btn btn-outline" style="padding:4px 12px" data-edit="${u.ID}">✎</button>
            <button class="btn btn-danger" style="padding:4px 12px" data-del="${u.ID}" data-n="${esc(u.username)}">🗑</button></td>
      </tr>`).join('')}</tbody></table></div>`;

  C().querySelectorAll('[data-unlock]').forEach(n => n.onclick = async () => { await App.post('/api/admin/unlock', { key: n.dataset.unlock }); toast('Unlocked', 'success'); usersSec(); });
  C().querySelectorAll('[data-detail]').forEach(n => n.onclick = () => openUserDrawer(+n.dataset.detail));
  C().querySelectorAll('[data-edit]').forEach(n => n.onclick = () => userForm(USERS.find(u => u.ID === +n.dataset.edit)));
  C().querySelectorAll('[data-del]').forEach(n => n.onclick = async () => {
    if (!(await App.confirmDialog('Delete user', `Delete <b>${esc(n.dataset.n)}</b> and all their assignments?`))) return;
    await App.del(`/api/admin/users/${n.dataset.del}`).then(() => { toast('Deleted', 'success'); usersSec(); }).catch(e => toast(e.message, 'error'));
  });
  $('u-new').onclick = () => userForm(null);
  $('u-invite').onclick = async () => {
    const isAdmin = confirm('Should the invited user be an admin? OK = admin, Cancel = regular user');
    const inv = await App.post('/api/admin/invitations', { is_admin: isAdmin, expires_in_hours: 72 });
    App.modal('Invitation link (valid 72h)', `<input class="form-input" value="${esc(inv.url)}" readonly onclick="this.select()"><button class="btn btn-primary mt" onclick="navigator.clipboard.writeText('${esc(inv.url)}');App.toast('Copied!','success')">Copy</button>`);
  };
  $('u-csv').onchange = async (e) => {
    const fd = new FormData(); fd.append('csv', e.target.files[0]);
    const res = await App.upload('/api/admin/users/import', fd).catch(err => { toast(err.message, 'error'); });
    if (res) { toast(`Imported ${res.created.length}, failed ${res.failed.length}`, 'success'); usersSec(); }
  };
}

function userForm(u) {
  const dlg = App.modal(u ? 'Edit user' : 'Create user', `
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">
      <input class="form-input" id="uf-name" placeholder="Username" value="${esc(u?.username || '')}">
      <input class="form-input" id="uf-email" placeholder="Email" value="${esc(u?.email || '')}">
      <input class="form-input" id="uf-pass" type="password" placeholder="${u ? 'Password (leave empty to keep)' : 'Password (min 8)'}">
      <div></div>
    </div>
    <div class="perm-grid mt">
      <label><input type="checkbox" id="uf-admin" ${u?.is_admin ? 'checked' : ''}> Admin</label>
      ${u ? `<label><input type="checkbox" id="uf-disabled" ${u.disabled ? 'checked' : ''}> Disabled</label>` : ''}
      <label><input type="checkbox" id="uf-must" ${u?.must_change_password ? 'checked' : ''}> Must change password</label>
    </div>`,
    `<button class="btn btn-primary" id="uf-save">${t('Save')}</button>`);
  dlg.querySelector('#uf-save').onclick = async () => {
    const body = {
      username: dlg.querySelector('#uf-name').value, email: dlg.querySelector('#uf-email').value,
      password: dlg.querySelector('#uf-pass').value, is_admin: dlg.querySelector('#uf-admin').checked,
      must_change_password: dlg.querySelector('#uf-must').checked,
    };
    if (u) body.disabled = dlg.querySelector('#uf-disabled').checked;
    try {
      u ? await App.put(`/api/admin/users/${u.ID}`, body) : await App.post('/api/admin/users', body);
      toast('Saved', 'success'); dlg.close(); dlg.remove(); usersSec();
    } catch (e) { toast(e.message, 'error'); }
  };
}

async function openUserDrawer(id) {
  const d = await App.get(`/api/admin/users/${id}`).catch(e => { toast(e.message, 'error'); });
  if (!d) return;
  const dr = $('drawer');
  dr.innerHTML = `<div class="flex" style="justify-content:space-between"><h2>${esc(d.user.username)}</h2>
    <button class="btn btn-outline btn-icon" onclick="document.getElementById('drawer').classList.remove('open')">✕</button></div>
    <p class="small">Sessions: ${d.active_sessions} · Last login: ${relTime(d.user.last_login_at)}</p>
    <h4 class="mt">Server access</h4>
    ${(d.assignments || []).map(a => { const s = SERVERS.find(x => x.ID === a.server_id); return `<div class="help-row"><span>${esc(s ? s.name : '#' + a.server_id)}</span><span class="small">${['can_start','can_stop','can_restart','can_view_logs','can_send_commands','can_edit_config','can_access_files','can_manage_backups'].filter(k => a[k]).map(k => k.replace('can_', '')).join(', ') || 'none'}${a.expires_at ? ' ⏳' : ''}</span></div>`; }).join('') || '<p class="small">No direct assignments.</p>'}
    <h4 class="mt">Groups</h4>${(d.groups || []).map(g => `<span class="chip">${esc(g.name)}</span>`).join(' ') || '<p class="small">None.</p>'}
    <h4 class="mt">Recent activity</h4>
    ${(d.recent_activity || []).slice(0, 12).map(l => `<div class="help-row"><span class="badge">${esc(l.action)}</span><span class="small">${relTime(l.CreatedAt || l.created_at)}</span></div>`).join('')}`;
  dr.classList.add('open');
}
