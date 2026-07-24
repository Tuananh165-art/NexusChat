package cmd

import (
	"github.com/Tuananh165-art/NexusChat/pkg/safety"
	"github.com/spf13/cobra"
)

var safetyCmd = &cobra.Command{
	Use:   "safety",
	Short: "trust and safety moderation service",
	Run:   func(*cobra.Command, []string) { safety.Main() },
}

func init() { rootCmd.AddCommand(safetyCmd) }
