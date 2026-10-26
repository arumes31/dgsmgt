// Escape everything coming from the API before it is inserted as HTML
const esc = DGSMgt.api.sanitize;

let allUsers = [];
let allServers = [];
let allAssignments = [];
let allLogs = [];

async function loadData() {
    try {
        const [users, servers, assignments, logs] = await Promise.all([
            DGSMgt.api.get('/api/admin/users'),
            DGSMgt.api.get('/api/admin/servers'),
            DGSMgt.api.get('/api/admin/assignments'),
            DGSMgt.api.get('/api/admin/audit-logs')
        ]);

        allUsers = users || [];
        allServers = servers || [];
        allAssignments = assignments || [];
        allLogs = logs || [];

        filterTables();
    } catch (err) {
        DGSMgt.ui.toast('Failed to load admin data', 'error');
    }
}

function filterTables() {
    const query = document.getElementById('adminSearch').value.toLowerCase();
    
    // Users
    document.getElementById('userTableBody').innerHTML = allUsers
        .filter(u => (u.Username || u.username).toLowerCase().includes(query))
        .map(u => `
            <tr>
                <td>${esc(u.ID)}</td>
                <td style="font-weight: 500;">${esc(u.Username || u.username)}</td>
                <td><span class="badge ${u.IsAdmin || u.is_admin ? 'badge-admin' : 'badge-user'}">${u.IsAdmin || u.is_admin ? 'Admin' : 'User'}</span></td>
                <td style="text-align: right;">
                    ${(u.Username || u.username) !== 'admin' ? `<button class="btn btn-danger btn-sm" data-action="deleteUser" data-id="${esc(u.ID)}" title="Delete User">Delete</button>` : '<span style="color: #64748b; font-size: 12px;">Protected</span>'}
                </td>
            </tr>
        `).join('');

    // Servers
    document.getElementById('serverTableBody').innerHTML = allServers
        .filter(s => (s.Name || s.name).toLowerCase().includes(query) || (s.Image || s.image).toLowerCase().includes(query))
        .map(s => `
            <tr>
                <td style="font-family: monospace; color: #94a3b8">${esc(s.ID || s.id)}</td>
                <td style="font-weight: 600;">${esc(s.Name || s.name)}</td>
                <td style="font-family: monospace; font-size: 12px; color: #94a3b8">${esc(s.Image || s.image)}</td>
                <td><span style="color: ${(s.Status || s.status) === 'running' ? '#4ade80' : '#f87171'}">${esc(s.Status || s.status || 'managed')}</span></td>
                <td style="text-align: right;">
                    <button class="btn btn-danger btn-sm" data-action="deleteServer" data-id="${esc(s.ID || s.id)}" title="Delete Server and Container">Delete</button>
                </td>
            </tr>
        `).join('');

    // Assignments
    document.getElementById('assignTableBody').innerHTML = allAssignments.map(a => {
        const uid = a.UserID || a.user_id;
        const sid = a.ServerID || a.server_id;
        const u = allUsers.find(user => user.ID === uid);
        const s = allServers.find(server => server.ID === sid);
        const uName = u ? (u.Username || u.username) : uid;
        const sName = s ? (s.Name || s.name) : sid;
        
        if (query && !uName.toLowerCase().includes(query) && !sName.toLowerCase().includes(query)) return '';

        return `
            <tr>
                <td>${esc(uName)}</td>
                <td>${esc(sName)}</td>
                <td style="font-size: 11px; color: #94a3b8">
                    <span style="color: ${a.CanStart || a.can_start ? '#4ade80' : '#f87171'}">Start</span> &bull; 
                    <span style="color: ${a.CanStop || a.can_stop ? '#4ade80' : '#f87171'}">Stop</span> &bull; 
                    <span style="color: ${a.CanRestart || a.can_restart ? '#4ade80' : '#f87171'}">Restart</span> &bull; 
                    <span style="color: ${a.CanViewLogs || a.can_view_logs ? '#4ade80' : '#f87171'}">Logs</span>
                </td>
                <td style="text-align: right;"><button class="btn btn-danger btn-sm" data-action="deleteAssign" data-uid="${esc(uid)}" data-sid="${esc(sid)}">Remove</button></td>
            </tr>
        `;
    }).join('');

    // Logs
    document.getElementById('auditTableBody').innerHTML = allLogs
        .filter(l => l.username.toLowerCase().includes(query) || l.action.toLowerCase().includes(query) || l.details.toLowerCase().includes(query))
        .map(l => `
            <tr>
                <td style="font-size: 11px; color: #64748b">${esc(new Date(l.CreatedAt).toLocaleString())}</td>
                <td style="font-weight: 500;">${esc(l.username)}</td>
                <td><span style="color: #60a5fa; font-weight: 600;">${esc(l.action)}</span></td>
                <td style="font-size: 12px; color: #94a3b8">${esc(l.details)}</td>
            </tr>
        `).join('');

    DGSMgt.ui.initTooltips();
}

async function removeAndReload(url) {
    try { await DGSMgt.api.delete(url); } catch (err) { DGSMgt.ui.toast(err.message, 'error'); }
    loadData();
}

async function deleteUser(id) { DGSMgt.ui.confirm('Delete user?', () => removeAndReload(`/api/admin/users/${id}`)); }
async function deleteServer(id) { DGSMgt.ui.confirm('Warning: This will delete the server record AND the Docker container. Continue?', () => removeAndReload(`/api/admin/servers/${id}`)); }
async function deleteAssign(uid, sid) { DGSMgt.ui.confirm('Remove assignment?', () => removeAndReload(`/api/admin/assignments/${uid}/${sid}`)); }

