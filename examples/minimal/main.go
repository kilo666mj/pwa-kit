// Minimal is a loopback-only enrollment example. A production app supplies its
// own authenticated routes, persistent subscription store and delivery policy.
package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"

	pwakit "go.michaelspost.com/pwa-kit"
)

//go:embed web/*
var files embed.FS

func main() {
	config := pwakit.Config{PublicKey: os.Getenv("PWA_PUBLIC_KEY"), PrivateKey: os.Getenv("PWA_PRIVATE_KEY"), Contact: os.Getenv("PWA_CONTACT")}
	if err := config.Validate(); err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /pwa-kit/", pwakit.Handler())
	mux.HandleFunc("GET /api/push/key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"public_key": config.PublicKey}); err != nil {
			log.Printf("encode public key response: %v", err)
		}
	})
	// Enrollment is deliberately non-persistent in this example. Replace these
	// handlers with authenticated, user-owned storage before deploying an app.
	mux.HandleFunc("POST /api/push/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if _, err := pwakit.DecodeSubscription(r.Body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /api/push/subscriptions", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	assets, _ := fs.Sub(files, "web")
	mux.Handle("GET /", http.FileServer(http.FS(assets)))
	log.Print("Enrollment example: http://127.0.0.1:8080 (subscriptions are not persisted)")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", mux))
}
