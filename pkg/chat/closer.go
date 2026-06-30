package chat

import (
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
)

type InfraCloser struct{}

func NewInfraCloser() *InfraCloser {
	return &InfraCloser{}
}

func (closer *InfraCloser) Close() error {
	if err := ForwarderConn.Conn.Close(); err != nil {
		return err
	}
	if err := UserConn.Conn.Close(); err != nil {
		return err
	}
	infra.CassandraSession.Close()
	return infra.RedisClient.Close()
}
