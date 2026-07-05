// Server detail tabs: console, metrics, files, backups, activity.
// ---------- Console tab ----------
function tabConsole(c) {
  c.innerHTML = `
    <div class="flex flex-wrap mb">
      <select class="form-input" id="con-tail" style="width:auto"><option>100</option><option>500</option><option>2000</option><option value="all">all</option></select>
      <label class="small flex"><input type="checkbox" id="con-ts"> timestamps</label>
      <label class="small flex"><input type="checkbox" id="con-wrap" checked> wrap</label>
      <input class="form-input grow" id="con-filter" placeholder="Filter lines (regex)…" style="max-width:280px">
      <button class="btn btn-outline" id="con-download">⬇ ${t('Download')}</button>
      <button class="btn btn-outline btn-icon" id="con-full" title="Fullscreen">⛶</button>
    </div>
    <div class="console" id="con"></div>
    <div class="console-input">
      <input class="form-input grow" id="con-cmd" placeholder="${current.can_send_commands ? 'Send command… (↑ history)' : 'No command permission'}" ${current.can_send_commands ? '' : 'disabled'}>
      <button class="btn btn-primary" id="con-send" ${current.can_send_commands ? '' : 'disabled'}>➤</button>
      <button class="btn btn-outline" id="con-snip">☰</button>
    </div>`;

  const con = $('con');
  let autoScroll = true, missed = 0, history = JSON.parse(localStorage.getItem('cmdh-' + current.ID) || '[]'), hIdx = -1;
  let filterRe = null;
  con.addEventListener('scroll', () => {
    autoScroll = con.scrollTop + con.clientHeight > con.scrollHeight - 40;
    if (autoScroll) { missed = 0; removePill(); }
  });
  const removePill = () => con.querySelector('.scroll-pill')?.remove();
  const showPill = () => {
    if (con.querySelector('.scroll-pill')) { con.querySelector('.scroll-pill').textContent = `↓ ${missed} new lines`; return; }
    const p = document.createElement('span'); p.className = 'scroll-pill'; p.textContent = `↓ ${missed} new lines`;
    p.onclick = () => { con.scrollTop = con.scrollHeight; };
    con.appendChild(p);
  };

  const ansi = (s) => esc(s)
    .replace(/\x1b\[[0-9;]*m/g, (m) => {
      const codes = m.slice(2, -1).split(';');
      if (codes.includes('0')) return '</span>';
      const colors = { '30':'#64748b','31':'#f87171','32':'#4ade80','33':'#fbbf24','34':'#60a5fa','35':'#c084fc','36':'#22d3ee','37':'#e2e8f0','90':'#94a3b8','91':'#fca5a5','92':'#86efac','93':'#fde047','94':'#93c5fd','95':'#d8b4fe','96':'#67e8f9','97':'#fff' };
      for (const code of codes) if (colors[code]) return `<span style="color:${colors[code]}">`;
      return '';
    })
    .replace(/\b(ERROR|SEVERE|FATAL|Exception|CRITICAL)\b/g, '<span class="hl-error">$1</span>')
    .replace(/\b(WARN|WARNING)\b/g, '<span class="hl-warn">$1</span>')
    .replace(/\b(\w+ (joined|left) the game)\b/g, '<span class="hl-join">$1</span>');

  const append = (data, stream) => {
    for (const line of data.split('\n')) {
      if (!line.trim()) continue;
      if (filterRe && !filterRe.test(line)) continue;
      const div = document.createElement('div');
      if (stream === 'stderr') div.className = 'line-err';
      div.innerHTML = ansi(line);
      con.appendChild(div);
    }
    while (con.childElementCount > 4000) con.removeChild(con.firstChild);
    if (autoScroll) con.scrollTop = con.scrollHeight;
    else { missed++; showPill(); }
  };

  let sock;
  const connect = () => {
    con.innerHTML = '';
    sock = App.ws(`/api/console/${current.container_id}?tail=${$('con-tail').value}&timestamps=${$('con-ts').checked}`);
    sockets.push(sock);
    sock.onmessage = (e) => { try { const f = JSON.parse(e.data); append(f.data, f.stream); } catch { append(e.data, 'stdout'); } };
    sock.onclose = () => append('\n[connection closed]', 'stderr');
  };
  connect();

  $('con-tail').onchange = () => { closeSockets(); connect(); };
  $('con-ts').onchange = () => { closeSockets(); connect(); };
  $('con-wrap').onchange = (e) => con.classList.toggle('nowrap', !e.target.checked);
  $('con-filter').oninput = (e) => { try { filterRe = e.target.value ? new RegExp(e.target.value, 'i') : null; e.target.style.borderColor=''; } catch { e.target.style.borderColor = 'var(--danger)'; } };
  $('con-download').onclick = () => App.download(`/api/logs/${current.container_id}/download`, `${current.name}.log`);
  $('con-full').onclick = () => con.classList.toggle('fullscreen');
  // Register the Escape-to-exit-fullscreen handler once for the page. A
  // per-tabConsole listener accumulates on every tab switch (and closes over a
  // stale `con`); this single handler acts on whichever console is fullscreen.
  if (!window.__conFsKeyHandler) {
    window.__conFsKeyHandler = true;
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') document.querySelector('.console.fullscreen')?.classList.remove('fullscreen');
    });
  }

  const send = () => {
    const cmd = $('con-cmd').value.trim();
    if (!cmd || sock.readyState !== 1) return;
    sock.send(cmd);
    append('> ' + cmd, 'stdout');
    history.unshift(cmd); history = history.slice(0, 50);
    localStorage.setItem('cmdh-' + current.ID, JSON.stringify(history));
    hIdx = -1; $('con-cmd').value = '';
  };
  $('con-send').onclick = send;
  $('con-cmd').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') send();
    else if (e.key === 'ArrowUp') { hIdx = Math.min(hIdx + 1, history.length - 1); $('con-cmd').value = history[hIdx] || ''; e.preventDefault(); }
    else if (e.key === 'ArrowDown') { hIdx = Math.max(hIdx - 1, -1); $('con-cmd').value = history[hIdx] || ''; e.preventDefault(); }
  });

  $('con-snip').onclick = async () => {
    const snips = await App.get(`/api/snippets/${current.container_id}`) || [];
    const dlg = App.modal('Command snippets', `
      <div id="snip-list">${snips.map(s => `<div class="help-row"><span class="chip" data-run="${esc(s.command)}">${esc(s.name)}</span>
        <span><code class="small">${esc(s.command)}</code> <button class="btn btn-danger" style="padding:2px 10px" data-del="${s.ID}">✕</button></span></div>`).join('') || '<p class="small">No snippets yet.</p>'}</div>
      ${current.can_send_commands ? `<div class="flex mt"><input class="form-input" id="sn-name" placeholder="Name"><input class="form-input grow" id="sn-cmd" placeholder="Command"><button class="btn btn-primary" id="sn-add">＋</button></div>` : ''}`);
    dlg.querySelectorAll('[data-run]').forEach(n => n.onclick = () => { $('con-cmd').value = n.dataset.run; dlg.close(); dlg.remove(); });
    dlg.querySelectorAll('[data-del]').forEach(n => n.onclick = async () => { await App.del(`/api/snippets/${current.container_id}/${n.dataset.del}`); n.closest('.help-row').remove(); });
    const add = dlg.querySelector('#sn-add');
    if (add) add.onclick = async () => {
      await App.post(`/api/snippets/${current.container_id}`, { name: dlg.querySelector('#sn-name').value, command: dlg.querySelector('#sn-cmd').value });
      toast('Saved', 'success'); dlg.close(); dlg.remove();
    };
  };
}

