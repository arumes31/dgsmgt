// Admin: server management, template/custom wizards, orphans, stacks, trash, nodes, settings.
// ================= Servers =================
async function serversSec() {
  await refreshBase();
  C().innerHTML = `<div class="topbar"><h1>Manage servers</h1>
      <button class="btn btn-primary" id="sv-tpl">🎮 Deploy from template</button>
      <button class="btn btn-outline" id="sv-new">＋ Custom server</button>
      <button class="btn btn-outline" id="sv-orphans">🔍 Orphans</button>
      <button class="btn btn-outline" id="sv-stacks">🧱 Stacks</button></div>
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
      <thead><tr><th>Name</th><th>Image</th><th>Node</th><th>Folder</th><th>Schedule</th><th></th></tr></thead>
      <tbody>${SERVERS.map(s => `<tr>
        <td><b>${esc(s.name)}</b><div class="small" style="font-family:var(--font-mono)">${esc((s.container_id || '').slice(0, 12))}</div></td>
        <td class="small">${esc(s.image)}</td><td class="small">${s.node_id || 'local'}</td>
        <td class="small">${esc(s.folder || '–')}</td>
        <td class="small">${esc(s.cron_schedule || '–')} ${s.backup_schedule ? '· 💾 ' + esc(s.backup_schedule) : ''}</td>
        <td><button class="btn btn-danger" style="padding:4px 12px" data-del="${s.ID}" data-name="${esc(s.name)}">🗑 Trash</button></td>
      </tr>`).join('')}</tbody></table></div>`;
  C().querySelectorAll('[data-del]').forEach(n => n.onclick = async () => {
    if (!(await App.confirmDialog('Move to trash', `The container is removed but volume data and config stay. Restore any time from Trash.`, { typed: n.dataset.name }))) return;
    await App.del(`/api/admin/servers/${n.dataset.del}`).then(() => { toast('Moved to trash', 'success'); serversSec(); }).catch(e => toast(e.message, 'error'));
  });
  $('sv-tpl').onclick = openTemplateWizard;
  $('sv-new').onclick = openCustomWizard;
  $('sv-orphans').onclick = openOrphans;
  $('sv-stacks').onclick = openStacks;
}

async function openTemplateWizard() {
  const templates = await App.get('/api/admin/templates').catch(() => []);
  const cats = [...new Set(templates.map(x => x.category))];
  const dlg = App.modal('Deploy a game server', `
    <div class="flex mb"><select class="form-input" id="tw-cat"><option value="">All categories</option>${cats.map(c => `<option>${esc(c)}</option>`).join('')}</select>
      <input class="form-input grow" id="tw-q" placeholder="Search games…"></div>
    <div class="grid-cards" id="tw-grid" style="grid-template-columns:repeat(auto-fill,minmax(240px,1fr))"></div>
    <div id="tw-form" class="hidden mt">
      <hr style="border-color:var(--glass-border);margin:16px 0">
      <h4 id="tw-sel"></h4><p class="small" id="tw-notes"></p>
      <div class="flex mt"><input class="form-input grow" id="tw-name" placeholder="Server name (e.g. mc-survival)">
      <input class="form-input" id="tw-image" placeholder="image override (optional)" style="flex:1"></div>
      <label class="small flex mt"><input type="checkbox" id="tw-start" checked> Start after deploy</label>
      <p class="small mt" id="tw-ports"></p>
      <button class="btn btn-primary mt" id="tw-go">🚀 Deploy</button></div>`);
  let sel = null;
  const grid = dlg.querySelector('#tw-grid');
  const render = () => {
    const q = dlg.querySelector('#tw-q').value.toLowerCase();
    const cat = dlg.querySelector('#tw-cat').value;
    grid.innerHTML = templates.filter(t => (!q || t.name.toLowerCase().includes(q)) && (!cat || t.category === cat))
      .map(t => `<div class="glass glass-card" style="padding:16px;border-radius:var(--radius-sm);cursor:pointer;${sel && sel.id === t.id ? 'border-color:var(--accent)' : ''}" data-tid="${t.id}">
        <div style="font-size:24px">${App.gameIcon(t.icon, t.image)}</div><b>${esc(t.name)}</b>
        <div class="small">${esc(t.image.split(':')[0])}</div></div>`).join('');
    grid.querySelectorAll('[data-tid]').forEach(n => n.onclick = () => {
      sel = templates.find(x => x.id === n.dataset.tid);
      dlg.querySelector('#tw-form').classList.remove('hidden');
      dlg.querySelector('#tw-sel').textContent = sel.name;
      dlg.querySelector('#tw-notes').textContent = sel.notes || '';
      dlg.querySelector('#tw-ports').textContent = sel.ports.length
        ? (sel.fixed_ports ? `Ports (fixed by game): ${sel.ports.join(', ')}` : `Container ports ${sel.ports.join(', ')} — host ports auto-allocated from the configured range`)
        : 'No ports predefined — add them after deploy.';
      render();
    });
  };
  render();
  dlg.querySelector('#tw-q').oninput = render;
  dlg.querySelector('#tw-cat').onchange = render;
  dlg.querySelector('#tw-go').onclick = async () => {
    if (!sel) return;
    const btn = dlg.querySelector('#tw-go'); btn.disabled = true; btn.textContent = 'Deploying (pulling image)…';
    try {
      const res = await App.post('/api/admin/servers/from-template', {
        template_id: sel.id, name: dlg.querySelector('#tw-name').value,
        image: dlg.querySelector('#tw-image').value, start: dlg.querySelector('#tw-start').checked,
      });
      toast(`Deployed ${res.server.name} — ports: ${res.allocated_ports.join(', ')}`, 'success');
      dlg.close(); dlg.remove(); serversSec();
    } catch (e) { toast(e.message, 'error'); btn.disabled = false; btn.textContent = '🚀 Deploy'; }
  };
}

