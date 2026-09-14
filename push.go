// Package pwakit provides Web Push transport and embedded PWA helpers.
// Applications own authentication, storage, recipient selection and notification policy.
package pwakit

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Subscription accepts the browser's PushSubscription.toJSON() representation,
// including Safari's null expirationTime.
type Subscription struct {
	Endpoint       string   `json:"endpoint"`
	ExpirationTime *float64 `json:"expirationTime,omitempty"`
	Keys           Keys     `json:"keys"`
}
type Keys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

const MaxSubscriptionBytes = 16 << 10

// DecodeSubscription validates a bounded, single JSON object. Authentication,
// ownership and DNS-aware outbound access policy belong to the application.
func DecodeSubscription(reader io.Reader) (Subscription, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxSubscriptionBytes+1))
	if err != nil || len(data) > MaxSubscriptionBytes {
		return Subscription{}, errors.New("invalid push subscription")
	}
	var sub Subscription
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&sub) != nil || decoder.Decode(new(any)) != io.EOF {
		return Subscription{}, errors.New("invalid push subscription")
	}
	if err := sub.Validate(); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}
func (s Subscription) Validate() error {
	endpoint, err := url.Parse(s.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" || len(s.Endpoint) > 4096 || len(s.Keys.P256dh) == 0 || len(s.Keys.P256dh) > 512 || len(s.Keys.Auth) == 0 || len(s.Keys.Auth) > 512 {
		return errors.New("invalid push subscription")
	}
	return nil
}

// NormalizeContact returns a canonical mailto: or HTTPS contact. It rejects
// known local-only names; callers must still supply a real, reachable contact.
// This intentionally does not perform DNS lookups or prove mailbox ownership.
func NormalizeContact(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, "\r\n\t ") {
		return "", errors.New("web push requires a public contact email or HTTPS URL")
	}
	if strings.HasPrefix(value, "https:") {
		u, err := url.Parse(value)
		if err == nil && u.Scheme == "https" && u.User == nil && publicContactHost(u.Hostname()) {
			return value, nil
		}
	} else {
		address := strings.TrimPrefix(value, "mailto:")
		parsed, err := mail.ParseAddress(address)
		if err == nil && parsed.Address == address {
			_, domain, _ := strings.Cut(address, "@")
			if publicContactHost(domain) {
				return "mailto:" + address, nil
			}
		}
	}
	return "", errors.New("web push requires a public contact email or HTTPS URL")
}
func publicContactHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return false
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".test", ".invalid", ".example", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

type Config struct{ PublicKey, PrivateKey, Contact string }

func (c Config) Validate() error {
	if _, err := NormalizeContact(c.Contact); err != nil {
		return err
	}
	private, err := decodeKey(c.PrivateKey)
	if err != nil {
		return errors.New("invalid VAPID private key")
	}
	key, err := ecdh.P256().NewPrivateKey(private)
	if err != nil {
		return errors.New("invalid VAPID private key")
	}
	public, err := decodeKey(c.PublicKey)
	if err != nil || !bytes.Equal(public, key.PublicKey().Bytes()) {
		return errors.New("VAPID public and private keys do not match")
	}
	return nil
}
func decodeKey(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
}

// Options keeps TTL and urgency explicit app policy; TTL zero remains zero.
type Options struct {
	TTL        int
	Urgency    string
	HTTPClient *http.Client
}
type Result struct {
	StatusCode int
	Reason     string
}

func (r Result) Expired() bool {
	return r.StatusCode == http.StatusGone || r.StatusCode == http.StatusNotFound
}
func (r Result) Retryable() bool {
	return r.StatusCode == http.StatusTooManyRequests || r.StatusCode >= 500
}

// DeliveryError never includes subscription endpoints, keys, payloads, raw
// provider responses or transport error text, so it is safe to log.
type DeliveryError struct {
	Kind   string
	Result Result
}

func (e *DeliveryError) Error() string {
	if e.Result.StatusCode != 0 {
		return fmt.Sprintf("Web Push rejected: HTTP %d (%s)", e.Result.StatusCode, e.Result.Reason)
	}
	return "Web Push " + e.Kind
}

var defaultHTTPClient = &http.Client{Timeout: 15 * time.Second}

// Send performs one delivery. Apps decide concurrency, retry policy and removal
// of expired subscriptions using Result. No delivery is retried implicitly.
func Send(ctx context.Context, config Config, sub Subscription, payload []byte, options Options) (Result, error) {
	if err := config.Validate(); err != nil {
		return Result{}, err
	}
	if err := sub.Validate(); err != nil {
		return Result{}, err
	}
	if options.TTL < 0 {
		return Result{}, errors.New("web push TTL cannot be negative")
	}
	urgency := options.Urgency
	if urgency == "" {
		urgency = "normal"
	}
	switch urgency {
	case "very-low", "low", "normal", "high":
	default:
		return Result{}, errors.New("invalid Web Push urgency")
	}
	contact, _ := NormalizeContact(config.Contact)
	client := options.HTTPClient
	if client == nil {
		client = defaultHTTPClient
	}
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth}}, &webpush.Options{Subscriber: strings.TrimPrefix(contact, "mailto:"), VAPIDPublicKey: config.PublicKey, VAPIDPrivateKey: config.PrivateKey, TTL: options.TTL, Urgency: webpush.Urgency(urgency), HTTPClient: client})
	if err != nil {
		return Result{}, &DeliveryError{Kind: "transport failed"}
	}
	defer func() {
		_ = response.Body.Close()
	}()
	result := Result{StatusCode: response.StatusCode}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return result, nil
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&body)
	switch body.Reason {
	case "BadJwtToken", "VapidPkHashMismatch", "BadVapidPublicKey", "BadAuthorizationHeader", "BadTtl", "BadUrgency", "BadWebPushRequest", "BadWebPushTopic", "BadPath", "MethodNotAllowed", "PayloadTooLarge", "TooManyRequests", "InternalServerError", "ServiceUnavailable", "Shutdown", "IdleTimeout":
		result.Reason = body.Reason
	default:
		result.Reason = "Rejected"
	}
	return result, &DeliveryError{Kind: "rejected", Result: result}
}
