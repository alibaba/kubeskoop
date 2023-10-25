package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

// rootCmd represents the base command when called without any subcommands
var (
	rootCmd = &cobra.Command{
		Use:   "skoop-controller",
		Short: "skoop centralized controller",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if debug {
				log.SetLevel(log.DebugLevel)
			} else {
				log.SetLevel(log.InfoLevel)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			NewServer().Run(agentPort, httpPort)
		},
	}

	debug     bool
	agentPort int
	httpPort  int
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
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug log information")
	rootCmd.PersistentFlags().IntVarP(&agentPort, "agent-port", "a", defaultAgentPort, "Controller Port For Agent Registration")
	rootCmd.PersistentFlags().IntVarP(&httpPort, "http-port", "p", defaultHttpPort, "Controller Port For Agent Registration")
}
