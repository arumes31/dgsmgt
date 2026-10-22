/**
 * DGSMgt Core JavaScript Utility
 */

const DGSMgt = {
    // API Utilities
    api: {
        token: () => localStorage.getItem('token'),
        
        headers: () => ({
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${localStorage.getItem('token')}`
        }),

        // Escapes text for use inside HTML markup, including attribute values
        sanitize: (text) => {
            const div = document.createElement('div');
            div.textContent = text == null ? '' : String(text);
            return div.innerHTML.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
        },

        // API responses are wrapped as { data, error, meta }
        unwrap: (json) => (json && typeof json === 'object' && 'data' in json) ? json.data : json,

        request: async (url, options = {}) => {
            const defaultOptions = {
                headers: DGSMgt.api.headers()
            };
            
            try {
                let response;
                try {
                    response = await fetch(url, { ...defaultOptions, ...options });
                } catch (networkErr) {
                    throw new Error('Network error: Unable to connect to the server');
                }
                
                if (response.status === 401) {
                    DGSMgt.auth.logout();
                    return null;
                }

                if (!response.ok) {
                    const text = await response.text().catch(() => '');
                    let message = text.trim();
                    try { message = JSON.parse(text).error || message; } catch (e) { /* plain text error */ }
                    throw new Error(message || `Request failed with status ${response.status}`);
                }

                if (response.status === 204) return true;
                return DGSMgt.api.unwrap(await response.json());
            } catch (err) {
                console.error(`API Error (${url}):`, err);
                throw err;
            }
        },

        get: (url) => DGSMgt.api.request(url),
        post: (url, data) => DGSMgt.api.request(url, {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        put: (url, data) => DGSMgt.api.request(url, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        delete: (url) => DGSMgt.api.request(url, { method: 'DELETE' })
    },

    // Authentication
    auth: {
        login: async (username, password) => {
            let response;
            try {
                response = await fetch('/api/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username, password })
                });
            } catch (networkErr) {
                throw new Error('Network error: Unable to connect to the server');
            }

            if (!response.ok) {
                const error = await response.json().catch(() => ({ error: 'Login failed' }));
                throw new Error(error.error || 'Invalid credentials');
            }

            const data = DGSMgt.api.unwrap(await response.json());
            localStorage.setItem('token', data.token);
            return data;
        },

        logout: () => {
            const token = localStorage.getItem('token');
            localStorage.removeItem('token');
            const redirect = () => { window.location.href = '/login.html'; };
            if (!token) return redirect();
            // Revoke the token server side; ignore failures (e.g. already expired)
            fetch('/api/logout', { method: 'POST', headers: { 'Authorization': `Bearer ${token}` }, keepalive: true })
                .catch(() => {})
                .finally(redirect);
        },

        check: () => {
            const token = localStorage.getItem('token');
            const isLoginPage = window.location.pathname.includes('login.html');
            const isIndexPage = window.location.pathname === '/' || window.location.pathname === '/index.html';
            
            if (!token && !isLoginPage && !isIndexPage) {
                window.location.href = '/login.html';
            } else if (token && isLoginPage) {
                window.location.href = '/dashboard.html';
            }
        }
    },

    // UI Helpers
    ui: {
        toast: (message, type = 'info') => {
            let container = document.getElementById('toast-container');
            if (!container) {
                container = document.createElement('div');
                container.id = 'toast-container';
                container.style.cssText = 'position: fixed; bottom: 24px; right: 24px; display: flex; flex-direction: column; gap: 12px; z-index: 9999; pointer-events: none;';
                document.body.appendChild(container);
            }

            const toast = document.createElement('div');
            toast.className = `toast toast-${type}`;
            toast.style.pointerEvents = 'auto';
            toast.innerHTML = `
                <div style="display: flex; justify-content: space-between; align-items: center; gap: 16px;">
                    <span class="toast-message"></span>
                    <button type="button" class="toast-close" style="background:none; border:none; color:inherit; cursor:pointer; padding:0; display:flex; opacity:0.7;">
                        <svg width="14" height="14" aria-hidden="true"><use href="/icons.svg#icon-close"></use></svg>
                    </button>
                </div>
            `;
            // Messages may contain server or user supplied text, never render them as HTML
            toast.querySelector('.toast-message').textContent = message;
            toast.querySelector('.toast-close').addEventListener('click', () => toast.remove());
            container.appendChild(toast);
            
            setTimeout(() => {
                toast.classList.add('show');
                setTimeout(() => {
                    toast.classList.remove('show');
                    setTimeout(() => toast.remove(), 300);
                }, 4000);
            }, 50);
        },

        setLoading: (elementId, isLoading, loadingText = 'Loading...') => {
            const el = typeof elementId === 'string' ? document.getElementById(elementId) : elementId;
            if (!el) return;

            if (isLoading) {
                el.dataset.originalHtml = el.innerHTML;
                el.disabled = true;
                el.innerHTML = `<span class="spinner" style="margin-right: 8px;"></span><span>${loadingText}</span>`;
            } else {
                el.disabled = false;
                el.innerHTML = el.dataset.originalHtml || el.innerHTML;
            }
        },

        initTooltips: () => {
            const buttons = document.querySelectorAll('button');
            buttons.forEach(btn => {
                const hasText = btn.innerText.trim().length > 0;
                const hasIconOnly = (btn.querySelector('svg') || btn.querySelector('use')) && !hasText;
                
                if (hasIconOnly && !btn.title) {
                    const onclickStr = btn.dataset.action || '';
                    const innerHtml = btn.innerHTML.toLowerCase();
                    if (onclickStr.includes('stop') || innerHtml.includes('stop')) btn.title = 'Stop Server';
                    else if (onclickStr.includes('start') || innerHtml.includes('start')) btn.title = 'Start Server';
                    else if (onclickStr.includes('logout')) btn.title = 'Logout';
                    else if (onclickStr.includes('openConsole') || innerHtml.includes('logs')) btn.title = 'View Logs';
                    else if (onclickStr.includes('closeModal') || onclickStr.includes('closeConsole')) btn.title = 'Close';
                    else if (innerHtml.includes('metrics')) btn.title = 'View Metrics';
                    else btn.title = 'Action';
                }
                
                if (hasText && btn.hasAttribute('title')) {
                    btn.removeAttribute('title');
                }
            });
        },

        modal: (options) => {
            const { title, body, footer, id = 'dg-modal', maxWidth = '500px', onClose } = options;
            const existing = document.getElementById(id);
            if (existing) existing.remove();

            const dialog = document.createElement('dialog');
            dialog.id = id;
            dialog.className = 'modal-dialog';
            dialog.style.maxWidth = maxWidth;
            
            dialog.innerHTML = `
                <div class="modal-content" role="document">
                    <div class="modal-header">
                        <h3 id="${id}-title"></h3>
                        <button type="button" class="btn btn-outline btn-icon dg-modal-close" title="Close" aria-label="Close modal">
                            <svg width="18" height="18" aria-hidden="true"><use href="/icons.svg#icon-close"></use></svg>
                        </button>
                    </div>
                    <div class="modal-body custom-scrollbar">
                        ${body}
                    </div>
                    ${footer ? `<div class="modal-footer">${footer}</div>` : ''}
                </div>
            `;
            
            // Titles often contain server names, so they are set as text
            dialog.querySelector('.modal-header h3').textContent = title;
            document.body.appendChild(dialog);

            const close = () => {
                dialog.close();
                dialog.remove();
                if (onClose) onClose();
            };

            dialog.querySelector('.dg-modal-close').onclick = close;
            
            // Close on backdrop click
            dialog.addEventListener('click', (e) => {
                const rect = dialog.getBoundingClientRect();
                const isInDialog = (rect.top <= e.clientY && e.clientY <= rect.top + rect.height &&
                                    rect.left <= e.clientX && e.clientX <= rect.left + rect.width);
                if (!isInDialog) {
                    close();
                }
            });

            dialog.addEventListener('cancel', (e) => {
                e.preventDefault();
                close();
            });

            dialog.showModal();

            return dialog;
        },

        confirm: (message, onConfirm) => {
            const body = `
                <div style="text-align: center; padding: 10px 0;">
                    <div style="width: 56px; height: 56px; background: rgba(239, 68, 68, 0.1); color: #ef4444; border-radius: 50%; display: flex; align-items: center; justify-content: center; margin: 0 auto 20px;">
                        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                    </div>
                    <p style="color: #94a3b8; font-size: 15px; line-height: 1.5; margin: 0;">${DGSMgt.api.sanitize(message)}</p>
                </div>
            `;
            const footer = `
                <button class="btn btn-outline dg-confirm-cancel" style="min-width: 100px;">Cancel</button>
                <button class="btn btn-danger dg-confirm-yes" style="min-width: 100px;">Confirm</button>
            `;
            
            const modal = DGSMgt.ui.modal({ title: 'Confirm Action', body, footer, id: 'dg-confirm', maxWidth: '400px' });
            
            modal.querySelector('.dg-confirm-cancel').onclick = () => modal.remove();
            modal.querySelector('.dg-confirm-yes').onclick = () => {
                modal.remove();
                onConfirm();
            };
            modal.querySelector('.dg-confirm-yes').focus();
        },

        // forced: the server requires a new password before anything else can be used
        showChangePassword: (forced = false) => {
            const notice = forced
                ? '<p style="color: #facc15; font-size: 14px; margin: 0 0 16px;">You must set a new password before you can continue.</p>'
                : '';
            const body = `
                ${notice}
                <form id="dg-pwd-form">
                    <div class="form-group">
                        <label class="form-label" for="dg-new-pwd">New Password</label>
                        <input type="password" id="dg-new-pwd" class="form-input" required minlength="8" placeholder="At least 8 characters">
                    </div>
                    <div class="form-group">
                        <label class="form-label" for="dg-conf-pwd">Confirm New Password</label>
                        <input type="password" id="dg-conf-pwd" class="form-input" required minlength="8">
                    </div>
                </form>
            `;
            const footer = `
                <button type="button" class="btn btn-outline dg-pwd-cancel">Cancel</button>
                <button type="submit" form="dg-pwd-form" id="dg-pwd-submit" class="btn btn-primary">Update Password</button>
            `;
            
            const modal = DGSMgt.ui.modal({
                title: 'Change Password', body, footer, id: 'dg-pwd-modal',
                onClose: forced ? () => DGSMgt.auth.logout() : null
            });
            modal.querySelector('.dg-pwd-cancel').onclick = () => forced ? DGSMgt.auth.logout() : modal.remove();
            modal.querySelector('#dg-new-pwd').focus();

            document.getElementById('dg-pwd-form').onsubmit = async (e) => {
                e.preventDefault();
                const pwd = document.getElementById('dg-new-pwd').value;
                const conf = document.getElementById('dg-conf-pwd').value;

                if (pwd !== conf) {
                    DGSMgt.ui.toast('Passwords do not match', 'error');
                    return;
                }

                DGSMgt.ui.setLoading('dg-pwd-submit', true);
                try {
                    const res = await DGSMgt.api.post('/api/me/password', { password: pwd });
                    // Changing the password revokes old tokens; the response carries a fresh one
                    if (res && res.token) localStorage.setItem('token', res.token);
                    DGSMgt.ui.toast('Password updated successfully', 'success');
                    modal.remove();
                    if (forced) window.location.reload();
                } catch (err) {
                    DGSMgt.ui.toast(err.message, 'error');
                } finally {
                    DGSMgt.ui.setLoading('dg-pwd-submit', false);
                }
            };
        },

        debounce: (func, wait) => {
            let timeout;
            return function executedFunction(...args) {
                const later = () => {
                    clearTimeout(timeout);
                    func(...args);
                };
                clearTimeout(timeout);
                timeout = setTimeout(later, wait);
            };
        }
    },

    // WebSocket Helper with auto-reconnect
    ws: {
        connect: (path, onMessage, onOpen = null, onClose = null) => {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const token = DGSMgt.api.token();
            if (!token) return null;

            let socket = null;
            let reconnectAttempts = 0;
            const maxAttempts = 5;
            let intentionalClose = false;

            const connect = async () => {
                // Browsers cannot send headers on WebSockets, so authenticate with a
                // short-lived single-use ticket instead of putting the JWT in the URL
                let ticket;
                try {
                    const res = await DGSMgt.api.post('/api/ws-ticket', {});
                    ticket = res && res.ticket;
                } catch (err) {
                    console.error('WebSocket ticket request failed:', err);
                }
                if (intentionalClose || !ticket) return;

                socket = new WebSocket(`${protocol}//${window.location.host}${path}?ticket=${encodeURIComponent(ticket)}`);

                socket.onopen = (e) => {
                    reconnectAttempts = 0;
                    if (onOpen) onOpen(e);
                };

                socket.onmessage = onMessage;

                socket.onclose = (e) => {
                    if (onClose) onClose(e, intentionalClose);
                    if (!intentionalClose && reconnectAttempts < maxAttempts) {
                        const timeout = Math.min(1000 * Math.pow(2, reconnectAttempts), 10000);
                        reconnectAttempts++;
                        console.log(`WebSocket closed. Reconnecting in ${timeout}ms...`);
                        setTimeout(connect, timeout);
                    }
                };

                socket.onerror = (err) => {
                    console.error('WebSocket Error:', err);
                    socket.close(); // Force close to trigger reconnect logic
                };
            };

            connect();

            return {
                close: () => {
                    intentionalClose = true;
                    if (socket) socket.close();
                },
                send: (data) => {
                    if (socket && socket.readyState === WebSocket.OPEN) {
                        socket.send(data);
                    }
                }
            };
        }
    }
};

