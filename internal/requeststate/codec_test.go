package requeststate

import (
	"strings"
	"testing"
	"time"
)

func TestCodecRejectsTamperingAndExpiry(t *testing.T) {
	c := New("test-secret")
	token, err := c.Encode(State{Operation: "move_todo", TodoID: "a", TargetListID: "done", ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Decode(token); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Decode(token + "x"); err == nil {
		t.Fatal("tampering accepted")
	}
	expired, err := c.Encode(State{ExpiresAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Decode(expired); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("want expiry error, got %v", err)
	}
}
