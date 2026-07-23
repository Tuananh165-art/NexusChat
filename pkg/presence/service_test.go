package presence

import (
	"testing"

	"github.com/Tuananh165-art/NexusChat/pkg/config"
)

func TestActiveTTL(t *testing.T) {
	if got := activeTTL(&config.PresenceConfig{TTLSecond: 45}); got != 180 {
		t.Fatalf("got %d, want 180", got)
	}
	if got := activeTTL(&config.PresenceConfig{TTLSecond: 10}); got != 120 {
		t.Fatalf("got %d, want 120", got)
	}
}
