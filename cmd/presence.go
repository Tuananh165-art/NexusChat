package cmd

import (
	"github.com/Tuananh165-art/NexusChat/pkg/presence"
	"github.com/spf13/cobra"
)

var presenceCmd = &cobra.Command{
	Use:   "presence",
	Short: "presence and device heartbeat service",
	Run:   func(*cobra.Command, []string) { presence.Main() },
}

func init() { rootCmd.AddCommand(presenceCmd) }
