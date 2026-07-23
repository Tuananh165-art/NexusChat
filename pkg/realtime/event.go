package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
)

const (
	ChatEventsTopic         = "nexuschat.chat.events.v1"
	PresenceEventsTopic     = "nexuschat.presence.events.v1"
	CallEventsTopic         = "nexuschat.call.events.v1"
	NotificationEventsTopic = "nexuschat.notification.events.v1"
)

type Event struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	SchemaVersion int             `json:"schema_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	AggregateID   string          `json:"aggregate_id"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	CausationID   string          `json:"causation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

func NewEvent(eventType, producer, aggregateID string, payload any) (Event, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC(),
		Producer:      producer,
		AggregateID:   aggregateID,
		Payload:       body,
	}, nil
}

type EventBus struct {
	producer sarama.SyncProducer
	brokers  []string
	version  sarama.KafkaVersion
	mu       sync.Mutex
	groups   []sarama.ConsumerGroup
}

func NewEventBus(brokers []string, version string) (*EventBus, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 5
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	parsed, err := sarama.ParseKafkaVersion(version)
	if err != nil {
		return nil, fmt.Errorf("parse Kafka version: %w", err)
	}
	cfg.Version = parsed
	producer, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	return &EventBus{producer: producer, brokers: brokers, version: parsed}, nil
}

func (b *EventBus) Publish(ctx context.Context, topic string, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	message := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(event.AggregateID),
		Value: sarama.ByteEncoder(body),
	}
	_, _, err = b.producer.SendMessage(message)
	return err
}

type EventHandler func(context.Context, Event) error

func (b *EventBus) Consume(ctx context.Context, groupID string, topics []string, workers int, handler EventHandler) error {
	if workers < 1 {
		workers = 1
	}
	cfg := sarama.NewConfig()
	cfg.Version = b.version
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	group, err := sarama.NewConsumerGroup(b.brokers, groupID, cfg)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.groups = append(b.groups, group)
	b.mu.Unlock()

	consumer := &consumerGroupHandler{
		workers: workers,
		handler: handler,
	}
	for {
		if err := group.Consume(ctx, topics, consumer); err != nil {
			if ctx.Err() != nil || errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			slog.Error("Kafka consumer cycle failed", "group", groupID, "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	}
}

func (b *EventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var joined error
	for _, group := range b.groups {
		joined = errors.Join(joined, group.Close())
	}
	if b.producer != nil {
		joined = errors.Join(joined, b.producer.Close())
	}
	return joined
}

type consumerGroupHandler struct {
	workers int
	handler EventHandler
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	jobs := make(chan *sarama.ConsumerMessage, h.workers*2)
	var wg sync.WaitGroup
	for i := 0; i < h.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for msg := range jobs {
				var event Event
				if err := json.Unmarshal(msg.Value, &event); err != nil {
					slog.Error("discarding invalid event", "topic", msg.Topic, "error", err)
					session.MarkMessage(msg, "invalid-json")
					continue
				}
				if err := h.handler(session.Context(), event); err != nil {
					slog.Error("event handler failed", "event_id", event.EventID, "type", event.EventType, "error", err)
					continue
				}
				session.MarkMessage(msg, "")
			}
		}()
	}
	for {
		select {
		case <-session.Context().Done():
			close(jobs)
			wg.Wait()
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				close(jobs)
				wg.Wait()
				return nil
			}
			jobs <- msg
		}
	}
}