async function openCustomWizard() {
  const networks = await App.get('/api/admin/networks').catch(() => []);
  const dlg = App.modal('Create custom server', `
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">
      <input class="form-input" id="cw-name" placeholder="Name (min 3 chars)">
      <div class="flex"><input class="form-input grow" id="cw-image" placeholder="Image, e.g. itzg/minecraft-server">
        <button class="btn btn-outline" id="cw-tags" title="List tags">🏷</button></div>
      <select class="form-input" id="cw-net"><option value="">default network</option>${networks.map(n => `<option>${esc(n)}</option>`).join('')}</select>
      <select class="form-input" id="cw-restart"><option>no</option><option>on-failure</option><option selected>unless-stopped</option><option>always</option></select>
    </div>
    <textarea class="form-input mt" id="cw-ports" rows="2" placeholder="Ports: 25565:25565 or 25565:25565/udp (one per line)"></textarea>
    <div class="small" id="cw-conflict"></div>
    <textarea class="form-input mt" id="cw-env" rows="3" placeholder="ENV: KEY=value (one per line)"></textarea>
    <textarea class="form-input mt" id="cw-vol" rows="2" placeholder="Volumes: /host/path:/container/path (one per line)"></textarea>`,
    `<button class="btn btn-primary" id="cw-go">Create</button>`);
  dlg.querySelector('#cw-tags').onclick = async () => {
    const img = dlg.querySelector('#cw-image').value.split(':')[0];
    if (!img) return;
    const tags = await App.get('/api/admin/image-tags?image=' + encodeURIComponent(img)).catch(() => []);
    App.modal('Tags: ' + img, tags.length ? tags.map(t => `<span class="chip" style="margin:4px">${esc(t)}</span>`).join('') : '<p class="small">No tags found (Docker Hub only).</p>');
  };
  dlg.querySelector('#cw-ports').onblur = async () => {
    const ports = dlg.querySelector('#cw-ports').value.split('\n').map(x => x.trim()).filter(Boolean);
    if (!ports.length) return;
    const res = await App.post('/api/admin/port-check', { ports, node_id: 0 }).catch(() => null);
    const el = dlg.querySelector('#cw-conflict');
    const conflicts = res && typeof res.conflicts === 'object' && res.conflicts ? res.conflicts : {};
    if (Object.keys(conflicts).length) {
      el.innerHTML = '⚠️ Port conflicts: ' + Object.entries(conflicts).map(([p, c]) => `<b>${esc(p)}</b> (used by ${esc(c)})`).join(', ');
      el.style.color = 'var(--warning)';
    } else el.textContent = '';
  };
  dlg.querySelector('#cw-go').onclick = async () => {
    const btn = dlg.querySelector('#cw-go'); btn.disabled = true; btn.textContent = 'Creating (pulling image)…';
    try {
      await App.post('/api/admin/servers', {
        name: dlg.querySelector('#cw-name').value, image: dlg.querySelector('#cw-image').value,
        ports: dlg.querySelector('#cw-ports').value.split('\n').map(x => x.trim()).filter(Boolean),
        env: dlg.querySelector('#cw-env').value.split('\n').map(x => x.trim()).filter(Boolean),
        volumes: dlg.querySelector('#cw-vol').value.split('\n').map(x => x.trim()).filter(Boolean),
        network_mode: dlg.querySelector('#cw-net').value, restart_policy: dlg.querySelector('#cw-restart').value,
      });
      toast('Server created', 'success'); dlg.close(); dlg.remove(); serversSec();
    } catch (e) { toast(e.message, 'error'); btn.disabled = false; btn.textContent = 'Create'; }
  };
}

