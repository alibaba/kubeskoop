package cmd

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var (
	listCmd = &cobra.Command{
		Use:   "list",
		Short: "list available options",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help() // nolint
		},
	}

	output string
)

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.PersistentFlags().StringVarP(&output, "output", "o", "text", "output format, support text/json/file")
}
