package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Config struct {
	PollInterval   time.Duration
	LeaseDuration  time.Duration
	MaxAttempts    int
	BaseRetryDelay time.Duration
	MaxRetryDelay  time.Duration
	BatchSize      int
	WorkerID       string
}

func DefaultConfig() Config {
	return Config{PollInterval: 2 * time.Second, LeaseDuration: 30 * time.Second, MaxAttempts: 8, BaseRetryDelay: time.Second, MaxRetryDelay: 15 * time.Minute, BatchSize: 50}
}

type Worker struct {
	store     *Store
	deliverer Deliverer
	config    Config
	cancel    context.CancelFunc
	done      chan struct{}
	mu        sync.Mutex
	running   bool
}

func NewWorker(store *Store, deliverer Deliverer, config Config) *Worker {
	defaults := DefaultConfig()
	if config.PollInterval <= 0 {
		config.PollInterval = defaults.PollInterval
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = defaults.LeaseDuration
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = defaults.MaxAttempts
	}
	if config.BaseRetryDelay <= 0 {
		config.BaseRetryDelay = defaults.BaseRetryDelay
	}
	if config.MaxRetryDelay <= 0 {
		config.MaxRetryDelay = defaults.MaxRetryDelay
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaults.BatchSize
	}
	if config.WorkerID == "" {
		config.WorkerID = fmt.Sprintf("notification-%d", time.Now().UnixNano())
	}
	return &Worker{store: store, deliverer: deliverer, config: config}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.store == nil || w.deliverer == nil {
		return errors.New("notification worker: incomplete configuration")
	}
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return errors.New("notification worker already running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel, w.done, w.running = cancel, make(chan struct{}), true
	done := w.done
	w.mu.Unlock()
	defer func() { w.mu.Lock(); w.running = false; close(done); w.mu.Unlock() }()
	w.process(runCtx)
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-runCtx.Done():
			return nil
		case <-ticker.C:
			w.process(runCtx)
		}
	}
}

func (w *Worker) process(ctx context.Context) {
	intents, err := w.store.ClaimDue(ctx, w.config.WorkerID, w.config.BatchSize, w.config.LeaseDuration)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("claim notification intents", "error", err)
		}
		return
	}
	for _, intent := range intents {
		if err := w.deliverer.Deliver(ctx, intent); err != nil {
			statusErr := w.store.Retry(ctx, intent, w.config.WorkerID, w.attempts(ctx, intent.ID)+1, err, w.config.MaxAttempts, w.config.BaseRetryDelay, w.config.MaxRetryDelay)
			if statusErr != nil {
				slog.Error("record notification retry", "id", intent.ID, "error", statusErr)
			}
			continue
		}
		if err := w.store.Complete(ctx, intent.ID, w.config.WorkerID); err != nil {
			slog.Error("complete notification intent", "id", intent.ID, "error", err)
		}
	}
}

func (w *Worker) attempts(ctx context.Context, id string) int {
	_, attempts, err := w.store.Status(ctx, id)
	if err != nil {
		return 0
	}
	return attempts
}

func (w *Worker) Close(ctx context.Context) error {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