async function openOrphans() {
  const o = await App.get('/api/admin/orphans').catch(() => ({ unmanaged_containers: [], missing_containers: [] }));
  const dlg = App.modal('Orphan detection', `
    <h4>Unmanaged containers on host</h4>
    ${(o.unmanaged_containers || []).map(c => `<div class="help-row"><span>${esc((c.names || []).join(', ') || c.id.slice(0, 12))} <span class="small">(${esc(c.image)}, ${esc(c.state)})</span></span>
      <button class="btn btn-outline" style="padding:4px 12px" data-adopt="${c.id}" data-n="${esc(((c.names || [])[0] || '').replace('/', ''))}">Adopt</button></div>`).join('') || '<p class="small">None 🎉</p>'}
    <h4 class="mt">DB servers with missing containers</h4>
    ${(o.missing_containers || []).map(s => `<div class="help-row"><span>${esc(s.name)}</span><span class="small">container gone — restore via Trash or redeploy</span></div>`).join('') || '<p class="small">None 🎉</p>'}`);
  dlg.querySelectorAll('[data-adopt]').forEach(n => n.onclick = async () => {
    const name = prompt('Panel name for this server:', n.dataset.n);
    if (!name) return;
    await App.post('/api/admin/adopt', { container_id: n.dataset.adopt, name, node_id: 0 })
      .then(() => { toast('Adopted', 'success'); dlg.close(); dlg.remove(); serversSec(); }).catch(e => toast(e.message, 'error'));
  });
}

async function openStacks() {
  const stacks = await App.get('/api/admin/stacks').catch(() => []);
  const dlg = App.modal('Stacks (multi-container groups)', `
    ${stacks.map(s => `<div class="help-row"><span><b>${esc(s.name)}</b> <span class="small">net: ${esc(s.network)}</span></span>
      <span><button class="btn btn-outline" style="padding:4px 10px" data-sa="start" data-id="${s.ID}">▶</button>
      <button class="btn btn-outline" style="padding:4px 10px" data-sa="stop" data-id="${s.ID}">■</button>
      <button class="btn btn-danger" style="padding:4px 10px" data-sd="${s.ID}">🗑</button></span></div>`).join('') || '<p class="small">No stacks. A stack groups servers on a shared network and starts them in depends_on order.</p>'}
    <div class="flex mt"><input class="form-input grow" id="st-name" placeholder="New stack name"><button class="btn btn-primary" id="st-add">＋</button></div>
    <p class="small mt">Assign servers to a stack + set depends_on in the server settings (stack_id).</p>`);
  dlg.querySelector('#st-add').onclick = async () => {
    await App.post('/api/admin/stacks', { name: dlg.querySelector('#st-name').value })
      .then(() => { toast('Stack created', 'success'); dlg.close(); dlg.remove(); openStacks(); }).catch(e => toast(e.message, 'error'));
  };
  dlg.querySelectorAll('[data-sa]').forEach(n => n.onclick = async () => {
    await App.post(`/api/admin/stacks/${n.dataset.id}/${n.dataset.sa}`).then(r => toast('Stack ' + n.dataset.sa + ' done', 'success')).catch(e => toast(e.message, 'error'));
  });
  dlg.querySelectorAll('[data-sd]').forEach(n => n.onclick = async () => {
    await App.del(`/api/admin/stacks/${n.dataset.sd}`)
      .then(() => { toast('Stack deleted', 'success'); dlg.close(); dlg.remove(); openStacks(); })
      .catch(e => toast(e.message, 'error'));
  });
}

