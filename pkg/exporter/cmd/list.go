package cmd

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var (
	listCmd = &cobra.Command{
		Use:   "list",
		Short: "list available options",
		Run: func(cmd *cobra.Command, _ []string) {
			_ = cmd.Help() // nolint
		},
	}
)

func init() {
	rootCmd.AddCommand(listCmd)
}