// ---------- Metrics tab ----------
function tabMetrics(c) {
  c.innerHTML = `
    <div class="stat-grid">
      <div class="stat-tile glass"><div class="stat-value" id="m-cpu">–</div><div class="stat-label">CPU %</div><canvas class="mini-chart" id="ch-cpu"></canvas></div>
      <div class="stat-tile glass"><div class="stat-value" id="m-mem">–</div><div class="stat-label">Memory</div><canvas class="mini-chart" id="ch-mem"></canvas></div>
      <div class="stat-tile glass"><div class="stat-value" id="m-net">–</div><div class="stat-label">Network rx/tx</div></div>
      <div class="stat-tile glass"><div class="stat-value" id="m-disk">–</div><div class="stat-label">Disk</div></div>
    </div>
    <div class="glass" style="border-radius:var(--radius-md);padding:24px">
      <div class="flex mb"><b>History</b>
        <select class="form-input" id="m-range" style="width:auto"><option value="1">1h</option><option value="24" selected>24h</option><option value="168">7d</option></select>
      </div>
      <canvas class="big-chart" id="ch-hist"></canvas>
    </div>`;

  const cpuData = [], memData = [];
  const drawMini = (cv, data, color) => {
    const ctx = cv.getContext('2d');
    cv.width = cv.clientWidth; cv.height = cv.clientHeight;
    ctx.clearRect(0, 0, cv.width, cv.height);
    if (data.length < 2) return;
    const max = Math.max(...data, 1);
    ctx.beginPath(); ctx.strokeStyle = color; ctx.lineWidth = 2;
    data.forEach((v, i) => { const x = i / (data.length - 1) * cv.width, y = cv.height - (v / max) * (cv.height - 4) - 2; i ? ctx.lineTo(x, y) : ctx.moveTo(x, y); });
    ctx.stroke();
  };

  const sock = App.ws(`/api/metrics/${current.container_id}`);
  sockets.push(sock);
  sock.onmessage = (e) => {
    try {
      const m = JSON.parse(e.data);
      if (m.error) return;
      $('m-cpu').textContent = m.cpu_percent.toFixed(1) + '%';
      $('m-mem').textContent = fmtBytes(m.mem_used) + (m.mem_limit ? ' / ' + fmtBytes(m.mem_limit) : '');
      $('m-net').textContent = `${fmtBytes(m.net_rx)} / ${fmtBytes(m.net_tx)}`;
      cpuData.push(m.cpu_percent); memData.push(m.mem_used);
      if (cpuData.length > 60) { cpuData.shift(); memData.shift(); }
      drawMini($('ch-cpu'), cpuData, '#818cf8');
      drawMini($('ch-mem'), memData, '#a855f7');
    } catch {}
  };

  App.get(`/api/disk/${current.container_id}`).then(d => {
    const vols = Object.values(d.volumes || {}).reduce((a, b) => a + b, 0);
    $('m-disk').textContent = fmtBytes(vols || d.size_rw);
  }).catch(() => $('m-disk').textContent = '–');

  const drawHist = async () => {
    const hours = $('m-range').value;
    const samples = await App.get(`/api/metrics/${current.container_id}/history?hours=${hours}`) || [];
    const cv = $('ch-hist'); const ctx = cv.getContext('2d');
    cv.width = cv.clientWidth; cv.height = cv.clientHeight;
    ctx.clearRect(0, 0, cv.width, cv.height);
    if (samples.length < 2) { ctx.fillStyle = '#64748b'; ctx.font = '13px sans-serif'; ctx.fillText('Not enough data yet — samples are collected every 30s.', 16, 30); return; }
    const cpu = samples.map(s => s.cpu_percent);
    const maxC = Math.max(...cpu, 10);
    ctx.strokeStyle = 'rgba(129,140,248,.9)'; ctx.lineWidth = 1.5; ctx.beginPath();
    samples.forEach((s, i) => { const x = i / (samples.length - 1) * cv.width; const y = cv.height - (s.cpu_percent / maxC) * (cv.height - 20) - 10; i ? ctx.lineTo(x, y) : ctx.moveTo(x, y); });
    ctx.stroke();
    // downtime shading
    ctx.fillStyle = 'rgba(239,68,68,.15)';
    samples.forEach((s, i) => { if (!s.running) { const x = i / (samples.length - 1) * cv.width; ctx.fillRect(x - 1, 0, 3, cv.height); } });
  };
  drawHist();
  $('m-range').onchange = drawHist;
}