// ================= Trash =================
async function trashSec() {
  const trash = await App.get('/api/admin/trash').catch(() => []);
  C().innerHTML = `<h1 class="mb">Trash</h1>
    <p class="small mb">Deleted servers keep their volume data. Restore recreates the container. Permanent delete requires the superadmin (root) account.</p>
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
      <thead><tr><th>Name</th><th>Image</th><th>Deleted</th><th></th></tr></thead>
      <tbody>${(trash || []).map(s => `<tr><td><b>${esc(s.name)}</b></td><td class="small">${esc(s.image)}</td>
        <td class="small">${relTime(s.DeletedAt?.Time || s.deleted_at)}</td>
        <td><button class="btn btn-outline" style="padding:4px 12px" data-rs="${s.ID}">↩ Restore</button>
        ${ME.is_root ? `<button class="btn btn-danger" style="padding:4px 12px" data-purge="${s.ID}" data-n="${esc(s.name)}">Delete forever</button>` : ''}</td></tr>`).join('') || `<tr><td colspan="4" class="small">Trash is empty.</td></tr>`}</tbody></table></div>`;
  C().querySelectorAll('[data-rs]').forEach(n => n.onclick = async () => {
    await App.post(`/api/admin/trash/${n.dataset.rs}/restore`).then(() => { toast('Restored', 'success'); trashSec(); }).catch(e => toast(e.message, 'error'));
  });
  C().querySelectorAll('[data-purge]').forEach(n => n.onclick = async () => {
    if (!(await App.confirmDialog('Permanent delete', 'This removes the server record permanently. Volume data on the host is NOT touched.', { typed: n.dataset.n }))) return;
    await App.del(`/api/admin/root/trash/${n.dataset.purge}`).then(() => { toast('Purged', 'success'); trashSec(); }).catch(e => toast(e.message, 'error'));
  });
}

// ================= Nodes (root) =================
async function nodesSec() {
  const nodes = await App.get('/api/admin/root/nodes').catch(e => { toast(e.message, 'error'); return []; });
  C().innerHTML = `<div class="topbar"><h1>Nodes</h1><button class="btn btn-primary" id="n-add">＋ Node</button></div>
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
      <thead><tr><th>Name</th><th>Address</th><th>Status</th><th></th></tr></thead>
      <tbody>${(nodes || []).map(n => `<tr><td><b>${esc(n.name)}</b></td><td class="small" style="font-family:var(--font-mono)">${esc(n.address)}</td>
        <td>${n.online ? '<span class="badge badge-success">online</span>' : '<span class="badge badge-danger">offline</span>'}</td>
        <td>${n.ID ? `<button class="btn btn-danger" style="padding:4px 12px" data-nd="${n.ID}">🗑</button>` : ''}</td></tr>`).join('')}</tbody></table></div>`;
  C().querySelectorAll('[data-nd]').forEach(n => n.onclick = async () => {
    await App.del(`/api/admin/root/nodes/${n.dataset.nd}`).then(nodesSec).catch(e => toast(e.message, 'error'));
  });
  $('n-add').onclick = () => {
    const dlg = App.modal('Add Docker node', `
      <input class="form-input mb" id="nn-name" placeholder="Name">
      <input class="form-input mb" id="nn-addr" placeholder="tcp://10.0.0.5:2376 or ssh-tunnelled tcp://localhost:23750">
      <p class="small mb">For TLS-protected daemons paste the PEM blocks:</p>
      <textarea class="form-input mb" id="nn-ca" rows="2" placeholder="CA cert (optional)"></textarea>
      <textarea class="form-input mb" id="nn-cert" rows="2" placeholder="Client cert (optional)"></textarea>
      <textarea class="form-input" id="nn-key" rows="2" placeholder="Client key (optional)"></textarea>`,
      `<button class="btn btn-primary" id="nn-go">Connect &amp; add</button>`);
    dlg.querySelector('#nn-go').onclick = async () => {
      await App.post('/api/admin/root/nodes', {
        name: dlg.querySelector('#nn-name').value, address: dlg.querySelector('#nn-addr').value,
        tls_ca: dlg.querySelector('#nn-ca').value, tls_cert: dlg.querySelector('#nn-cert').value, tls_key: dlg.querySelector('#nn-key').value,
      }).then(() => { toast('Node added', 'success'); dlg.close(); dlg.remove(); nodesSec(); }).catch(e => toast(e.message, 'error'));
    };
  };
}

