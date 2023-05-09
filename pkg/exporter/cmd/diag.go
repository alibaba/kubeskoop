package cmd

import (
	"github.com/spf13/cobra"
)

// diagCmd represents the diag command
var (
	diagCmd = &cobra.Command{
		Use:   "diag",
		Short: "Run command in the command line to probe metrics and events.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help() // nolint
		},
	}

	podname string
)

func init() {
	rootCmd.AddCommand(diagCmd)
	diagCmd.PersistentFlags().StringVarP(&podname, "pod", "i", "", "specified pod")
}
