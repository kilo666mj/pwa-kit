package pwakit

import (
	"errors"
	"net/url"
	"strings"
)

// Notification describes a visible notification without requiring worker execution.
// Navigate must be an absolute HTTP(S) URL; applications enforce their own origin.
// AppBadge is a decimal string on supporting Apple platforms.
type Notification struct {
	Title              string `json:"title"`
	Body               string `json:"body,omitempty"`
	Navigate           string `json:"navigate"`
	Tag                string `json:"tag,omitempty"`
	Icon               string `json:"icon,omitempty"`
	Badge              string `json:"badge,omitempty"`
	Timestamp          int64  `json:"timestamp,omitempty"`
	Renotify           bool   `json:"renotify,omitempty"`
	RequireInteraction bool   `json:"requireInteraction,omitempty"`
	AppBadge           string `json:"app_badge,omitempty"`
}

// DeclarativePush defaults to immutable delivery: supporting browsers display it
// without running JavaScript. Older browsers use installPushHandlers instead.
// Embed this in an application's payload to retain legacy fields during upgrades.
type DeclarativePush struct {
	WebPush      int          `json:"web_push"`
	Notification Notification `json:"notification"`
}

// NewDeclarativePush validates the required presentation and navigation fields.
// Applications may embed the result alongside their legacy payload fields.
func NewDeclarativePush(notification Notification) (DeclarativePush, error) {
	u, err := url.Parse(notification.Navigate)
	if strings.TrimSpace(notification.Title) == "" || err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || notification.Timestamp < 0 {
		return DeclarativePush{}, errors.New("notification requires a title, absolute HTTP(S) navigation URL and nonnegative timestamp")
	}
	if notification.Renotify && notification.Tag == "" {
		return DeclarativePush{}, errors.New("renotify requires a notification tag")
	}
	return DeclarativePush{WebPush: 8030, Notification: notification}, nil
}
