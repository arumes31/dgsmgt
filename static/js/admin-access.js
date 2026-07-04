// Admin: groups, permission dialogs, assignments, matrix, audit log.
// ================= Groups =================
async function groupsSec() {
  await refreshBase();
  C().innerHTML = `<div class="topbar"><h1>Groups</h1>
    <div class="flex"><input class="form-input" id="g-name" placeholder="New group name"><button class="btn btn-primary" id="g-add">＋</button></div></div>
    <div id="g-list">${GROUPS.map(g => `<div class="glass mb" style="border-radius:var(--radius-md);padding:20px">
      <div class="flex" style="justify-content:space-between"><b>${esc(g.name)}</b>
        <span><button class="btn btn-outline" style="padding:4px 12px" data-gm="${g.ID}">👥 Members (${(g.members || []).length})</button>
        <button class="btn btn-outline" style="padding:4px 12px" data-gs="${g.ID}">🗄 Grant server</button>
        <button class="btn btn-danger" style="padding:4px 12px" data-gd="${g.ID}">🗑</button></span></div>
      <div class="mt small">${(g.servers || []).map(gs => { const s = SERVERS.find(x => x.ID === gs.server_id); return `<span class="chip" title="remove" data-gsd="${g.ID}:${gs.server_id}">${esc(s ? s.name : '#' + gs.server_id)} ✕</span>`; }).join(' ') || 'No server grants.'}</div>
    </div>`).join('') || '<div class="empty-state glass" style="border-radius:var(--radius-md)"><h3>No groups</h3><p>Groups let you assign many users to many servers in one step.</p></div>'}</div>`;
  $('g-add').onclick = async () => { await App.post('/api/admin/groups', { name: $('g-name').value }).then(groupsSec).catch(e => toast(e.message, 'error')); };
  C().querySelectorAll('[data-gd]').forEach(n => n.onclick = async () => {
    if (await App.confirmDialog('Delete group', 'Members lose access granted via this group.')) { await App.del(`/api/admin/groups/${n.dataset.gd}`); groupsSec(); }
  });
  C().querySelectorAll('[data-gsd]').forEach(n => n.onclick = async () => {
    const [g, s] = n.dataset.gsd.split(':');
    await App.del(`/api/admin/groups/${g}/servers/${s}`); groupsSec();
  });
  C().querySelectorAll('[data-gm]').forEach(n => n.onclick = () => {
    const g = GROUPS.find(x => x.ID === +n.dataset.gm);
    const dlg = App.modal('Members: ' + g.name, `<div class="perm-grid">${USERS.map(u =>
      `<label><input type="checkbox" value="${u.ID}" ${(g.members || []).includes(u.ID) ? 'checked' : ''}> ${esc(u.username)}</label>`).join('')}</div>`,
      `<button class="btn btn-primary" id="gm-save">${t('Save')}</button>`);
    dlg.querySelector('#gm-save').onclick = async () => {
      const ids = [...dlg.querySelectorAll('input:checked')].map(i => +i.value);
      await App.put(`/api/admin/groups/${g.ID}/members`, { user_ids: ids });
      toast('Saved', 'success'); dlg.close(); dlg.remove(); groupsSec();
    };
  });
  C().querySelectorAll('[data-gs]').forEach(n => n.onclick = () => {
    const g = GROUPS.find(x => x.ID === +n.dataset.gs);
    permsDialog(`Grant server to group "${g.name}"`, async (serverId, preset, perms) => {
      await App.post(`/api/admin/groups/${g.ID}/servers`, { server_id: serverId, preset, ...perms });
      groupsSec();
    });
  });
}

