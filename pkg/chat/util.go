package chat

import (
	"encoding/json"
	"strings"
)

func DecodeToMessagePresenter(data []byte) (*MessagePresenter, error) {
	var msg MessagePresenter
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func DecodeToMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func splitEditPayload(payload string) []string {
	idx := strings.Index(payload, "|")
	if idx == -1 {
		return nil
	}
	return []string{payload[:idx], payload[idx+1:]}
}

func splitReactionPayload(payload string) []string {
	parts := strings.SplitN(payload, "|", 3)
	if len(parts) != 3 {
		return nil
	}
	return parts
}

func splitPinPayload(payload string) []string {
	parts := strings.SplitN(payload, "|", 2)
	if len(parts) != 2 {
		return nil
	}
	return parts
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