function showNewUserModal() {
    const body = `
        <form id="newUserForm">
            <div class="form-group"><label class="form-label" for="nu-user">Username</label><input type="text" id="nu-user" class="form-input" required minlength="3"></div>
            <div class="form-group"><label class="form-label" for="nu-pass">Password</label><input type="password" id="nu-pass" class="form-input" required minlength="8"></div>
            <label class="form-checkbox" for="nu-admin"><input type="checkbox" id="nu-admin"> Grant Admin Privileges</label>
        </form>
    `;
    const footer = `<button type="button" class="btn btn-outline dg-close">Cancel</button><button type="submit" form="newUserForm" id="nu-submit" class="btn btn-primary">Create User</button>`;
    const m = DGSMgt.ui.modal({ title: 'Create User', body, footer });
    m.querySelector('.dg-close').onclick = () => m.remove();
    document.getElementById('newUserForm').onsubmit = async (e) => {
        e.preventDefault();
        DGSMgt.ui.setLoading('nu-submit', true);
        try {
            await DGSMgt.api.post('/api/admin/users', {
                username: document.getElementById('nu-user').value,
                password: document.getElementById('nu-pass').value,
                is_admin: document.getElementById('nu-admin').checked
            });
            DGSMgt.ui.toast('User created', 'success'); m.remove(); loadData();
        } catch (err) { DGSMgt.ui.toast(err.message, 'error'); DGSMgt.ui.setLoading('nu-submit', false); }
    };
}

function showNewServerModal() {
    const body = `
        <form id="newServerForm">
            <div class="form-group"><label class="form-label">Container Name</label><input type="text" id="ns-name" class="form-input" required></div>
            <div class="form-group"><label class="form-label">Docker Image</label><input type="text" id="ns-image" class="form-input" required></div>
            <div class="form-group"><label class="form-label">Ports (CSV)</label><input type="text" id="ns-ports" class="form-input" placeholder="8080:80"></div>
        </form>
    `;
    const footer = `<button type="button" class="btn btn-outline dg-close">Cancel</button><button type="submit" form="newServerForm" id="ns-submit" class="btn btn-primary">Deploy Server</button>`;
    const m = DGSMgt.ui.modal({ title: 'Deploy Server', body, footer });
    m.querySelector('.dg-close').onclick = () => m.remove();
    document.getElementById('newServerForm').onsubmit = async (e) => {
        e.preventDefault();
        DGSMgt.ui.setLoading('ns-submit', true, 'Deploying...');
        try {
            const ports = document.getElementById('ns-ports').value ? document.getElementById('ns-ports').value.split(',').map(s => s.trim()) : [];
            await DGSMgt.api.post('/api/admin/servers', {
                name: document.getElementById('ns-name').value,
                image: document.getElementById('ns-image').value,
                ports: ports
            });
            DGSMgt.ui.toast('Server deployed', 'success'); m.remove(); loadData();
        } catch (err) { DGSMgt.ui.toast(err.message, 'error'); DGSMgt.ui.setLoading('ns-submit', false); }
    };
}

function showAssignModal() {
    const uOpts = allUsers.map(u => `<option value="${esc(u.ID)}">${esc(u.Username || u.username)}</option>`).join('');
    const sOpts = allServers.map(s => `<option value="${esc(s.ID || s.id)}">${esc(s.Name || s.name)}</option>`).join('');
    const body = `
        <form id="newAssignForm">
            <div class="form-group"><label class="form-label">User</label><select id="na-user" class="form-input">${uOpts}</select></div>
            <div class="form-group"><label class="form-label">Server</label><select id="na-server" class="form-input">${sOpts}</select></div>
            <div style="display:grid; grid-template-columns:1fr 1fr; gap:10px;">
                <label class="form-checkbox"><input type="checkbox" id="na-start" checked> Start</label>
                <label class="form-checkbox"><input type="checkbox" id="na-stop" checked> Stop</label>
                <label class="form-checkbox"><input type="checkbox" id="na-restart" checked> Restart</label>
                <label class="form-checkbox"><input type="checkbox" id="na-logs" checked> Logs</label>
            </div>
        </form>
    `;
    const footer = `<button type="button" class="btn btn-outline dg-close">Cancel</button><button type="submit" form="newAssignForm" id="na-submit" class="btn btn-primary">Assign Access</button>`;
    const m = DGSMgt.ui.modal({ title: 'Assign Access', body, footer });
    m.querySelector('.dg-close').onclick = () => m.remove();
    document.getElementById('newAssignForm').onsubmit = async (e) => {
        e.preventDefault();
        DGSMgt.ui.setLoading('na-submit', true);
        try {
            await DGSMgt.api.post('/api/admin/assign', {
                user_id: parseInt(document.getElementById('na-user').value),
                server_id: parseInt(document.getElementById('na-server').value),
                can_start: document.getElementById('na-start').checked,
                can_stop: document.getElementById('na-stop').checked,
                can_restart: document.getElementById('na-restart').checked,
                can_view_logs: document.getElementById('na-logs').checked
            });
            DGSMgt.ui.toast('Access assigned', 'success'); m.remove(); loadData();
        } catch (err) { DGSMgt.ui.toast(err.message, 'error'); DGSMgt.ui.setLoading('na-submit', false); }
    };
}

Object.assign(DGSMgt.actions, {
    filterTables: () => filterTables(),
    showNewUserModal: () => showNewUserModal(),
    showNewServerModal: () => showNewServerModal(),
    showAssignModal: () => showAssignModal(),
    deleteUser: (el) => deleteUser(el.dataset.id),
    deleteServer: (el) => deleteServer(el.dataset.id),
    deleteAssign: (el) => deleteAssign(el.dataset.uid, el.dataset.sid)
});

loadData();
