const CACHE_NAME = 'dgsmgt-cache-v3';
const ASSETS_TO_CACHE = [
  '/css/app.css',
  '/css/app2.css',
  '/js/app.js',
  '/js/dashboard.js',
  '/js/dashboard-tabs.js',
  '/js/dashboard-settings.js',
  '/js/admin.js',
  '/js/admin-servers.js',
  '/js/admin-access.js',
  '/favicon.svg',
  '/icons.svg',
  '/404.html'
];

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(ASSETS_TO_CACHE)));
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((names) => Promise.all(
      names.map((n) => n !== CACHE_NAME ? caches.delete(n) : undefined)
    ))
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  if (event.request.method !== 'GET' || !event.request.url.startsWith(self.location.origin) || event.request.url.includes('/api/')) {
    return;
  }
  event.respondWith(
    caches.match(event.request).then((response) => {
      if (response) {
        fetch(event.request).then((net) => {
          if (net && net.status === 200) caches.open(CACHE_NAME).then((c) => c.put(event.request, net));
        }).catch(() => {});
        return response;
      }
      return fetch(event.request).then((net) => {
        if (!net || net.status !== 200 || net.type !== 'basic' || (event.request.headers.get('accept') || '').includes('text/html')) {
          return net;
        }
        const clone = net.clone();
        caches.open(CACHE_NAME).then((c) => c.put(event.request, clone));
        return net;
      });
    })
  );
});

// Web Push: server-down alerts etc.
self.addEventListener('push', (event) => {
  let data = { title: 'DGSMgt', body: '' };
  try { data = event.data.json(); } catch { data.body = event.data ? event.data.text() : ''; }
  event.waitUntil(self.registration.showNotification(data.title || 'DGSMgt', {
    body: data.body || '',
    icon: '/assets/logo.png',
    badge: '/assets/logo.png',
    tag: 'dgsmgt',
  }));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  event.waitUntil(clients.matchAll({ type: 'window' }).then((list) => {
    for (const c of list) { if ('focus' in c) return c.focus(); }
    return clients.openWindow('/dashboard.html');
  }));
});
