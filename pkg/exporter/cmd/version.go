package cmd

import (
	"github.com/alibaba/kubeskoop/version"
	"github.com/spf13/cobra"
)

var (
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "show version",
		Run: func(_ *cobra.Command, _ []string) {
			version.PrintVersion()
		},
	}
)

func init() {
	rootCmd.AddCommand(versionCmd)
}
