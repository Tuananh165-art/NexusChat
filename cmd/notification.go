package cmd

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tuananh165-art/NexusChat/pkg/config"
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
	"github.com/Tuananh165-art/NexusChat/pkg/notification"
	"github.com/spf13/cobra"
)

var notificationCmd = &cobra.Command{
	Use:   "notification",
	Short: "notification outbox worker",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.NewConfig()
		if err != nil {
			slog.Error("load config", "error", err)
			return
		}
		session, err := infra.NewCassandraSession(cfg)
		if err != nil {
			slog.Error("connect Cassandra", "error", err)
			return
		}
		defer session.Close()
		store := notification.NewStore(session)
		workerCfg := notification.DefaultConfig()
		if cfg.Notification != nil {
			workerCfg.PollInterval = time.Duration(cfg.Notification.PollIntervalSecond) * time.Second
			workerCfg.LeaseDuration = time.Duration(cfg.Notification.LeaseSecond) * time.Second
			workerCfg.MaxAttempts = cfg.Notification.MaxAttempts
			workerCfg.BaseRetryDelay = time.Duration(cfg.Notification.BaseRetrySecond) * time.Second
			workerCfg.MaxRetryDelay = time.Duration(cfg.Notification.MaxRetrySecond) * time.Second
			workerCfg.BatchSize = cfg.Notification.BatchSize
			workerCfg.WorkerID = cfg.Notification.WorkerID
		}
		worker := notification.NewWorker(store, notification.NewCassandraDeliverer(session), workerCfg)
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("notification worker stopped", "error", err)
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = worker.Close(closeCtx)
	},
}

func init() { rootCmd.AddCommand(notificationCmd) }
