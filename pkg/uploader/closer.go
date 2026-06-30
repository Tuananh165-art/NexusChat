package uploader

import (
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
)

type InfraCloser struct{}

func NewInfraCloser() *InfraCloser {
	return &InfraCloser{}
}

func (closer *InfraCloser) Close() error {
	return infra.RedisClient.Close()
}
