package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gocql/gocql"
)

func PublishDurably(ctx context.Context, session *gocql.Session, bus *EventBus, service, topic string, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := session.Query(
		"INSERT INTO outbox_events (service, bucket, event_id, topic, payload, published, created_at) VALUES (?, ?, ?, ?, ?, false, ?)",
		service, now.Truncate(24*time.Hour), event.EventID, topic, string(body), now,
	).WithContext(ctx).Exec(); err != nil {
		return err
	}
	if err := bus.Publish(ctx, topic, event); err != nil {
		return err
	}
	return session.Query(
		"UPDATE outbox_events SET published = true WHERE service = ? AND bucket = ? AND created_at = ? AND event_id = ?",
		service, now.Truncate(24*time.Hour), now, event.EventID,
	).WithContext(ctx).Exec()
}

func RelayOutbox(ctx context.Context, session *gocql.Session, bus *EventBus, service string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bucket := time.Now().UTC().Truncate(24 * time.Hour)
			iter := session.Query(
				"SELECT created_at, event_id, topic, payload FROM outbox_events WHERE service = ? AND bucket = ? AND published = false ALLOW FILTERING",
				service, bucket,
			).WithContext(ctx).Iter()
			var createdAt time.Time
			var eventID, topic, payload string
			for iter.Scan(&createdAt, &eventID, &topic, &payload) {
				var event Event
				if err := json.Unmarshal([]byte(payload), &event); err != nil {
					slog.Error("invalid outbox payload", "service", service, "event_id", eventID, "error", err)
					continue
				}
				if err := bus.Publish(ctx, topic, event); err != nil {
					slog.Error("outbox publish failed", "service", service, "event_id", eventID, "error", err)
					continue
				}
				if err := session.Query(
					"UPDATE outbox_events SET published = true WHERE service = ? AND bucket = ? AND created_at = ? AND event_id = ?",
					service, bucket, createdAt, eventID,
				).WithContext(ctx).Exec(); err != nil {
					slog.Error("outbox mark published failed", "error", err)
				}
			}
			if err := iter.Close(); err != nil {
				slog.Error("outbox scan failed", "service", service, "error", fmt.Errorf("%w", err))
			}
		}
	}
}
