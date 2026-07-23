package notification

import "testing"

func TestTruncatePreservesUnicode(t *testing.T) {
	if got := truncate("Xin chào NexusChat", 8); got != "Xin chào…" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("short", 20); got != "short" {
		t.Fatalf("unexpected truncation: %q", got)
	}
}
