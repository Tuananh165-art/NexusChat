package notification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gocql/gocql"
)

const (
	StatusPending    = "pending"
	StatusComplete   = "complete"
	StatusDead       = "dead_letter"
	StatusDeadLetter = StatusDead
)

var ErrInvalidIntent = errors.New("invalid notification intent")

type Intent struct {
	ID        string
	UserID    uint64
	Type      string
	ActorID   uint64
	Payload   string
	CreatedAt time.Time
}

type Enqueuer interface {
	Enqueue(context.Context, Intent) error
}

type Store struct {
	session *gocql.Session
	now     func() time.Time
}

func NewStore(session *gocql.Session) *Store {
	return &Store{session: session, now: time.Now}
}

func DeterministicID(namespace string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(namespace))
	for _, part := range parts {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) Enqueue(ctx context.Context, intent Intent) error {
	if err := validate(intent); err != nil {
		return err
	}
	if s == nil || s.session == nil {
		return errors.New("notification outbox: nil Cassandra session")
	}
	now := s.now().UTC()
	created := intent.CreatedAt.UTC()
	applied, err := s.session.Query(`INSERT INTO notification_outbox_by_id
		(notification_id, user_id, type, actor_id, payload, created_at, status, attempts, next_attempt_at, lease_owner, lease_until, last_error, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) IF NOT EXISTS`,
		intent.ID, intent.UserID, intent.Type, intent.ActorID, intent.Payload, created, StatusPending, 0, now, "", time.Unix(0, 0).UTC(), "", now).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("enqueue notification %q: %w", intent.ID, err)
	}
	if !applied {
		_, status, _, nextAttempt, _, getErr := s.get(ctx, intent.ID)
		if getErr != nil || status != StatusPending {
			return getErr
		}
		return s.insertIndex(ctx, intent, StatusPending, nextAttempt)
	}
	return s.insertIndex(ctx, intent, StatusPending, now)
}

func validate(intent Intent) error {
	if intent.ID == "" || intent.UserID == 0 || intent.Type == "" || intent.CreatedAt.IsZero() {
		return ErrInvalidIntent
	}
	return nil
}

func bucket(t time.Time) string { return t.UTC().Format("2006-01-02") }

