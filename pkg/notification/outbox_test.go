package notification

import (
	"testing"
	"time"
)

func TestDeterministicID(t *testing.T) {
	first := DeterministicID("friend-request", "12", "34")
	if first == "" || first != DeterministicID("friend-request", "12", "34") {
		t.Fatal("deterministic ID changed for identical input")
	}
	if first == DeterministicID("friend-request", "12", "35") {
		t.Fatal("different input produced the same ID")
	}
}

func TestRetryDelayExponentialAndCapped(t *testing.T) {
	if got := retryDelay(time.Second, 10*time.Second, 3); got != 4*time.Second {
		t.Fatalf("attempt 3 delay = %s, want 4s", got)
	}
	if got := retryDelay(time.Second, 10*time.Second, 8); got != 10*time.Second {
		t.Fatalf("capped delay = %s, want 10s", got)
	}
}