const PERM_KEYS = [['can_start','Start'],['can_stop','Stop'],['can_restart','Restart'],['can_view_logs','View logs'],['can_send_commands','Send commands'],['can_edit_config','Edit config'],['can_access_files','File access'],['can_manage_backups','Backups']];
function permsDialog(title, onSave, withUser = false, withExpiry = false) {
  const dlg = App.modal(title, `
    ${withUser ? `<select class="form-input mb" id="pd-user">${USERS.map(u => `<option value="${u.ID}">${esc(u.username)}</option>`).join('')}</select>` : ''}
    <select class="form-input mb" id="pd-server">${SERVERS.map(s => `<option value="${s.ID}">${esc(s.name)}</option>`).join('')}</select>
    <label class="form-label">Preset</label>
    <select class="form-input mb" id="pd-preset"><option value="">Custom</option><option value="viewer">Viewer (logs only)</option><option value="operator">Operator (control + commands)</option><option value="owner">Owner (everything)</option></select>
    <div class="perm-grid" id="pd-perms">${PERM_KEYS.map(([k, l]) => `<label><input type="checkbox" name="${k}"> ${l}</label>`).join('')}</div>
    ${withExpiry ? `<label class="form-label mt">Access expires (optional)</label><input class="form-input" type="datetime-local" id="pd-exp">` : ''}`,
    `<button class="btn btn-primary" id="pd-save">${t('Save')}</button>`);
  dlg.querySelector('#pd-preset').onchange = (e) => {
    const presets = { viewer: ['can_view_logs'], operator: ['can_start','can_stop','can_restart','can_view_logs','can_send_commands'], owner: PERM_KEYS.map(([k]) => k) };
    const sel = presets[e.target.value] || [];
    dlg.querySelectorAll('#pd-perms input').forEach(i => i.checked = sel.includes(i.name));
  };
  dlg.querySelector('#pd-save').onclick = async () => {
    const perms = {}; dlg.querySelectorAll('#pd-perms input').forEach(i => perms[i.name] = i.checked);
    const exp = withExpiry && dlg.querySelector('#pd-exp').value ? new Date(dlg.querySelector('#pd-exp').value).toISOString() : null;
    try {
      await onSave(+dlg.querySelector('#pd-server').value, dlg.querySelector('#pd-preset').value, perms,
        withUser ? +dlg.querySelector('#pd-user').value : null, exp);
      toast('Saved', 'success'); dlg.close(); dlg.remove();
    } catch (e) { toast(e.message, 'error'); }
  };
}

// ================= Access (assignments + matrix) =================
async function accessSec() {
  await refreshBase();
  const assignments = await App.get('/api/admin/assignments').catch(() => []);
  C().innerHTML = `<div class="topbar"><h1>Access</h1>
      <button class="btn btn-primary" id="ac-new">＋ Assign</button>
      <button class="btn btn-outline" id="ac-bulk">👥 Bulk assign</button>
      <button class="btn btn-outline" id="ac-matrix">🔎 Permission matrix</button></div>
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
      <thead><tr><th>User</th><th>Server</th><th>Permissions</th><th>Expires</th><th></th></tr></thead>
      <tbody>${assignments.map(a => {
        const u = USERS.find(x => x.ID === a.user_id), s = SERVERS.find(x => x.ID === a.server_id);
        return `<tr><td>${esc(u ? u.username : '#' + a.user_id)}</td><td>${esc(s ? s.name : '#' + a.server_id)}</td>
          <td class="small">${PERM_KEYS.filter(([k]) => a[k]).map(([, l]) => l).join(', ') || '–'}</td>
          <td class="small">${a.expires_at ? relTime(a.expires_at) : '–'}</td>
          <td><button class="btn btn-danger" style="padding:4px 12px" data-ad="${a.user_id}:${a.server_id}">✕</button></td></tr>`;
      }).join('')}</tbody></table></div>`;
  C().querySelectorAll('[data-ad]').forEach(n => n.onclick = async () => {
    const [u, s] = n.dataset.ad.split(':');
    await App.del(`/api/admin/assignments/${u}/${s}`); accessSec();
  });
  $('ac-new').onclick = () => permsDialog('Assign server to user', async (serverId, preset, perms, userId, exp) => {
    await App.post('/api/admin/assign', { user_id: userId, server_id: serverId, preset, expires_at: exp, ...perms }); accessSec();
  }, true, true);
  $('ac-bulk').onclick = () => {
    const dlg = App.modal('Bulk assign', `
      <label class="form-label">Users</label><div class="perm-grid mb">${USERS.map(u => `<label><input type="checkbox" data-u="${u.ID}"> ${esc(u.username)}</label>`).join('')}</div>
      <label class="form-label">Servers</label><div class="perm-grid mb">${SERVERS.map(s => `<label><input type="checkbox" data-s="${s.ID}"> ${esc(s.name)}</label>`).join('')}</div>
      <select class="form-input" id="ba-preset"><option value="viewer">Viewer</option><option value="operator" selected>Operator</option><option value="owner">Owner</option></select>`,
      `<button class="btn btn-primary" id="ba-go">Assign</button>`);
    dlg.querySelector('#ba-go').onclick = async () => {
      const user_ids = [...dlg.querySelectorAll('[data-u]:checked')].map(i => +i.dataset.u);
      const server_ids = [...dlg.querySelectorAll('[data-s]:checked')].map(i => +i.dataset.s);
      const res = await App.post('/api/admin/bulk-assign', { user_ids, server_ids, preset: dlg.querySelector('#ba-preset').value }).catch(e => toast(e.message, 'error'));
      if (res) { toast(`Created ${res.created} assignments`, 'success'); dlg.close(); dlg.remove(); accessSec(); }
    };
  };
  $('ac-matrix').onclick = async () => {
    const m = await App.get('/api/admin/permission-matrix').catch(e => { toast(e.message, 'error'); return null; });
    if (!m) return;
    const byUser = {};
    m.forEach(c => { (byUser[c.user_id] = byUser[c.user_id] || []).push(c); });
    App.modal('Permission matrix (effective, incl. groups)', `<div class="table-wrap"><table class="data">
      <thead><tr><th>User</th>${SERVERS.map(s => `<th>${esc(s.name)}</th>`).join('')}</tr></thead>
      <tbody>${USERS.map(u => `<tr><td><b>${esc(u.username)}</b></td>${SERVERS.map(s => {
        const cell = (byUser[u.ID] || []).find(c => c.server_id === s.ID);
        if (!cell) return '<td class="small">–</td>';
        const n = PERM_KEYS.filter(([k]) => cell.perms[k]).length;
        return `<td title="${PERM_KEYS.filter(([k]) => cell.perms[k]).map(([, l]) => l).join(', ')}">${n === 8 ? '🟢 all' : '🟡 ' + n}</td>`;
      }).join('')}</tr>`).join('')}</tbody></table></div>`);
  };
}

