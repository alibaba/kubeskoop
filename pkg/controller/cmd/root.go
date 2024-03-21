package cmd

import (
	"fmt"
	"os"

	"github.com/alibaba/kubeskoop/pkg/controller/k8s"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var (
	rootCmd = &cobra.Command{
		Use:   "skoop-controller",
		Short: "skoop centralized controller",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &Config{}
			var err error
			if configPath != "" {
				config, err = loadConfig(configPath)
				if err != nil {
					return err
				}
			}

			if err := mergeConfig(config); err != nil {
				return err
			}

			initK8S(&config.Kubernetes)
			NewServer(config).Run()
			return nil
		},
	}

	debug      bool
	agentPort  int
	httpPort   int
	configPath string
)

func initK8S(config *k8s.Config) {
	if err := k8s.InitKubernetesClient(config); err != nil {
		panic(fmt.Sprintf("failed create k8s client: %v", err))
	}

	if err := k8s.InitInformer(k8s.Client); err != nil {
		panic(fmt.Errorf("failed init informer: %v", err))
	}

}

func mergeConfig(config *Config) error {
	if debug {
		config.LogLevel = "debug"
	}
	logLevel, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %s", config.LogLevel)
	}
	log.SetLevel(logLevel)

	if config.Server.AgentPort == 0 {
		config.Server.AgentPort = defaultAgentPort
	}
	if agentPort > 0 {
		config.Server.AgentPort = agentPort
	}
	if config.Server.HTTPPort == 0 {
		config.Server.HTTPPort = defaultHTTPPort
	}
	if httpPort > 0 {
		config.Server.HTTPPort = httpPort
	}

	config.Controller.KubeConfig = config.Kubernetes.KubeConfig

	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "/etc/kubeskoop/controller.yaml", "Config file path for kubeskoop controller")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug log information")
	rootCmd.PersistentFlags().IntVarP(&agentPort, "agent-port", "a", -1, "Controller port for agent registration")
	rootCmd.PersistentFlags().IntVarP(&httpPort, "http-port", "p", -1, "Controller port for http access")
}
