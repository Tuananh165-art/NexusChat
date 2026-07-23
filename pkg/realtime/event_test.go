package realtime

import (
	"encoding/json"
	"testing"
)

func TestEventEnvelope(t *testing.T) {
	event, err := NewEvent("call.ringing", "call-service", "call-1", map[string]string{"call_id": "call-1"})
	if err != nil {
		t.Fatal(err)
	}
	if event.EventID == "" || event.SchemaVersion != 1 || event.EventType != "call.ringing" {
		t.Fatalf("invalid event: %+v", event)
	}
	var payload map[string]string
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["call_id"] != "call-1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
