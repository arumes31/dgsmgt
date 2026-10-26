// Escape everything coming from the API before it is inserted as HTML
const esc = DGSMgt.api.sanitize;

let allServers = [];
let currentWs = null;
let metricsWs = null;
let cpuData = [];
let ramData = [];
const maxDataPoints = 30;

async function loadDashboard() {
    try {
        const me = await DGSMgt.api.get('/api/me');
        if (me && me.must_change_password) {
            // The server rejects everything else until the initial/default password is replaced
            if (!document.getElementById('dg-pwd-modal')) DGSMgt.ui.showChangePassword(true);
            return;
        }
        const servers = await DGSMgt.api.get('/api/my-servers');

        if (me) {
            document.getElementById('userDisplay').textContent = `User: ${me.username}`;
            if (me.is_admin) document.getElementById('adminLink').style.display = 'flex';
        }

        allServers = servers || [];
        updateStats(allServers);
        filterServers();
    } catch (err) { DGSMgt.ui.toast('Error connecting to backend', 'error'); }
}

function updateStats(servers) {
    const running = servers.filter(s => s.status.toLowerCase().includes('up')).length;
    document.getElementById('stat-total').textContent = servers.length;
    document.getElementById('stat-running').textContent = running;
    document.getElementById('stat-offline').textContent = servers.length - running;
}

function renderServers(servers) {
    const grid = document.getElementById('serverGrid');
    if (servers.length === 0) {
        grid.innerHTML = '<div style="grid-column:1/-1; text-align:center; padding:40px; color:#64748b; background:rgba(255,255,255,0.02); border-radius:20px; border:1px dashed #333;">No servers found matching your search.</div>';
        return;
    }
    grid.innerHTML = servers.map(s => {
        const isRunning = s.status.toLowerCase().includes('up');
        return `
        <div class="server-card">
            <div style="display:flex; justify-content:space-between; align-items:start; margin-bottom:16px;">
                <div>
                    <h3 style="margin:0; font-size:18px;">${esc(s.name)}</h3>
                    <div style="font-size:11px; color:#64748b; font-family:monospace; margin-top:2px;">${esc(s.id.substring(0,12))}</div>
                </div>
                <span class="status-badge ${isRunning ? 'status-running' : 'status-stopped'}">${esc(s.status)}</span>
            </div>
            <div style="font-size:13px; color:#94a3b8; margin-bottom:24px; flex-grow:1;">
                <div style="display:flex; justify-content:space-between; margin-bottom:4px;"><span>Image</span><span style="font-family:monospace; color:#64748b;">${esc(s.image)}</span></div>
                <div style="display:flex; justify-content:space-between;"><span>Uptime</span><span style="color:#64748b;">${esc(s.uptime || 'Offline')}</span></div>
            </div>
            <div style="display:flex; gap:8px;">
                ${isRunning 
                    ? `<button data-action="serverAction" data-type="stop" data-id="${esc(s.id)}" class="btn btn-danger btn-sm" style="flex:1">Stop</button>`
                    : `<button data-action="serverAction" data-type="start" data-id="${esc(s.id)}" class="btn btn-primary btn-sm" style="flex:1">Start</button>`
                }
                <button data-action="serverAction" data-type="restart" data-id="${esc(s.id)}" class="btn btn-outline btn-sm" title="Restart"><svg width="16" height="16" style="display:block;"><path d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" stroke="currentColor" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></button>
                <button data-action="openConsole" data-id="${esc(s.id)}" data-name="${esc(s.name)}" class="btn btn-outline btn-sm" title="Logs"><svg width="16" height="16"><use href="/icons.svg#icon-logs"></use></svg></button>
                ${isRunning ? `<button data-action="openMetrics" data-id="${esc(s.id)}" data-name="${esc(s.name)}" class="btn btn-outline btn-sm" title="Metrics"><svg width="16" height="16"><use href="/icons.svg#icon-terminal"></use></svg></button>` : ''}
            </div>
        </div>`;
    }).join('');
    DGSMgt.ui.initTooltips();
}

