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

        sanitize: (text) => {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        },

        request: async (url, options = {}) => {
            const defaultOptions = {
                headers: DGSMgt.api.headers()
            };
            
            try {
                const response = await fetch(url, { ...defaultOptions, ...options });
                
                if (response.status === 401) {
                    DGSMgt.auth.logout();
                    return null;
                }

                if (!response.ok) {
                    const error = await response.json().catch(() => ({ error: 'Unknown error' }));
                    throw new Error(error.error || `Request failed with status ${response.status}`);
                }

                return response.json();
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
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password })
            });

            if (!response.ok) {
                const error = await response.json().catch(() => ({ error: 'Login failed' }));
                throw new Error(error.error || 'Invalid credentials');
            }

            const data = await response.json();
            localStorage.setItem('token', data.token);
            return data;
        },

        logout: () => {
            localStorage.removeItem('token');
            window.location.href = '/login.html';
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
        toasts: [],
        toast: (message, type = 'info') => {
            const container = document.getElementById('toast-container') || (() => {
                const c = document.createElement('div');
                c.id = 'toast-container';
                c.style.cssText = 'position: fixed; bottom: 24px; right: 24px; display: flex; flex-direction: column; gap: 12px; z-index: 9999;';
                document.body.appendChild(c);
                return c;
            })();

            const toast = document.createElement('div');
            toast.className = `toast toast-${type}`;
            toast.style.margin = '0'; // Reset margin for container stacking
            toast.innerHTML = `
                <div style="display: flex; justify-content: space-between; align-items: center; gap: 16px;">
                    <span>${message}</span>
                    <button onclick="this.parentElement.parentElement.remove()" style="background:none; border:none; color:inherit; cursor:pointer; padding:0; display:flex; opacity:0.7;">
                        <svg width="14" height="14"><use href="/icons.svg#icon-close"></use></svg>
                    </button>
                </div>
            `;
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
            const el = document.getElementById(elementId);
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
                    const onclickStr = btn.onclick?.toString() || '';
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

        confirm: (message, onConfirm) => {
            const existing = document.getElementById('dgsmgt-confirm');
            if (existing) existing.remove();

            const overlay = document.createElement('div');
            overlay.id = 'dgsmgt-confirm';
            overlay.className = 'modal-overlay';
            overlay.style.display = 'flex';
            overlay.style.zIndex = '10000';
            
            overlay.innerHTML = `
                <div class="modal" role="dialog" aria-modal="true">
                    <div style="padding: 32px; text-align: center;">
                        <div style="width: 56px; height: 56px; background: rgba(239, 68, 68, 0.1); color: #ef4444; border-radius: 50%; display: flex; align-items: center; justify-content: center; margin: 0 auto 20px;">
                            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                        </div>
                        <h3 style="margin: 0 0 12px 0; font-size: 18px; font-weight: 700;">Confirm Action</h3>
                        <p style="color: #94a3b8; font-size: 15px; line-height: 1.5; margin: 0 0 24px 0;">${message}</p>
                        <div style="display: flex; gap: 12px; justify-content: center;">
                            <button id="dg-confirm-cancel" class="btn btn-outline" style="min-width: 100px;">Cancel</button>
                            <button id="dg-confirm-yes" class="btn btn-danger" style="min-width: 100px;">Confirm</button>
                        </div>
                    </div>
                </div>
            `;
            
            document.body.appendChild(overlay);
            
            const cancelBtn = document.getElementById('dg-confirm-cancel');
            const confirmBtn = document.getElementById('dg-confirm-yes');
            
            confirmBtn.focus();

            const close = () => overlay.remove();
            cancelBtn.onclick = close;
            overlay.onclick = (e) => { if(e.target === overlay) close(); };
            
            confirmBtn.onclick = () => {
                close();
                onConfirm();
            };

            // Keyboard support
            const handleKey = (e) => {
                if (e.key === 'Escape') close();
                if (e.key === 'Tab') {
                    if (e.shiftKey && document.activeElement === cancelBtn) { e.preventDefault(); confirmBtn.focus(); }
                    else if (!e.shiftKey && document.activeElement === confirmBtn) { e.preventDefault(); cancelBtn.focus(); }
                }
            };
            window.addEventListener('keydown', handleKey);
            // Cleanup event listener when removed
            const observer = new MutationObserver((mutations) => {
                if (!document.body.contains(overlay)) {
                    window.removeEventListener('keydown', handleKey);
                    observer.disconnect();
                }
            });
            observer.observe(document.body, { childList: true });
        },

        showChangePassword: () => {
            const existing = document.getElementById('dgsmgt-pwd-modal');
            if (existing) existing.remove();

            const overlay = document.createElement('div');
            overlay.id = 'dgsmgt-pwd-modal';
            overlay.className = 'modal-overlay';
            overlay.style.display = 'flex';
            overlay.style.zIndex = '9998';

            overlay.innerHTML = `
                <div class="modal" role="dialog" aria-modal="true">
                    <div class="modal-header">
                        <h3>Change Password</h3>
                        <button onclick="document.getElementById('dgsmgt-pwd-modal').remove()" class="btn btn-outline btn-icon" title="Close"><svg width="18" height="18"><use href="/icons.svg#icon-close"></use></svg></button>
                    </div>
                    <form id="dgsmgt-pwd-form">
                        <div class="modal-body custom-scrollbar">
                            <div class="form-group">
                                <label class="form-label" for="dg-new-pwd">New Password</label>
                                <input type="password" id="dg-new-pwd" class="form-input" required minlength="8" placeholder="At least 8 characters">
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="dg-conf-pwd">Confirm New Password</label>
                                <input type="password" id="dg-conf-pwd" class="form-input" required minlength="8">
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" onclick="document.getElementById('dgsmgt-pwd-modal').remove()" class="btn btn-outline">Cancel</button>
                            <button type="submit" id="dgsmgt-pwd-submit" class="btn btn-primary">Update Password</button>
                        </div>
                    </form>
                </div>
            `;

            document.body.appendChild(overlay);
            document.getElementById('dg-new-pwd').focus();

            document.getElementById('dgsmgt-pwd-form').onsubmit = async (e) => {
                e.preventDefault();
                const pwd = document.getElementById('dg-new-pwd').value;
                const conf = document.getElementById('dg-conf-pwd').value;

                if (pwd !== conf) {
                    DGSMgt.ui.toast('Passwords do not match', 'error');
                    return;
                }

                DGSMgt.ui.setLoading('dgsmgt-pwd-submit', true);
                try {
                    await DGSMgt.api.post('/api/me/password', { password: pwd });
                    DGSMgt.ui.toast('Password updated successfully', 'success');
                    overlay.remove();
                } catch (err) {
                    DGSMgt.ui.toast(err.message, 'error');
                } finally {
                    DGSMgt.ui.setLoading('dgsmgt-pwd-submit', false);
                }
            };
        }
    }
};

// Initial check
DGSMgt.auth.check();
document.addEventListener('DOMContentLoaded', DGSMgt.ui.initTooltips);