// ================= Settings (root) =================
async function settingsSec() {
  const s = await App.get('/api/admin/root/settings').catch(e => { toast(e.message, 'error'); return {}; });
  let ch = {};
  try { ch = JSON.parse(s.notify_channels || '{}'); } catch {}
  C().innerHTML = `<h1 class="mb">Panel settings</h1>
    <div class="glass mb" style="border-radius:var(--radius-md);padding:24px">
      <label class="form-label">Maintenance banner (empty = off)</label>
      <input class="form-input" id="st-maint" value="${esc(s.maintenance_message || '')}">
      <label class="form-label mt">Community template URL (JSON)</label>
      <input class="form-input" id="st-tpl" value="${esc(s.template_url || '')}" placeholder="https://example.com/templates.json">
    </div>
    <div class="glass mb" style="border-radius:var(--radius-md);padding:24px"><b>Notification channels</b>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px" class="mt">
        <div><label class="form-label">Discord webhook URL</label><input class="form-input" id="ch-discord" value="${esc(ch.discord_webhook || '')}"></div>
        <div><label class="form-label">Generic webhook URL</label><input class="form-input" id="ch-generic" value="${esc(ch.generic_webhook || '')}"></div>
        <div><label class="form-label">Telegram bot token</label><input class="form-input" id="ch-tgt" value="${esc(ch.telegram_bot_token || '')}"></div>
        <div><label class="form-label">Telegram chat ID</label><input class="form-input" id="ch-tgc" value="${esc(ch.telegram_chat_id || '')}"></div>
        <div><label class="form-label">ntfy topic URL</label><input class="form-input" id="ch-ntfy" value="${esc(ch.ntfy_url || '')}" placeholder="https://ntfy.sh/mytopic"></div>
        <div><label class="form-label">Gotify URL + token</label><div class="flex"><input class="form-input" id="ch-gou" value="${esc(ch.gotify_url || '')}"><input class="form-input" id="ch-got" value="${esc(ch.gotify_token || '')}" placeholder="token"></div></div>
      </div>
      <label class="small flex mt"><input type="checkbox" id="ch-email" ${ch.email_enabled ? 'checked' : ''}> Send emails to users (needs SMTP_* env)</label>
    </div>
    <button class="btn btn-primary" id="st-save">${t('Save')}</button>`;
  $('st-save').onclick = async () => {
    const channels = {
      discord_webhook: $('ch-discord').value, generic_webhook: $('ch-generic').value,
      telegram_bot_token: $('ch-tgt').value, telegram_chat_id: $('ch-tgc').value,
      ntfy_url: $('ch-ntfy').value, gotify_url: $('ch-gou').value, gotify_token: $('ch-got').value,
      email_enabled: $('ch-email').checked,
    };
    await App.put('/api/admin/root/settings', {
      maintenance_message: $('st-maint').value, template_url: $('st-tpl').value,
      notify_channels: JSON.stringify(channels),
    }).then(() => toast('Saved', 'success')).catch(e => toast(e.message, 'error'));
  };
}
