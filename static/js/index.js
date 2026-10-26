window.onload = () => {
    const token = localStorage.getItem('token');
    if (token) {
        window.location.replace('/dashboard.html');
    } else {
        window.location.replace('/login.html');
    }
};
    
