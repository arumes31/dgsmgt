document.getElementById('loginForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    const username = document.getElementById('username').value;
    const password = document.getElementById('password').value;
    
    DGSMgt.ui.setLoading('submitBtn', true, 'Authenticating...');

    try {
        await DGSMgt.auth.login(username, password);
        DGSMgt.ui.toast('Login successful!', 'success');
        window.location.href = '/dashboard.html';
    } catch (err) {
        DGSMgt.ui.toast(err.message, 'error');
        DGSMgt.ui.setLoading('submitBtn', false);
    }
});