// ---------- Files tab ----------
function tabFiles(c) {
  let cwd = '/';
  c.innerHTML = `
    <div class="flex flex-wrap mb">
      <div class="breadcrumbs grow" id="f-crumbs" style="margin:0"></div>
      <button class="btn btn-outline" id="f-mkdir">＋ Folder</button>
      <label class="btn btn-outline">⬆ ${t('Upload')}<input type="file" id="f-upload" multiple hidden></label>
      <label class="btn btn-outline">📦 Extract tar<input type="file" id="f-extract" accept=".tar" hidden></label>
    </div>
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data" id="f-table">
      <thead><tr><th>Name</th><th>Size</th><th>Mode</th><th style="width:220px"></th></tr></thead><tbody></tbody></table></div>`;

  const crumbs = () => {
    const parts = cwd.split('/').filter(Boolean);
    $('f-crumbs').innerHTML = `<span data-p="/">/</span>` + parts.map((p, i) =>
      `<span data-p="/${parts.slice(0, i + 1).join('/')}">${esc(p)}/</span>`).join('');
    $('f-crumbs').querySelectorAll('[data-p]').forEach(n => n.onclick = () => { cwd = n.dataset.p; load(); });
  };

  const load = async () => {
    crumbs();
    const tb = c.querySelector('tbody');
    tb.innerHTML = `<tr><td colspan="4" class="small">${t('Loading...')}</td></tr>`;
    try {
      const res = await App.get(`/api/files/${current.container_id}/list?path=${encodeURIComponent(cwd)}`);
      const entries = (res.entries || []).sort((a, b) => (b.is_dir - a.is_dir) || a.name.localeCompare(b.name));
      tb.innerHTML = entries.map(e => `<tr>
        <td style="cursor:pointer" data-nav="${esc(e.name)}" data-dir="${e.is_dir}">${e.is_dir ? '📁' : '📄'} ${esc(e.name)}</td>
        <td class="small">${e.is_dir ? '–' : fmtBytes(e.size)}</td>
        <td class="small" style="font-family:var(--font-mono)">${esc(e.mode)}</td>
        <td>
          ${!e.is_dir && e.size < 5242880 ? `<button class="btn btn-outline" style="padding:4px 10px" data-edit="${esc(e.name)}">✎</button>` : ''}
          <button class="btn btn-outline" style="padding:4px 10px" data-dl="${esc(e.name)}">⬇</button>
          <button class="btn btn-outline" style="padding:4px 10px" data-mv="${esc(e.name)}">✂</button>
          <button class="btn btn-danger" style="padding:4px 10px" data-rm="${esc(e.name)}">🗑</button>
        </td></tr>`).join('') || `<tr><td colspan="4" class="small">Empty directory</td></tr>`;

      const join = (n) => (cwd === '/' ? '' : cwd) + '/' + n;
      tb.querySelectorAll('[data-nav]').forEach(n => n.onclick = () => { if (n.dataset.dir === 'true') { cwd = join(n.dataset.nav); load(); } });
      tb.querySelectorAll('[data-dl]').forEach(n => n.onclick = () => App.download(`/api/files/${current.container_id}/download?path=${encodeURIComponent(join(n.dataset.dl))}`, n.dataset.dl + '.tar'));
      tb.querySelectorAll('[data-rm]').forEach(n => n.onclick = async () => {
        if (!(await App.confirmDialog('Delete', `Delete <b>${esc(n.dataset.rm)}</b>? This cannot be undone.`))) return;
        await App.post(`/api/files/${current.container_id}/delete`, { path: join(n.dataset.rm) }).then(() => { toast('Deleted', 'success'); load(); }).catch(e => toast(e.message, 'error'));
      });
      tb.querySelectorAll('[data-mv]').forEach(n => n.onclick = async () => {
        const to = prompt('Move/rename to (full path):', join(n.dataset.mv));
        if (!to) return;
        await App.post(`/api/files/${current.container_id}/move`, { from: join(n.dataset.mv), to }).then(() => load()).catch(e => toast(e.message, 'error'));
      });
      tb.querySelectorAll('[data-edit]').forEach(n => n.onclick = async () => {
        const p = join(n.dataset.edit);
        try {
          const f = await App.get(`/api/files/${current.container_id}/read?path=${encodeURIComponent(p)}`);
          const dlg = App.modal('Edit: ' + p, `<textarea class="editor-area" id="ed-txt" spellcheck="false"></textarea>`,
            `<button class="btn btn-outline" data-close2>${t('Cancel')}</button><button class="btn btn-primary" id="ed-save">${t('Save')}</button>`);
          const ta = dlg.querySelector('#ed-txt'); ta.value = f.content;
          let dirty = false; ta.oninput = () => dirty = true;
          dlg.querySelector('[data-close2]').onclick = async () => {
            if (dirty && !(await App.confirmDialog('Discard changes?', 'You have unsaved changes.'))) return;
            dlg.close(); dlg.remove();
          };
          dlg.querySelector('#ed-save').onclick = async () => {
            await App.post(`/api/files/${current.container_id}/write`, { path: p, content: ta.value });
            toast('Saved', 'success'); dlg.close(); dlg.remove();
          };
        } catch (e) { toast(e.message, 'error'); }
      });
    } catch (e) { tb.innerHTML = `<tr><td colspan="4" class="small" style="color:var(--danger)">${esc(e.message)}</td></tr>`; }
  };
  load();

  $('f-mkdir').onclick = async () => {
    const name = prompt('New folder name:');
    if (!name) return;
    await App.post(`/api/files/${current.container_id}/mkdir`, { path: (cwd === '/' ? '' : cwd) + '/' + name }).then(load).catch(e => toast(e.message, 'error'));
  };
  $('f-upload').onchange = async (e) => {
    const fd = new FormData();
    fd.append('path', cwd);
    for (const f of e.target.files) fd.append('files', f);
    toast('Uploading…', 'info');
    await App.upload(`/api/files/${current.container_id}/upload`, fd).then(() => { toast('Uploaded', 'success'); load(); }).catch(err => toast(err.message, 'error'));
  };
  $('f-extract').onchange = async (e) => {
    const fd = new FormData();
    fd.append('path', cwd);
    fd.append('archive', e.target.files[0]);
    await App.upload(`/api/files/${current.container_id}/extract`, fd).then(() => { toast('Extracted', 'success'); load(); }).catch(err => toast(err.message, 'error'));
  };
}

