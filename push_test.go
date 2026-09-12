package pwakit

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(t *testing.T) (Config, Subscription) {
	t.Helper()
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Config{PublicKey: public, PrivateKey: private, Contact: "mailto:ops@example.com"}, Subscription{Endpoint: "https://web.push.apple.com/secret-endpoint", Keys: Keys{P256dh: base64.RawURLEncoding.EncodeToString(receiver.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16))}}
}
func TestContactAndKeys(t *testing.T) {
	for _, value := range []string{"ops@example.com", "mailto:ops@example.com", "https://example.com/contact"} {
		if _, err := NormalizeContact(value); err != nil {
			t.Errorf("%s: %v", value, err)
		}
	}
	for _, value := range []string{"", "mailto:mailto:ops@example.com", "admin@localhost", "mailto:admin@infra.internal", "https://app.internal", "https://localhost", "https://127.0.0.1", "https://user:pass@example.com", "Name <ops@example.com>", "mailto:ops@example.com?subject=test", "ops@bad..com", "https://bad_host.example.com"} {
		if _, err := NormalizeContact(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	cfg, _ := fixture(t)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	other, _ := fixture(t)
	cfg.PrivateKey = other.PrivateKey
	if cfg.Validate() == nil {
		t.Fatal("accepted mismatched keys")
	}
}
func TestBrowserSubscriptionJSON(t *testing.T) {
	payload := `{"endpoint":"https://web.push.apple.com/device","expirationTime":null,"keys":{"p256dh":"key","auth":"auth"}}`
	if _, err := DecodeSubscription(strings.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{payload + ` {}`, strings.Replace(payload, `null`, `"never"`, 1), strings.Replace(payload, `"expirationTime":null`, `"unknown":null`, 1), strings.Replace(payload, `https:`, `http:`, 1), `null`, strings.Repeat(" ", MaxSubscriptionBytes+1)} {
		if _, err := DecodeSubscription(strings.NewReader(value)); err == nil {
			t.Fatal("accepted malformed subscription")
		}
	}
}
func TestWireContactAndExplicitTTL(t *testing.T) {
	cfg, sub := fixture(t)
	for _, contact := range []string{"ops@example.com", "mailto:ops@example.com", "https://example.com/contact"} {
		cfg.Contact = contact
		called := false
		client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			called = true
			if r.Header.Get("TTL") != "0" || r.Header.Get("Urgency") != "high" {
				t.Fatalf("lost delivery policy")
			}
			token := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "vapid t="), ",")[0]
			parts := strings.Split(token, ".")
			if len(parts) != 3 {
				t.Fatal("missing JWT")
			}
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err != nil {
				t.Fatal(err)
			}
			var claims struct{ Sub, Aud string }
			if json.Unmarshal(payload, &claims) != nil {
				t.Fatal("invalid JWT")
			}
			want, _ := NormalizeContact(contact)
			if claims.Sub != want || claims.Aud != "https://web.push.apple.com" {
				t.Fatalf("bad claims: %+v", claims)
			}
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(""))}, nil
		})}
		if _, err := Send(t.Context(), cfg, sub, []byte(`{"title":"Test"}`), Options{TTL: 0, Urgency: "high", HTTPClient: client}); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("no delivery")
		}
	}
}
func TestSafeRejectionsAndExpiry(t *testing.T) {
	cfg, sub := fixture(t)
	for _, test := range []struct {
		status         int
		body, reason   string
		expired, retry bool
	}{
		{403, `{"reason":"BadJwtToken"}`, "BadJwtToken", false, false},
		{410, `secret-endpoint`, "Rejected", true, false}, {404, `{}`, "Rejected", true, false},
		{429, `{"reason":"TooManyRequests"}`, "TooManyRequests", false, true},
		{503, `{"reason":"secret-endpoint"}`, "Rejected", false, true},
	} {
		client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}, nil
		})}
		result, err := Send(t.Context(), cfg, sub, []byte(`{}`), Options{HTTPClient: client})
		if err == nil || result.Reason != test.reason || result.Expired() != test.expired || result.Retryable() != test.retry {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if strings.Contains(err.Error(), "secret-endpoint") {
			t.Fatal("leaked endpoint")
		}
	}
	client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New(sub.Endpoint) })}
	_, err := Send(context.Background(), cfg, sub, []byte(`{}`), Options{HTTPClient: client})
	if err == nil || strings.Contains(err.Error(), "secret-endpoint") {
		t.Fatal("unsafe transport error")
	}
}
func TestEmbeddedAssets(t *testing.T) {
	for _, name := range []string{"browser.js", "worker.js"} {
		r := httptest.NewRecorder()
		Handler().ServeHTTP(r, httptest.NewRequest("GET", "/pwa-kit/"+name, nil))
		if r.Code != 200 || !strings.Contains(r.Header().Get("Content-Type"), "javascript") || r.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal("asset unavailable")
		}
	}
	r := httptest.NewRecorder()
	Handler().ServeHTTP(r, httptest.NewRequest("GET", "/pwa-kit/private.key", nil))
	if r.Code != 404 {
		t.Fatal("unexpected asset exposed")
	}
}
