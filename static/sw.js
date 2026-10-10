const CACHE_NAME = 'dgsmgt-cache-v1';
const ASSETS_TO_CACHE = [
  '/css/app.css',
  '/js/auth.js',
  '/favicon.svg',
  '/icons.svg',
  '/404.html'
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then((cache) => {
        return cache.addAll(ASSETS_TO_CACHE);
      })
  );
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME) {
            return caches.delete(cacheName);
          }
        })
      );
    })
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  // Only intercept GET requests for same-origin, and skip API calls
  if (event.request.method !== 'GET' || !event.request.url.startsWith(self.location.origin) || event.request.url.includes('/api/')) {
    return;
  }

  event.respondWith(
    caches.match(event.request)
      .then((response) => {
        // Return cached response if found
        if (response) {
          // Fetch new in background to update cache (Stale-while-revalidate strategy)
          fetch(event.request).then((networkResponse) => {
            if (networkResponse && networkResponse.status === 200) {
              caches.open(CACHE_NAME).then((cache) => {
                cache.put(event.request, networkResponse);
              });
            }
          }).catch(() => {});
          
          return response;
        }

        // Otherwise fetch from network
        return fetch(event.request).then((networkResponse) => {
          // Don't cache if not a valid response or if it's an HTML page (to ensure fresh data)
          if (!networkResponse || networkResponse.status !== 200 || networkResponse.type !== 'basic' || event.request.headers.get('accept').includes('text/html')) {
            return networkResponse;
          }

          const responseToCache = networkResponse.clone();
          caches.open(CACHE_NAME)
            .then((cache) => {
              cache.put(event.request, responseToCache);
            });

          return networkResponse;
        });
      })
  );
});