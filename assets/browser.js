/* pwa-kit: app-owned rendering, storage adapters and notification policy. */
(function (root) {
  'use strict';
  const messages = {
    checking: 'Checking notifications…', enabling: 'Enabling notifications…', disabling: 'Disabling notifications…',
    on: 'Notifications are enabled on this device.', off: 'Notifications are off on this device.',
    blocked: 'Notifications are blocked. Allow this app in device or browser notification settings.',
    install: 'On iPhone or iPad, add this app to your Home Screen in Safari, then open that icon to enable notifications.',
    unsupported: 'This browser does not support Web Push.', signedOut: 'Sign in to enable notifications.',
    error: 'Could not verify notifications. Try again.'
  };
  function isAppleMobile() { return /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1); }
  function isStandalone() { return Boolean(navigator.standalone || root.matchMedia?.('(display-mode: standalone)').matches); }
  function supported() { return 'Notification' in root && 'PushManager' in root && 'serviceWorker' in navigator; }
  function applicationServerKey(value) {
    const raw = atob(value.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - value.length % 4) % 4));
    return Uint8Array.from(raw, c => c.charCodeAt(0));
  }
  function timeout(promise, ms) {
    let timer;
    return Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Notifications could not start. Reload the app and try again.')), ms); })]).finally(() => clearTimeout(timer));
  }
  function createPushClient(options) {
    if (typeof options.save !== 'function' || typeof options.getPublicKey !== 'function') throw new Error('Push client requires save and getPublicKey adapters');
    let current = {status: 'checking', message: messages.checking, subscription: null};
    let operation = null, registrationPromise = null, destroyed = false;
    const storageKey = options.storageKey || 'pwa-kit-notifications-disabled';
    function disabledByUser() { try { return localStorage.getItem(storageKey) === '1'; } catch (_) { return false; } }
    function rememberDisabled(value) { try { value ? localStorage.setItem(storageKey, '1') : localStorage.removeItem(storageKey); } catch (_) {} }
    function publish(status, subscription = null, message = messages[status]) {
      current = {status, message, subscription};
      if (!destroyed) options.onState?.(current);
      return current;
    }
    function unavailable() {
      if (options.isAuthenticated && !options.isAuthenticated()) return 'signedOut';
      if (isAppleMobile() && !isStandalone()) return 'install';
      if (!supported()) return 'unsupported';
      if (Notification.permission === 'denied') return 'blocked';
      return '';
    }
    async function registration() {
      if (!registrationPromise) {
        registrationPromise = timeout(Promise.resolve().then(async () => {
          const result = options.registration ? await options.registration() : await navigator.serviceWorker.register(options.workerURL || '/sw.js', {updateViaCache: 'none'});
          if (!result) throw new Error('Service worker registration failed. Reload the app and try again.');
          return result.active ? result : await navigator.serviceWorker.ready;
        }), options.timeoutMs || 10000).catch(error => { registrationPromise = null; throw error; });
      }
      return registrationPromise;
    }
    async function save(subscription) {
      await options.save(subscription);
      // Permission or authentication may have changed while saving.
      const status = unavailable();
      if (status) return publish(status);
      return Notification.permission === 'granted' ? publish('on', subscription) : publish('off');
    }
    async function subscribe(reg) {
      const key = await options.getPublicKey();
      if (!key) throw new Error('Notifications are not configured on the server.');
      return reg.pushManager.subscribe({userVisibleOnly: true, applicationServerKey: applicationServerKey(key)});
    }
    function run(action) {
      if (operation) return operation;
      if (destroyed) return Promise.resolve(current);
      // Invoke synchronously: permission requests must retain Safari's user gesture.
      let work;
      try { work = action(); } catch (error) { work = Promise.reject(error); }
      operation = Promise.resolve(work).catch(error => {
        const status = unavailable();
        return publish(status || 'error', null, status ? messages[status] : (error.message || messages.error));
      }).finally(() => { operation = null; });
      return operation;
    }
    function refresh() {
      return run(async () => {
        const status = unavailable();
        if (status) return publish(status);
        if (Notification.permission !== 'granted') return publish('off');
        publish('checking');
        const reg = await registration();
        let subscription = await reg.pushManager.getSubscription();
        if (!subscription && options.renewMissing && !disabledByUser()) subscription = await subscribe(reg);
        return subscription ? save(subscription) : publish('off');
      });
    }
    function enable() {
      return run(async () => {
        const status = unavailable();
        if (status) return publish(status);
        publish('enabling');
        const permission = Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission();
        if (permission !== 'granted') return publish(permission === 'denied' ? 'blocked' : 'off');
        const reg = await registration();
        const subscription = await reg.pushManager.getSubscription() || await subscribe(reg);
        const result = await save(subscription);
        if (result.status === 'on') rememberDisabled(false);
        return result;
      });
    }
    function disable() {
      return run(async () => {
        if (typeof options.remove !== 'function') throw new Error('Disabling notifications is not configured.');
        publish('disabling');
        const reg = await registration();
        const subscription = await reg.pushManager.getSubscription();
        if (subscription) {
          await options.remove(subscription);
          if (!await subscription.unsubscribe()) throw new Error('Could not disable this device subscription. Try again.');
        }
        rememberDisabled(true);
        return publish('off');
      });
    }
    function visible() { if (document.visibilityState === 'visible') refresh(); }
    if (options.watch !== false) {
      root.addEventListener('focus', refresh); root.addEventListener('online', refresh);
      document.addEventListener('visibilitychange', visible);
    }
    function destroy() {
      destroyed = true;
      root.removeEventListener('focus', refresh); root.removeEventListener('online', refresh);
      document.removeEventListener('visibilitychange', visible);
    }
    return {refresh, enable, disable, registration, destroy, get state() { return current; }};
  }
  root.PWAKit = Object.freeze({createPushClient, isAppleMobile, isStandalone, supported, applicationServerKey});
})(globalThis);