// ================= Audit =================
async function auditSec(page = 1) {
  C().innerHTML = `<div class="topbar"><h1>Audit log</h1>
      <input class="form-input" id="au-user" placeholder="user" style="width:130px">
      <input class="form-input" id="au-action" placeholder="action" style="width:130px">
      <select class="form-input" id="au-ok" style="width:auto"><option value="">all</option><option value="true">success</option><option value="false">failed</option></select>
      <button class="btn btn-outline" id="au-go">Filter</button>
      <button class="btn btn-outline" id="au-csv">⬇ CSV</button></div>
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
      <thead><tr><th>Time</th><th>User</th><th>Action</th><th>Server</th><th>Details</th><th>IP</th><th>OK</th></tr></thead>
      <tbody id="au-body"></tbody></table></div><div class="pager" id="au-pager"></div>`;
  const load = async (p) => {
    const q = new URLSearchParams({ page: p, per_page: 50 });
    if ($('au-user').value) q.set('user', $('au-user').value);
    if ($('au-action').value) q.set('action', $('au-action').value);
    if ($('au-ok').value) q.set('success', $('au-ok').value);
    const { data, meta } = await App.getFull('/api/admin/audit-logs?' + q);
    $('au-body').innerHTML = (data || []).map(l => `<tr>
      <td class="small">${relTime(l.CreatedAt || l.created_at)}</td><td>${esc(l.username)}</td>
      <td><span class="badge">${esc(l.action)}</span></td><td class="small">${esc(l.server_name || '–')}</td>
      <td class="small" style="max-width:340px">${esc(l.details)}${l.diff_json ? ` <span class="chip" data-diff='${esc(l.diff_json)}'>diff</span>` : ''}</td>
      <td class="small">${esc(l.ip)}</td><td>${l.success ? '✅' : '❌'}</td></tr>`).join('');
    $('au-body').querySelectorAll('[data-diff]').forEach(n => n.onclick = () =>
      App.modal('Change diff', `<pre class="console" style="height:auto;max-height:50vh">${esc(JSON.stringify(JSON.parse(n.dataset.diff), null, 2))}</pre>`));
    const pages = Math.ceil((meta?.total || 0) / 50) || 1;
    $('au-pager').innerHTML = `<button class="btn btn-outline" ${p <= 1 ? 'disabled' : ''} id="pg-prev">←</button> Page ${p} / ${pages} (${meta?.total || 0}) <button class="btn btn-outline" ${p >= pages ? 'disabled' : ''} id="pg-next">→</button>`;
    $('pg-prev').onclick = () => load(p - 1);
    $('pg-next').onclick = () => load(p + 1);
  };
  load(page);
  $('au-go').onclick = () => load(1);
  $('au-csv').onclick = () => App.download('/api/admin/audit-logs?format=csv', 'audit-log.csv');
}
