// Server detail: settings tab (edit/recreate, redeploy, clone, log alerts).
// ---------- Settings tab (server edit) ----------
async function tabSettings(c) {
  const s = current;
  let cfg = {};
  try { cfg = JSON.parse(s.config_json || '{}'); } catch {}
  const envRows = (cfg.env || []).map(e => { const i = e.indexOf('='); return [e.slice(0, i), e.slice(i + 1)]; });
  const cronSel = App.cronPresets.map(p => `<option value="${p.value}" ${p.value === (s.cron_schedule||'') ? 'selected' : ''}>${p.label}</option>`).join('');

  c.innerHTML = `<form id="s-form" class="glass" style="border-radius:var(--radius-md);padding:28px">
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:16px">
      <div><label class="form-label">${t('Name')}</label><input class="form-input" name="name" value="${esc(s.name)}" required minlength="3"></div>
      <div><label class="form-label">${t('Image')}</label><input class="form-input" name="image" value="${esc(s.image)}" required></div>
      <div><label class="form-label">Folder</label><input class="form-input" name="folder" value="${esc(s.folder || '')}"></div>
      <div><label class="form-label">Restart policy</label>
        <select class="form-input" name="restart_policy">${['no','on-failure','unless-stopped','always'].map(p => `<option ${p===s.restart_policy?'selected':''}>${p}</option>`).join('')}</select></div>
      <div><label class="form-label">CPU limit (cores, 0=∞)</label><input class="form-input" name="cpu_limit" type="number" step="0.5" min="0" value="${s.cpu_limit || 0}"></div>
      <div><label class="form-label">Memory limit (MB, 0=∞)</label><input class="form-input" name="memory_limit_mb" type="number" min="0" value="${s.memory_limit_mb || 0}"></div>
      <div><label class="form-label">Stop timeout (s)</label><input class="form-input" name="stop_timeout" type="number" min="1" value="${s.stop_timeout || 10}"></div>
      <div><label class="form-label">Stop signal</label><input class="form-input" name="stop_signal" placeholder="e.g. SIGINT" value="${esc(s.stop_signal || '')}"></div>
      <div><label class="form-label">Console mode</label>
        <select class="form-input" name="console_mode">${['attach','rcon','none'].map(m => `<option ${m===s.console_mode?'selected':''}>${m}</option>`).join('')}</select></div>
      <div><label class="form-label">RCON host:port / password</label>
        <div class="flex"><input class="form-input" name="rcon_host" placeholder="host" value="${esc(s.rcon_host||'')}" style="flex:2"><input class="form-input" name="rcon_port" type="number" placeholder="port" value="${s.rcon_port||''}" style="flex:1"><input class="form-input" name="rcon_password" type="password" placeholder="(unchanged)" style="flex:2"></div></div>
      <div><label class="form-label">Scheduled action</label>
        <div class="flex"><select class="form-input" id="s-cron">${cronSel}</select>
        <select class="form-input" name="cron_action" style="width:auto">${['restart','stop','start','command'].map(a => `<option ${a===s.cron_action?'selected':''}>${a}</option>`).join('')}</select></div>
        <input class="form-input mt hidden" id="s-cron-custom" name="cron_schedule_custom" placeholder="cron: 0 4 * * *" value="${esc(s.cron_schedule||'')}">
        <input class="form-input mt" name="cron_command" placeholder="command (for action=command)" value="${esc(s.cron_command||'')}"></div>
      <div><label class="form-label">Backup schedule / retention / target</label>
        <div class="flex"><select class="form-input" id="s-bcron">${App.cronPresets.map(p => `<option value="${p.value}" ${p.value === (s.backup_schedule||'') ? 'selected' : ''}>${p.label}</option>`).join('')}</select>
        <input class="form-input" name="backup_retention" type="number" min="0" value="${s.backup_retention}" style="width:80px">
        <select class="form-input" name="backup_target" style="width:auto">${['local','s3','sftp'].map(x => `<option ${x===s.backup_target?'selected':''}>${x}</option>`).join('')}</select></div>
        <input class="form-input mt hidden" id="s-bcron-custom" placeholder="cron" value="${esc(s.backup_schedule||'')}"></div>
    </div>
    <div class="mt"><label class="form-label">Ports (host:container/proto, one per line)</label>
      <textarea class="form-input" name="ports" rows="3" style="font-family:var(--font-mono)">${esc((cfg.ports || []).join('\n'))}</textarea></div>
    <div class="mt"><label class="form-label">Environment variables</label><div id="s-env">
      ${envRows.map(([k, v]) => envRowHTML(k, v)).join('')}</div>
      <button class="btn btn-outline mt" type="button" id="s-env-add">＋ Variable</button></div>
    <div class="mt"><label class="form-label">Volumes (host:container, one per line)</label>
      <textarea class="form-input" name="volumes" rows="3" style="font-family:var(--font-mono)">${esc((cfg.volumes || []).join('\n'))}</textarea></div>
    <div class="mt perm-grid">
      <label><input type="checkbox" name="auto_restart" ${s.auto_restart ? 'checked' : ''}> Auto-restart on crash</label>
      <label><input type="checkbox" name="persist_logs" ${s.persist_logs ? 'checked' : ''}> Persist logs to disk</label>
      <label><input type="checkbox" name="auto_update" ${s.auto_update ? 'checked' : ''}> Notify on image updates</label>
    </div>
    <div class="flex mt flex-wrap">
      <button class="btn btn-primary" type="submit">${t('Save')}</button>
      <button class="btn btn-outline" type="button" id="s-redeploy">⬆ Pull latest &amp; redeploy</button>
      ${ME.is_admin ? `<button class="btn btn-outline" type="button" id="s-clone">⧉ Clone</button>` : ''}
      <span class="small" id="s-cron-next"></span>
    </div>
    <div class="mt" id="s-alerts"></div>
  </form>`;

  function envRowHTML(k = '', v = '') {
    const secret = /pass|secret|token|key/i.test(k);
    return `<div class="kv-row"><input class="form-input" placeholder="KEY" value="${esc(k)}">
      <input class="form-input" placeholder="value" type="${secret ? 'password' : 'text'}" value="${esc(v)}">
      <button class="btn btn-danger btn-icon" type="button" onclick="this.parentNode.remove()">✕</button></div>`;
  }
  $('s-env-add').onclick = () => $('s-env').insertAdjacentHTML('beforeend', envRowHTML());

  const cronCustom = (selId, inputId) => {
    const sel = $(selId), inp = $(inputId);
    const upd = () => inp.classList.toggle('hidden', sel.value !== 'custom');
    if (![...sel.options].some(o => o.value === sel.value && sel.value !== 'custom' && o.selected)) { /* noop */ }
    if (sel.value !== 'custom' && inp.value && ![...sel.options].some(o => o.value === inp.value)) { sel.value = 'custom'; }
    upd(); sel.onchange = upd;
  };
  cronCustom('s-cron', 's-cron-custom');
  cronCustom('s-bcron', 's-bcron-custom');

  $('s-form').onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    const env = [...$('s-env').querySelectorAll('.kv-row')].map(r => {
      const [k, v] = r.querySelectorAll('input');
      return k.value ? `${k.value}=${v.value}` : null;
    }).filter(Boolean);
    const cron = $('s-cron').value === 'custom' ? $('s-cron-custom').value : $('s-cron').value;
    const bcron = $('s-bcron').value === 'custom' ? $('s-bcron-custom').value : $('s-bcron').value;
    const body = {
      name: f.get('name'), image: f.get('image'), folder: f.get('folder'), icon: s.icon,
      node_id: s.node_id, depends_on: s.depends_on, stack_id: s.stack_id,
      ports: f.get('ports').split('\n').map(x => x.trim()).filter(Boolean),
      env, volumes: f.get('volumes').split('\n').map(x => x.trim()).filter(Boolean),
      restart_policy: f.get('restart_policy'), cpu_limit: +f.get('cpu_limit'), memory_limit_mb: +f.get('memory_limit_mb'),
      stop_timeout: +f.get('stop_timeout'), stop_signal: f.get('stop_signal'),
      console_mode: f.get('console_mode'), rcon_host: f.get('rcon_host'), rcon_port: +f.get('rcon_port') || 0,
      rcon_password: f.get('rcon_password'),
      cron_schedule: cron, cron_action: f.get('cron_action'), cron_command: f.get('cron_command'),
      backup_schedule: bcron, backup_retention: +f.get('backup_retention'), backup_target: f.get('backup_target'),
      auto_restart: f.get('auto_restart') === 'on', persist_logs: f.get('persist_logs') === 'on', auto_update: f.get('auto_update') === 'on',
      health_check_type: s.health_check_type, health_check_port: s.health_check_port,
    };
    try {
      const updated = await App.put(`/api/admin/servers/${s.ID}`, body);
      Object.assign(current, updated);
      toast('Saved — container recreated if needed', 'success');
      refreshDetailHead();
    } catch (err) { toast(err.message, 'error'); }
  };

  $('s-redeploy').onclick = () => {
    const dlg = App.modal('Redeploy: pulling latest image', `<div class="console" style="height:300px" id="rd-log"></div>`);
    const log = dlg.querySelector('#rd-log');
    const sock = App.ws(`/api/admin/servers/${s.ID}/redeploy`);
    sock.onmessage = (e) => {
      try {
        const j = JSON.parse(e.data);
        if (j.done) { log.innerHTML += '<div class="hl-join">✔ Redeployed successfully</div>'; toast('Redeployed', 'success'); refreshDetailHead(); return; }
        if (j.error) { log.innerHTML += `<div class="hl-error">${esc(j.error)}</div>`; return; }
        log.innerHTML += `<div>${esc(j.status || e.data)} ${esc(j.progress || '')}</div>`;
      } catch { log.innerHTML += `<div>${esc(e.data)}</div>`; }
      log.scrollTop = log.scrollHeight;
    };
  };

  const clone = $('s-clone');
  if (clone) clone.onclick = async () => {
    const name = prompt('Name for the clone:', s.name + '-copy');
    if (!name) return;
    await App.post(`/api/admin/servers/${s.ID}/clone`, { name, ports: [] })
      .then(() => { toast('Cloned (created stopped, adjust ports!)', 'success'); loadServers(); })
      .catch(e => toast(e.message, 'error'));
  };

  // Log alerts
  const alerts = await App.get(`/api/log-alerts/${current.container_id}`).catch(() => []);
  $('s-alerts').innerHTML = `<label class="form-label">Log alerts (regex → notification)</label>
    <div id="al-list">${(alerts || []).map(a => `<div class="flex mb"><code class="chip grow">${esc(a.pattern)}</code>
      <button class="btn btn-danger btn-icon" data-al="${a.ID}">✕</button></div>`).join('')}</div>
    <div class="flex"><input class="form-input grow" id="al-pat" placeholder="e.g. Exception|OutOfMemory"><button class="btn btn-outline" type="button" id="al-add">＋</button></div>`;
  $('s-alerts').querySelectorAll('[data-al]').forEach(n => n.onclick = async () => { await App.del(`/api/log-alerts/${current.container_id}/${n.dataset.al}`); n.parentNode.remove(); });
  $('al-add').onclick = async () => {
    const pattern = $('al-pat').value.trim();
    if (!pattern) return;
    await App.post(`/api/log-alerts/${current.container_id}`, { pattern }).then(() => { toast('Alert added', 'success'); switchTab('settings'); }).catch(e => toast(e.message, 'error'));
  };
}
