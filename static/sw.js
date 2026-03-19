// Minimal service worker for PWA installability.
// No caching strategy — just passes requests through to the network.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("fetch", (event) => {
  // Let SSE streams bypass the service worker entirely
  if (event.request.url.includes("/api/state/stream")) return;

  event.respondWith(fetch(event.request));
});
