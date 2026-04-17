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
            
            if (!token && !isLoginPage) {
                window.location.href = '/login.html';
            } else if (token && isLoginPage) {
                window.location.href = '/dashboard.html';
            }
        }
    },

    // UI Helpers
    ui: {
        toast: (message, type = 'info') => {
            const toast = document.createElement('div');
            toast.className = `toast toast-${type}`;
            toast.textContent = message;
            document.body.appendChild(toast);
            
            setTimeout(() => {
                toast.classList.add('show');
                setTimeout(() => {
                    toast.classList.remove('show');
                    setTimeout(() => toast.remove(), 300);
                }, 3000);
            }, 100);
        },

        setLoading: (elementId, isLoading, loadingText = 'Loading...') => {
            const el = document.getElementById(elementId);
            if (!el) return;

            if (isLoading) {
                el.dataset.originalHtml = el.innerHTML;
                el.disabled = true;
                el.innerHTML = `<span class="spinner"></span> <span>${loadingText}</span>`;
            } else {
                el.disabled = false;
                el.innerHTML = el.dataset.originalHtml || el.innerHTML;
            }
        },

        initTooltips: () => {
            const buttons = document.querySelectorAll('button');
            buttons.forEach(btn => {
                const hasText = btn.innerText.trim().length > 0;
                const hasIconOnly = btn.querySelector('svg') && !hasText;
                
                if (hasIconOnly && !btn.title) {
                    // Try to infer title from onclick or class
                    const onclickStr = btn.onclick?.toString() || '';
                    if (onclickStr.includes('stop')) btn.title = 'Stop Server';
                    else if (onclickStr.includes('start')) btn.title = 'Start Server';
                    else if (onclickStr.includes('logout')) btn.title = 'Logout';
                    else if (onclickStr.includes('openConsole')) btn.title = 'View Logs';
                    else if (onclickStr.includes('closeModal') || onclickStr.includes('closeConsole')) btn.title = 'Close';
                    else btn.title = 'Action';
                }
                
                if (hasText && btn.hasAttribute('title')) {
                    btn.removeAttribute('title');
                }
            });
        }
    }
};

// Initial check
DGSMgt.auth.check();
document.addEventListener('DOMContentLoaded', DGSMgt.ui.initTooltips);
