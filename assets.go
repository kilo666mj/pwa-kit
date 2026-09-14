package pwakit

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"path"
)

//go:embed assets/browser.js assets/worker.js
var assets embed.FS

// AssetVersion changes when either browser or worker helper changes. Include it
// in app shell fingerprints so module upgrades also invalidate cached shells.
var AssetVersion = func() string {
	h := sha256.New()
	for _, name := range []string{"browser.js", "worker.js"} {
		data, _ := assets.ReadFile("assets/" + name)
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}()

// Handler serves the two scripts. Mount at /pwa-kit/ (or another app-owned
// prefix). Scripts are embedded in the binary; no CDN or Node build is needed.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := path.Base(r.URL.Path)
		if name != "browser.js" && name != "worker.js" {
			http.NotFound(w, r)
			return
		}
		data, _ := assets.ReadFile("assets/" + name)
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	})
}
