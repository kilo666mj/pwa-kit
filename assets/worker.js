/* pwa-kit: notification handling only; the app owns worker caching/lifecycle. */
(function (root) {
  'use strict';
  function localURL(value) {
    try { const url = new URL(value || '/', root.location.origin); if (url.origin === root.location.origin) return url.href; } catch (_) {}
    return root.location.origin + '/';
  }
  async function openNotification(value) {
    const target = localURL(value);
    const clients = await root.clients.matchAll({type: 'window', includeUncontrolled: true});
    for (const client of clients) {
      if (typeof client.navigate !== 'function') continue;
      try { const navigated = await client.navigate(target); if (navigated) return await navigated.focus(); } catch (_) {}
    }
    return root.clients.openWindow(target);
  }
  function installPushHandlers(options = {}) {
    root.addEventListener('push', event => {
      let data = {};
      try { const parsed = event.data?.json(); if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) data = parsed; } catch (_) {}
      const presentation = options.notificationOptions?.(data) || {};
      const notification = {
        body: data.body || '', icon: options.icon, badge: options.badge,
        tag: data.tag || options.tag || 'notification',
        ...presentation, data: {...presentation.data, url: localURL(data.url)}
      };
      // Display immediately. Badging and app prefetch never gate visibility.
      const work = [root.registration.showNotification(data.title || options.title || 'Update', notification)];
      const count = options.badgeCount ? options.badgeCount(data) : data.badge;
      if (root.navigator?.setAppBadge && (count === 'dot' || Number.isFinite(count) && count >= 0)) {
        work.push(Promise.resolve().then(() => count === 'dot' ? root.navigator.setAppBadge() : root.navigator.setAppBadge(count)).catch(() => {}));
      }
      if (options.afterPush) work.push(Promise.resolve().then(() => options.afterPush({...data, url: localURL(data.url)})).catch(() => {}));
      event.waitUntil(Promise.all(work));
    });
    root.addEventListener('notificationclick', event => {
      event.notification.close();
      event.waitUntil(openNotification(event.notification.data?.url));
    });
  }
  root.PWAKitWorker = Object.freeze({installPushHandlers, localURL, openNotification});
})(self);