func (s *Store) insertIndex(ctx context.Context, intent Intent, status string, dueAt time.Time) error {
	return s.session.Query(`INSERT INTO notification_outbox_by_bucket
		(bucket, status, due_at, notification_id, user_id, type, actor_id, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, bucket(dueAt), status, dueAt.UTC(), intent.ID, intent.UserID, intent.Type, intent.ActorID, intent.Payload, intent.CreatedAt.UTC()).WithContext(ctx).Exec()
}

func (s *Store) deleteIndex(ctx context.Context, id, status string, dueAt time.Time) error {
	return s.session.Query(`DELETE FROM notification_outbox_by_bucket WHERE bucket = ? AND status = ? AND due_at = ? AND notification_id = ?`, bucket(dueAt), status, dueAt.UTC(), id).WithContext(ctx).Exec()
}

func retryDelay(base, max time.Duration, attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if base <= 0 {
		base = time.Second
	}
	if max <= 0 {
		max = 5 * time.Minute
	}
	delay := base
	for i := 1; i < attempts && delay < max; i++ {
		if delay > max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}

func (s *Store) get(ctx context.Context, id string) (Intent, string, int, time.Time, string, error) {
	var intent Intent
	var status, leaseOwner, lastError string
	var attempts int
	var nextAttempt time.Time
	err := s.session.Query(`SELECT user_id, type, actor_id, payload, created_at, status, attempts, next_attempt_at, lease_owner, last_error FROM notification_outbox_by_id WHERE notification_id = ?`, id).WithContext(ctx).Scan(&intent.UserID, &intent.Type, &intent.ActorID, &intent.Payload, &intent.CreatedAt, &status, &attempts, &nextAttempt, &leaseOwner, &lastError)
	if err != nil {
		return Intent{}, "", 0, time.Time{}, "", err
	}
	intent.ID = id
	return intent, status, attempts, nextAttempt, leaseOwner, nil
}

// ClaimDue returns durable intents whose leases can be claimed by owner.
func (s *Store) ClaimDue(ctx context.Context, owner string, limit int, lease time.Duration) ([]Intent, error) {
	if s == nil || s.session == nil {
		return nil, errors.New("notification outbox: nil Cassandra session")
	}
	if limit <= 0 {
		limit = 50
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := s.now().UTC()
	result := make([]Intent, 0, limit)
	for days := 0; days <= 7 && len(result) < limit; days++ {
		day := now.AddDate(0, 0, -days)
		iter := s.session.Query(`SELECT due_at, notification_id FROM notification_outbox_by_bucket WHERE bucket = ? AND status = ? AND due_at <= ? LIMIT ?`, bucket(day), StatusPending, now, limit-len(result)).WithContext(ctx).Iter()
		var dueAt time.Time
		var id string
		for iter.Scan(&dueAt, &id) {
			intent, status, attempts, nextAttempt, _, err := s.get(ctx, id)
			if err != nil && err != gocql.ErrNotFound {
				_ = iter.Close()
				return nil, err
			}
			if err != nil || status != StatusPending || nextAttempt.After(now) {
				continue
			}
			applied, err := s.session.Query(`UPDATE notification_outbox_by_id SET lease_owner = ?, lease_until = ?, updated_at = ? WHERE notification_id = ? IF status = ? AND lease_until < ?`, owner, now.Add(lease), now, id, StatusPending, now).WithContext(ctx).MapScanCAS(map[string]interface{}{})
			if err != nil {
				_ = iter.Close()
				return nil, err
			}
			if applied {
				_ = attempts
				result = append(result, intent)
			}
		}
		if err := iter.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Store) Complete(ctx context.Context, id, owner string) error {
	now := s.now().UTC()
	applied, err := s.session.Query(`UPDATE notification_outbox_by_id SET status = ?, lease_owner = '', lease_until = ?, updated_at = ? WHERE notification_id = ? IF status = ? AND lease_owner = ?`, StatusComplete, time.Unix(0, 0).UTC(), now, id, StatusPending, owner).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err != nil {
		return err
	}
	if !applied {
		return nil
	}
	return nil
}

func (s *Store) Retry(ctx context.Context, intent Intent, owner string, attempts int, cause error, maxAttempts int, baseDelay, maxDelay time.Duration) error {
	if attempts >= maxAttempts {
		_, err := s.session.Query(`UPDATE notification_outbox_by_id SET status = ?, last_error = ?, lease_owner = '', lease_until = ?, updated_at = ? WHERE notification_id = ? IF status = ? AND lease_owner = ?`, StatusDead, errorString(cause), time.Unix(0, 0).UTC(), s.now().UTC(), intent.ID, StatusPending, owner).WithContext(ctx).MapScanCAS(map[string]interface{}{})
		return err
	}
	dueAt := s.now().UTC().Add(retryDelay(baseDelay, maxDelay, attempts))
	if err := s.insertIndex(ctx, intent, StatusPending, dueAt); err != nil {
		return err
	}
	_, err := s.session.Query(`UPDATE notification_outbox_by_id SET attempts = ?, next_attempt_at = ?, last_error = ?, lease_owner = '', lease_until = ?, updated_at = ? WHERE notification_id = ? IF status = ? AND lease_owner = ?`, attempts, dueAt, errorString(cause), time.Unix(0, 0).UTC(), s.now().UTC(), intent.ID, StatusPending, owner).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	return err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Store) Status(ctx context.Context, id string) (string, int, error) {
	_, status, attempts, _, _, err := s.get(ctx, id)
	return status, attempts, err
}

func FormatUserID(id uint64) string { return strconv.FormatUint(id, 10) }
