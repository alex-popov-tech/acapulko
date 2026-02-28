// Minimal service worker for PWA installability.
// No caching strategy — just passes requests through to the network.
self.addEventListener("fetch", (event) => {
  event.respondWith(fetch(event.request));
});
