const API = {
    login: async (username, password) => {
        const response = await fetch('/api/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        });
        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Login failed');
        }
        return response.json();
    },

    get: async (url) => {
        const token = localStorage.getItem('token');
        const response = await fetch(url, {
            headers: { 'Authorization': `Bearer ${token}` }
        });
        if (response.status === 401) {
            localStorage.removeItem('token');
            window.location.href = '/login.html';
            return;
        }
        return response.json();
    },

    post: async (url, data) => {
        const token = localStorage.getItem('token');
        const response = await fetch(url, {
            method: 'POST',
            headers: { 
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}` 
            },
            body: JSON.stringify(data)
        });
        if (response.status === 401) {
            localStorage.removeItem('token');
            window.location.href = '/login.html';
            return;
        }
        return response.json();
    }
};

// Login page logic
if (document.getElementById('loginForm')) {
    // Redirect if already logged in
    if (localStorage.getItem('token')) {
        window.location.href = '/dashboard.html';
    }

    document.getElementById('loginForm').addEventListener('submit', async (e) => {
        e.preventDefault();
        const username = document.getElementById('username').value;
        const password = document.getElementById('password').value;
        const submitBtn = document.getElementById('submitBtn');
        const errorMsg = document.getElementById('errorMessage');

        submitBtn.disabled = true;
        submitBtn.innerHTML = '<span>Authenticating...</span>';
        errorMsg.style.display = 'none';

        try {
            const data = await API.login(username, password);
            localStorage.setItem('token', data.token);
            window.location.href = '/dashboard.html';
        } catch (err) {
            errorMsg.textContent = err.message;
            errorMsg.style.display = 'flex';
            submitBtn.disabled = false;
            submitBtn.innerHTML = '<span>Sign In</span>';
        }
    });
}

function logout() {
    localStorage.removeItem('token');
    window.location.href = '/login.html';
}
