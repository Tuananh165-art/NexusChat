package cmd

import (
	"github.com/Tuananh165-art/NexusChat/pkg/discovery"
	"github.com/spf13/cobra"
)

var discoveryCmd = &cobra.Command{
	Use:   "discovery",
	Short: "interest-aware matching and discovery service",
	Run:   func(*cobra.Command, []string) { discovery.Main() },
}

func init() { rootCmd.AddCommand(discoveryCmd) }
