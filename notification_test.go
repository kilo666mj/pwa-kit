package pwakit

import (
	"encoding/json"
	"testing"
)

func TestDeclarativePush(t *testing.T) {
	n := Notification{Title: "Backup complete", Body: "Done", Navigate: "https://app.example.com/?notification=123", Tag: "123", Renotify: true, RequireInteraction: true, Timestamp: 1234, AppBadge: "1"}
	p, err := NewDeclarativePush(n)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	notification := value["notification"].(map[string]any)
	if value["web_push"] != float64(8030) || notification["navigate"] != n.Navigate || notification["requireInteraction"] != true || notification["app_badge"] != "1" {
		t.Fatalf("bad envelope: %s", raw)
	}
	if _, exists := value["mutable"]; exists {
		t.Fatal("must not require JavaScript")
	}
	for _, navigate := range []string{"/relative", "javascript:alert(1)", "https://user:password@app.example.com/", "//app.example.com/"} {
		n.Navigate = navigate
		if _, err := NewDeclarativePush(n); err == nil {
			t.Fatalf("accepted %q", navigate)
		}
	}
	n.Navigate = "https://app.example.com/"
	n.Tag = ""
	if _, err := NewDeclarativePush(n); err == nil {
		t.Fatal("accepted renotify without tag")
	}
	n.Tag = "123"
	n.Title = " "
	if _, err := NewDeclarativePush(n); err == nil {
		t.Fatal("accepted empty title")
	}
}
