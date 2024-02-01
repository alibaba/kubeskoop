package cmd

import (
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/db"
	"github.com/alibaba/kubeskoop/pkg/controller/diagnose"
	"gopkg.in/yaml.v3"
	"os"
)

type ServerConfig struct {
	AgentPort  int    `yaml:"agentPort"`
	HttpPort   int    `yaml:"httpPort"`
	KubeConfig string `yaml:"kubeConfig"`
}

type Config struct {
	LogLevel string          `yaml:"logLevel"`
	Server   ServerConfig    `yaml:"server"`
	DB       db.Config       `yaml:"database"`
	Diagnose diagnose.Config `yaml:"diagnose"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed read config file %s: %w", path, err)
	}

	config := Config{}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed parse config file: %s: %w", path, err)
	}

	return &config, nil
}
