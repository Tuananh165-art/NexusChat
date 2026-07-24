package cmd

import (
	"github.com/Tuananh165-art/NexusChat/pkg/workspace"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "chat tasks, notes, bookmarks and reminders service",
	Run:   func(*cobra.Command, []string) { workspace.Main() },
}

func init() { rootCmd.AddCommand(workspaceCmd) }
