package realtime

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestWorkerPoolRunsBoundedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var count atomic.Int32
	done := make(chan struct{}, 4)
	pool := NewWorkerPool(ctx, 2, 4, func(error) { t.Fail() })
	for i := 0; i < 4; i++ {
		if err := pool.Submit(func(context.Context) error {
			count.Add(1)
			done <- struct{}{}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	pool.Close()
	if count.Load() != 4 {
		t.Fatalf("expected four jobs, got %d", count.Load())
	}
}