// Click/input handlers registered by name and bound through data-action / data-input
// attributes, so pages need no inline event handlers (the CSP blocks inline scripts)
DGSMgt.actions = {
    toggleSidebar: () => toggleSidebar(),
    logout: () => DGSMgt.auth.logout(),
    changePassword: () => DGSMgt.ui.showChangePassword()
};
document.addEventListener('click', (e) => {
    const el = e.target.closest('[data-action]');
    const handler = el && DGSMgt.actions[el.dataset.action];
    if (!handler) return;
    if (el.tagName === 'A') e.preventDefault();
    handler(el, e);
});
document.addEventListener('input', (e) => {
    const el = e.target.closest('[data-input]');
    const handler = el && DGSMgt.actions[el.dataset.input];
    if (handler) handler(el, e);
});

// Initial check
DGSMgt.auth.check();
document.addEventListener('DOMContentLoaded', DGSMgt.ui.initTooltips);
function toggleSidebar() {
    const s = document.getElementById('sidebar');
    const o = document.getElementById('sidebarOverlay');
    if(s && o) {
        s.classList.toggle('open');
        o.classList.toggle('show');
    }
}

// Service Worker Registration
if ('serviceWorker' in navigator) {
    window.addEventListener('load', () => {
        navigator.serviceWorker.register('/sw.js').catch(err => {
            console.error('ServiceWorker registration failed: ', err);
        });
    });
}
