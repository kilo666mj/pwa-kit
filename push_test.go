package pwakit

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

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

func TestPublicAddressPolicy(t *testing.T) {
	for _, address := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !isPublicAddress(netip.MustParseAddr(address)) {
			t.Errorf("rejected public address %s", address)
		}
	}
	for _, address := range []string{
		"0.0.0.0", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.169.254",
		"172.16.0.1", "192.0.2.1", "192.168.1.1", "198.18.0.1", "198.51.100.1",
		"203.0.113.1", "224.0.0.1", "255.255.255.255", "::", "::1", "::ffff:127.0.0.1",
		"64:ff9b:1::1", "100::1", "2001:db8::1", "fd00::1", "fe80::1", "ff02::1",
	} {
		if isPublicAddress(netip.MustParseAddr(address)) {
			t.Errorf("accepted non-public address %s", address)
		}
	}
}

func TestPublicDialValidatesResolutionAndDialsLiteralAddress(t *testing.T) {
	lookups := map[string][]netip.Addr{
		"public.example":  {netip.MustParseAddr("1.1.1.1")},
		"private.example": {netip.MustParseAddr("10.0.0.7")},
		"mixed.example":   {netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("127.0.0.1")},
	}
	lookup := func(_ context.Context, _, host string) ([]netip.Addr, error) {
		return lookups[host], nil
	}
	var dialed []string
	dial := publicDialContext(lookup, func(_ context.Context, _, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		return nil, errors.New("test stop")
	})

	if _, err := dial(t.Context(), "tcp", "public.example:443"); err == nil {
		t.Fatal("expected fake dial failure")
	}
	if len(dialed) != 1 || dialed[0] != "1.1.1.1:443" {
		t.Fatalf("dialed = %v, want resolved public literal", dialed)
	}
	for _, address := range []string{"private.example:443", "mixed.example:443", "127.0.0.1:443", "[::1]:443"} {
		dialed = nil
		if _, err := dial(t.Context(), "tcp", address); err == nil {
			t.Fatalf("accepted %s", address)
		}
		if len(dialed) != 0 {
			t.Fatalf("dialed non-public destination %s through %v", address, dialed)
		}
	}
}

func TestPublicHTTPClientBlocksRedirects(t *testing.T) {
	requests := 0
	client := NewPublicHTTPClient(time.Second)
	client.Transport = transportFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://127.0.0.1/internal"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://push.example/device", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		t.Fatal("followed Web Push redirect")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
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
