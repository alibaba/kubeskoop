package cmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// rootCmd represents the base command when called without any subcommands
var (
	rootCmd = &cobra.Command{
		Use:   "inspector",
		Short: "network inspection tool",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if debug {
				opts := slog.HandlerOptions{
					AddSource: true,
					Level:     slog.DebugLevel,
				}

				slog.SetDefault(slog.New(opts.NewJSONHandler(os.Stderr)))
			} else {
				slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard)))
			}
		},
	}

	debug   bool
	verbose bool
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug log information")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable detail information")
}
