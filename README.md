# pwa-kit

Shared Web Push transport and PWA notification helpers for Go applications.
Browser and service-worker scripts are embedded in the Go module, so one pinned
module version upgrades all three layers. No CDN, npm install or browser build
step is required by consuming apps.

## Install

```sh
go get github.com/kilo666mj/pwa-kit@v0.1.0
```

`pwa-kit` requires Go 1.26.5 or newer and tests the minimum and current Go
releases. Browser and worker helpers are versioned with the Go module, so pin
one module tag rather than copying the scripts. See the Go API on
[pkg.go.dev](https://pkg.go.dev/github.com/kilo666mj/pwa-kit).

Mount the scripts before the app's fallback route:

```go
mux.Handle("GET /pwa-kit/", pwakit.Handler())
```

Load `/pwa-kit/browser.js` before your app script. Import
`/pwa-kit/worker.js` from your app's service worker. `AssetVersion` is a content
fingerprint: include it in your app's asset version so upgrades invalidate its
cached shell. The scripts use `Cache-Control: no-cache`; service-worker
registration uses `updateViaCache: 'none'`.

## Application boundaries

| Shared kit | Application |
|---|---|
| VAPID contact normalization and key-pair validation | Secret storage and key provisioning |
| Bounded subscription JSON decoder, including `expirationTime: null` | Authentication, same-origin checks and subscription ownership |
| One push delivery, a public-address-only HTTP client and sanitized provider errors | Recipients, payloads, concurrency, TTL, urgency and retry policy |
| Permission and subscription lifecycle | Controls, styling, product wording and install UI |
| Visible push notifications, badges and safe click navigation | Worker activation, caching and optional message prefetch |

Each app keeps its own keys and subscriptions. This is a library, not a shared
notification service. `Subscription.Validate` checks shape and limits.
`NewPublicHTTPClient` resolves at dial time, refuses private and special-use
addresses, dials the validated IP directly, disables proxies, and rejects
redirects. `Send` uses that protected client when `HTTPClient` is nil; callers
that override it own equivalent outbound protections.

## Server

Construct a `Config{PublicKey, PrivateKey, Contact}` and call `Validate` at startup.
Both `ops@your-public-domain.com` and `mailto:ops@your-public-domain.com` are
accepted, as is an HTTPS contact URL. Known local-only hosts such as `.internal`
and `localhost` are rejected before delivery. Validation does not prove that a
mailbox or public hostname exists; provide a real contact.

Use `DecodeSubscription(r.Body)` on your authenticated enrollment route. It
accepts one JSON object, rejects unknown fields, and limits the body to 16 KiB.
Store the resulting subscription under the authenticated user's identity.

```go
outboundClient := pwakit.NewPublicHTTPClient(15 * time.Second)
result, err := pwakit.Send(ctx, config, subscription, payload, pwakit.Options{
    TTL: 300,
    Urgency: "normal",
    HTTPClient: outboundClient,
})
if result.Expired() {
    // Remove only this user's expired subscription from your store.
}
if err != nil {
    logger.Warn("push delivery failed", "error", err)
}
```

TTL is explicit: zero stays zero, allowing apps to prevent queued notifications
from replaying later. The kit performs no implicit retry. `Result.Retryable()`
classifies 429 and 5xx responses; the app chooses when and whether to retry.
`DeliveryError` omits endpoints, keys, payloads and raw response bodies while
retaining known provider reasons such as `BadJwtToken` and `VapidPkHashMismatch`.
An accepted response proves provider acceptance, not on-device display.

## Browser

See [the minimal enrollment example](examples/minimal/README.md). Configure
`PWAKit.createPushClient` with:

- `getPublicKey()`: return the app's VAPID public key.
- `save(subscription)` and optional `remove(subscription)`: persist/delete via
  authenticated app APIs; reject on non-success responses.
- `onState(state)`: render `checking`, `enabling`, `disabling`, `on`, `off`,
  `blocked`, `install`, `unsupported`, `signedOut` or `error`.
- Optional `isAuthenticated()`, `workerURL`, `registration()`, `renewMissing`,
  `storageKey`, `timeoutMs` and `watch`.

Call `refresh()` after sign-in. Call `enable()` **directly from the click handler**,
without awaiting other work first; Safari requires the original user gesture.
`disable()` removes server registration before unsubscribing. Explicit disables
are remembered on this origin, so `renewMissing: true` does not silently undo
them. `on` is emitted only after server registration succeeds. Reopening or
focusing the app rechecks permission and restores existing server registration.

Use `destroy()` when replacing a page or signing out; it removes event listeners.
It suppresses later UI callbacks, but does not cancel in-flight app API requests.
`client.registration()` supports local notification previews; a local preview
must not be described as a test of server delivery.

## Worker

```js
importScripts('/pwa-kit/worker.js');
PWAKitWorker.installPushHandlers({
  title: 'My app', icon: '/icons/icon-192.png', tag: 'my-app-update',
  notificationOptions: data => ({requireInteraction: Boolean(data.urgent)}),
  afterPush: data => prefetchAppContent(data.url)
});
```

A notification is displayed immediately, even if optional badging or prefetch
fails. Numeric `data.badge` updates the app badge; override `badgeCount(data)` to
return a nonnegative number or `'dot'`. Click targets are limited to the app's
origin, and failed navigation of an existing window falls back to opening one.
Caching and worker install/activate handlers remain in your app.

## New-app adoption checklist

1. Pin this module and serve its embedded scripts from the app origin.
2. Supply durable per-app keys and a real public VAPID contact; validate at startup.
3. Adapt authenticated, user-owned subscription storage and use the protected
   default transport or `NewPublicHTTPClient`.
4. Connect visible, keyboard-accessible controls to the shared state callbacks.
5. Keep notification policy and worker caching explicit in the app.
6. Test permission denial, failed saves, reload/resume and notification clicks.
7. On a physical iPhone, install from Safari, grant permission from a tap, close
   the app, send through the server, and confirm receipt. Browser mocks cannot
   establish actual APNs/device delivery.

Use [`examples/minimal`](examples/minimal/README.md) as the starting point for
new integrations. Its server is
loopback-only and intentionally does not persist subscriptions or send pushes.
Set `PWA_PUBLIC_KEY`, `PWA_PRIVATE_KEY` and `PWA_CONTACT`, then run
`go run ./examples/minimal`.

## Development

```sh
gofmt -w .
go test ./...
go vet ./...
node --check assets/browser.js
node --check assets/worker.js
node --test tests/*.test.cjs
```

The transport tests use generated keys and a captured HTTP request. Browser and
worker tests cover user-gesture timing, save failures, revoked permission,
explicit disable, renewal, startup timeout recovery and notification navigation.
