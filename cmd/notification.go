package cmd

import (
	"github.com/Tuananh165-art/NexusChat/pkg/notification"
	"github.com/spf13/cobra"
)

var notificationCmd = &cobra.Command{
	Use:   "notification",
	Short: "notification inbox and web push service",
	Run:   func(*cobra.Command, []string) { notification.Main() },
}

func init() { rootCmd.AddCommand(notificationCmd) }
