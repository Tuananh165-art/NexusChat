package user

import (
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
	"github.com/gocql/gocql"
)

type InfraCloser struct {
	cassandra *gocql.Session
}

func NewInfraCloser(cassandra *gocql.Session) *InfraCloser {
	return &InfraCloser{cassandra: cassandra}
}

func (closer *InfraCloser) Close() error {
	if closer.cassandra != nil {
		closer.cassandra.Close()
	}
	return infra.RedisClient.Close()
}
