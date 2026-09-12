importScripts('/pwa-kit/worker.js');
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
PWAKitWorker.installPushHandlers({title:'My app',tag:'my-app-update'});
