package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrWorkerQueueFull = errors.New("worker queue is full")

type Job func(context.Context) error

// WorkerPool bounds both goroutine count and queued work.
type WorkerPool struct {
	jobs   chan Job
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

func NewWorkerPool(parent context.Context, workers, queueSize int, onError func(error)) *WorkerPool {
	if workers < 1 {
		workers = 1
	}
	if queueSize < workers {
		queueSize = workers
	}
	ctx, cancel := context.WithCancel(parent)
	pool := &WorkerPool{jobs: make(chan Job, queueSize), cancel: cancel}
	for i := 0; i < workers; i++ {
		pool.wg.Add(1)
		go func() {
			defer pool.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-pool.jobs:
					if !ok {
						return
					}
					func() {
						defer func() {
							if recovered := recover(); recovered != nil && onError != nil {
								onError(fmt.Errorf("worker panic: %v", recovered))
							}
						}()
						if err := job(ctx); err != nil && onError != nil {
							onError(err)
						}
					}()
				}
			}
		}()
	}
	return pool
}

func (p *WorkerPool) Submit(job Job) error {
	select {
	case p.jobs <- job:
		return nil
	default:
		return ErrWorkerQueueFull
	}
}

func (p *WorkerPool) Close() {
	p.cancel()
	close(p.jobs)
	p.wg.Wait()
}
