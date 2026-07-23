package cmd

import (
	"github.com/Tuananh165-art/NexusChat/pkg/call"
	"github.com/spf13/cobra"
)

var callCmd = &cobra.Command{
	Use:   "call",
	Short: "WebRTC call signaling service",
	Run:   func(*cobra.Command, []string) { call.Main() },
}

func init() { rootCmd.AddCommand(callCmd) }