// ---------- Backups tab ----------
function tabBackups(c) {
  c.innerHTML = `
    <div class="flex mb"><button class="btn btn-primary" id="b-create">＋ Create backup</button><span class="small">Target: ${esc(current.backup_target || 'local')} · Retention: ${current.backup_retention} · Schedule: ${esc(current.backup_schedule || '—')}</span></div>
    <div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
      <thead><tr><th>File</th><th>Size</th><th>Status</th><th>Created</th><th style="width:260px"></th></tr></thead>
      <tbody id="b-body"></tbody></table></div>`;
  const load = async () => {
    const list = await App.get(`/api/backups/${current.container_id}`) || [];
    $('b-body').innerHTML = list.map(b => `<tr>
      <td style="font-family:var(--font-mono);font-size:12.5px">${esc(b.file_name)}${b.note ? `<div class="small">${esc(b.note)}</div>` : ''}</td>
      <td>${fmtBytes(b.size_bytes)}</td>
      <td><span class="badge ${b.status === 'done' ? 'badge-success' : b.status === 'failed' ? 'badge-danger' : 'badge-warn'}">${esc(b.status)}</span>${b.error ? `<div class="small" style="color:var(--danger)">${esc(b.error)}</div>` : ''}</td>
      <td class="small">${relTime(b.CreatedAt || b.created_at)}</td>
      <td>
        <button class="btn btn-outline" style="padding:4px 12px" data-rs="${b.ID}">${t('Restore')}</button>
        <button class="btn btn-outline" style="padding:4px 12px" data-dl="${b.ID}" data-f="${esc(b.file_name)}">⬇</button>
        <button class="btn btn-danger" style="padding:4px 12px" data-rm="${b.ID}">🗑</button>
      </td></tr>`).join('') || `<tr><td colspan="5" class="small">No backups yet.</td></tr>`;
    $('b-body').querySelectorAll('[data-rs]').forEach(n => n.onclick = async () => {
      if (!(await App.confirmDialog('Restore backup', 'Current volume data will be overwritten. Stop the server first for consistency.', { typed: current.name }))) return;
      await App.post(`/api/backups/${current.container_id}/${n.dataset.rs}/restore`).then(() => toast('Restored', 'success')).catch(e => toast(e.message, 'error'));
    });
    $('b-body').querySelectorAll('[data-dl]').forEach(n => n.onclick = () => App.download(`/api/backups/${current.container_id}/${n.dataset.dl}/download`, n.dataset.f));
    $('b-body').querySelectorAll('[data-rm]').forEach(n => n.onclick = async () => {
      if (!(await App.confirmDialog('Delete backup', 'Delete this backup permanently?'))) return;
      await App.del(`/api/backups/${current.container_id}/${n.dataset.rm}`).then(load).catch(e => toast(e.message, 'error'));
    });
  };
  load();
  $('b-create').onclick = async () => {
    const note = prompt('Backup note (optional):') || '';
    await App.post(`/api/backups/${current.container_id}`, { note }).then(() => { toast('Backup started — refresh in a bit', 'info'); setTimeout(load, 3000); }).catch(e => toast(e.message, 'error'));
  };
}

// ---------- Activity tab ----------
async function tabActivity(c) {
  c.innerHTML = `<div class="table-wrap glass" style="border-radius:var(--radius-md)"><table class="data">
    <thead><tr><th>Time</th><th>User</th><th>Action</th><th>Details</th><th>OK</th></tr></thead><tbody id="a-body">
    <tr><td colspan="5" class="small">${t('Loading...')}</td></tr></tbody></table></div>`;
  const logs = await App.get(`/api/activity/${current.container_id}`).catch(() => []);
  $('a-body').innerHTML = (logs || []).map(l => `<tr>
    <td class="small">${relTime(l.CreatedAt || l.created_at)}</td><td>${esc(l.username)}</td>
    <td><span class="badge">${esc(l.action)}</span></td><td class="small">${esc(l.details)}</td>
    <td>${l.success ? '✅' : '❌'}</td></tr>`).join('') || `<tr><td colspan="5" class="small">No activity.</td></tr>`;
}
