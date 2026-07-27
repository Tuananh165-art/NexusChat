package notification

import (
	"context"
	"errors"

	"github.com/gocql/gocql"
)

type Deliverer interface {
	Deliver(context.Context, Intent) error
}

type CassandraDeliverer struct{ session *gocql.Session }

func NewCassandraDeliverer(session *gocql.Session) *CassandraDeliverer {
	return &CassandraDeliverer{session: session}
}

func (d *CassandraDeliverer) Deliver(ctx context.Context, intent Intent) error {
	if d == nil || d.session == nil {
		return errors.New("notification delivery: nil Cassandra session")
	}
	return d.session.Query(`INSERT INTO notifications_by_user
		(user_id, created_at, notification_id, type, actor_id, payload, read)
		VALUES (?, ?, ?, ?, ?, ?, false)`, intent.UserID, intent.CreatedAt.UTC(), intent.ID, intent.Type, intent.ActorID, intent.Payload).WithContext(ctx).Exec()
}