const debouncedFilterServers = DGSMgt.ui.debounce(() => {
    const query = document.getElementById('serverSearch').value.toLowerCase();
    renderServers(allServers.filter(s => s.name.toLowerCase().includes(query) || s.id.toLowerCase().includes(query)));
}, 300);

function filterServers() {
    debouncedFilterServers();
}

async function handleAction(id, type, btn) {
    const exec = async () => {
        const old = btn.innerHTML;
        btn.disabled = true; btn.innerHTML = '<span class="spinner" style="margin:0"></span>';
        try {
            await DGSMgt.api.post(`/api/action/${encodeURIComponent(id)}/${type}`, {});
            DGSMgt.ui.toast(`${type.charAt(0).toUpperCase() + type.slice(1)} command sent`, 'success');
            setTimeout(loadDashboard, 1000);
        } catch (err) { DGSMgt.ui.toast(err.message, 'error'); btn.disabled = false; btn.innerHTML = old; }
    };
    if (type === 'stop' || type === 'restart') DGSMgt.ui.confirm(`Are you sure you want to ${type} this server?`, exec);
    else exec();
}

const ansiRegex = /[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g;
function ansiToHtml(text) {
    const colors = { 30:'black', 31:'#ef4444', 32:'#4ade80', 33:'#facc15', 34:'#60a5fa', 35:'#c084fc', 36:'#2dd4bf', 37:'white', 90:'gray' };
    let html = text.replace(/</g, '&lt;').replace(/>/g, '&gt;'), spanOpen = false;
    html = html.replace(/\x1b\[([0-9;]*)[mG]/g, (m, codes) => {
        if (codes === '0' || codes === '') { const res = spanOpen ? '</span>' : ''; spanOpen = false; return res; }
        const parts = codes.split(';'); let styles = [];
        for (let code of parts) { if (colors[code]) styles.push(`color: ${colors[code]}`); else if (code === '1') styles.push(`font-weight: bold`); }
        if (styles.length > 0) { const p = spanOpen ? '</span>' : ''; spanOpen = true; return `${p}<span style="${styles.join(';')}">`; }
        return '';
    });
    return spanOpen ? html + '</span>' : html;
}

function openConsole(id, name) {
    const body = `<div id="consoleOutput" class="console-output custom-scrollbar">Connecting to stream...</div>`;
    const m = DGSMgt.ui.modal({ title: `Logs: ${name}`, body, maxWidth: '900px', onClose: () => { if(currentWs) currentWs.close(); } });
    const output = m.querySelector('#consoleOutput');
    if (currentWs) currentWs.close();
    
    currentWs = DGSMgt.ws.connect(`/api/logs/${encodeURIComponent(id)}`, (e) => {
        const d = document.createElement('div'); 
        d.innerHTML = ansiToHtml(e.data);
        output.appendChild(d); 
        if (output.childNodes.length > 500) output.removeChild(output.firstChild);
        output.scrollTop = output.scrollHeight;
    }, () => {
        output.innerHTML += '<div style="color: #4ade80;">[Connected]</div>';
    }, (e, intentional) => {
        if (!intentional) output.innerHTML += '<div style="color: #facc15;">[Disconnected - Reconnecting...]</div>';
    });
}

function openMetrics(id, name) {
    const body = `
        <div style="display:flex; flex-direction:column; gap:20px;">
            <div><div style="display:flex; justify-content:space-between; margin-bottom:8px;"><span class="stat-label">CPU Usage</span><span id="cpuText" style="color:#4ade80; font-family:monospace;">0.00%</span></div><div class="chart-container"><canvas id="cpuCanvas"></canvas></div></div>
            <div><div style="display:flex; justify-content:space-between; margin-bottom:8px;"><span class="stat-label">RAM Usage</span><span id="ramText" style="color:#60a5fa; font-family:monospace;">0 MB / 0 MB</span></div><div class="chart-container"><canvas id="ramCanvas"></canvas></div></div>
        </div>
    `;
    const m = DGSMgt.ui.modal({ title: `Metrics: ${name}`, body, maxWidth: '700px', onClose: () => { if(metricsWs) metricsWs.close(); } });
    cpuData = new Array(maxDataPoints).fill(0); ramData = new Array(maxDataPoints).fill(0);
    
    const draw = () => {
        if(!document.getElementById('cpuCanvas')) return;
        drawChart('cpuCanvas', cpuData, '#4ade80', 'rgba(74, 222, 128, 0.1)');
        drawChart('ramCanvas', ramData, '#60a5fa', 'rgba(96, 165, 250, 0.1)');
    };

    if (metricsWs) metricsWs.close();
    
    metricsWs = DGSMgt.ws.connect(`/api/metrics/${encodeURIComponent(id)}`, (e) => {
        try {
            const stats = JSON.parse(e.data);
            const cpuD = stats.cpu_stats.cpu_usage.total_usage - stats.precpu_stats.cpu_usage.total_usage;
            const sysD = stats.cpu_stats.system_cpu_usage - stats.precpu_stats.system_cpu_usage;
            const cpuP = (sysD > 0 && cpuD > 0) ? (cpuD / sysD) * stats.cpu_stats.online_cpus * 100 : 0;
            const memU = stats.memory_stats.usage - (stats.memory_stats.stats?.cache || 0);
            const memL = stats.memory_stats.limit;
            const memP = memL > 0 ? (memU / memL) * 100 : 0;

            document.getElementById('cpuText').textContent = `${cpuP.toFixed(2)}%`;
            document.getElementById('ramText').textContent = `${(memU/1024/1024).toFixed(1)} MB / ${(memL/1024/1024/1024).toFixed(1)} GB`;

            cpuData.push(Math.min(cpuP, 100)); cpuData.shift();
            ramData.push(Math.min(memP, 100)); ramData.shift();
            draw();
        } catch(err) {}
    });
    setTimeout(draw, 100);
}

function drawChart(id, data, stroke, fill) {
    const c = document.getElementById(id); if (!c) return;
    const r = c.parentElement.getBoundingClientRect();
    c.width = r.width * window.devicePixelRatio; c.height = r.height * window.devicePixelRatio;
    const ctx = c.getContext('2d'); ctx.scale(window.devicePixelRatio, window.devicePixelRatio);
    const w = r.width, h = r.height, p = 5, cw = w - p*2, ch = h - p*2, step = cw / (maxDataPoints-1);
    ctx.clearRect(0,0,w,h);
    ctx.beginPath(); ctx.strokeStyle = 'rgba(255,255,255,0.03)'; ctx.lineWidth = 1;
    for(let i=0; i<=4; i++) { const y = p + (ch/4)*i; ctx.moveTo(p,y); ctx.lineTo(w-p,y); } ctx.stroke();
    ctx.beginPath(); ctx.strokeStyle = stroke; ctx.lineWidth = 2; ctx.lineJoin = 'round';
    data.forEach((v, i) => { const x = p + i*step, y = h - p - (v/100)*ch; if(i===0) ctx.moveTo(x,y); else ctx.lineTo(x,y); });
    ctx.stroke(); ctx.lineTo(w-p, h-p); ctx.lineTo(p, h-p); ctx.closePath(); ctx.fillStyle = fill; ctx.fill();
}

Object.assign(DGSMgt.actions, {
    filterServers: () => filterServers(),
    serverAction: (el) => handleAction(el.dataset.id, el.dataset.type, el),
    openConsole: (el) => openConsole(el.dataset.id, el.dataset.name),
    openMetrics: (el) => openMetrics(el.dataset.id, el.dataset.name)
});

loadDashboard(); setInterval(loadDashboard, 30000);
