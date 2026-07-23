package call

import "testing"

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from, to State
		ok       bool
	}{
		{StateRinging, StateAccepted, true},
		{StateRinging, StateRejected, true},
		{StateAccepted, StateConnected, true},
		{StateConnected, StateEnded, true},
		{StateRinging, StateConnected, false},
		{StateEnded, StateConnected, false},
	}
	for _, test := range tests {
		if got := validTransition(test.from, test.to); got != test.ok {
			t.Fatalf("%s -> %s: got %v, want %v", test.from, test.to, got, test.ok)
		}
	}
}
