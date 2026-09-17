# Minimal enrollment example

This loopback-only app demonstrates the complete asset and enrollment wiring:
the embedded browser helper, an app-owned service worker that imports the
shared worker helper, visible permission controls, VAPID public-key delivery,
and subscription decoding.

It deliberately does not authenticate users, persist subscriptions, or send
notifications. Copy the integration shape, not those development-only policy
choices.

Generate a VAPID key pair for local development, then replace the placeholders
with those values and a real public contact before running:

```sh
PWA_PUBLIC_KEY='REDACTED' \
PWA_PRIVATE_KEY='REDACTED' \
PWA_CONTACT='mailto:ops@your-public-domain.com' \
go run ./examples/minimal
```

Open `http://127.0.0.1:8080`. Before adapting the example for production,
follow the repository [adoption checklist](../../README.md#new-app-adoption-checklist):
add authentication and same-origin checks, store each subscription under its
user, define delivery policy in the app, and test receipt on a physical device.
