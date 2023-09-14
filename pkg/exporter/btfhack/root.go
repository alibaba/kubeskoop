package btfhack

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	defaultBTFPath = "/etc/btf"
)

var rootCmd = &cobra.Command{
	Use:   "btfhack",
	Short: "A tool to automatically discover btf file from local path or online",
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
