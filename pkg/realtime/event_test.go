package realtime

import (
	"encoding/json"
	"testing"
)

func TestEventEnvelope(t *testing.T) {
	event, err := NewEvent("safety.report.created", "safety-service", "report-1", map[string]string{"report_id": "report-1"})
	if err != nil {
		t.Fatal(err)
	}
	if event.EventID == "" || event.SchemaVersion != 1 || event.EventType != "safety.report.created" {
		t.Fatalf("invalid event: %+v", event)
	}
	var payload map[string]string
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["report_id"] != "report-1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
