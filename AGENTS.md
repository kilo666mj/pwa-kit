# pwa-kit

Keep this module independent of application storage, authentication, recipients,
notification policy and worker caching. Preserve app-supplied TTL (including 0),
urgency, HTTP client and message presentation.

Never log push endpoints, subscription keys, raw provider responses or payloads.
A provider acceptance status is not proof of on-device receipt.

Run gofmt -w ., go test ./..., go vet ./..., node --check assets/browser.js,
node --check assets/worker.js and node --test tests/*.test.cjs after changes.
Use generated credentials and fake transports in tests. Exercise consuming app
controls when changing browser states or worker behavior.

New PWA apps should use this kit's Go package, embedded browser script and worker
helper with the examples/minimal starter and README adoption checklist.
