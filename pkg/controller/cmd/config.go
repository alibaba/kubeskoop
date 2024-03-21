package cmd

import (
	"fmt"
	"os"

	"github.com/alibaba/kubeskoop/pkg/controller/k8s"

	"github.com/alibaba/kubeskoop/pkg/controller/service"
	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	AgentPort int `yaml:"agentPort"`
	HTTPPort  int `yaml:"httpPort"`
}

type Config struct {
	LogLevel   string         `yaml:"logLevel"`
	Kubernetes k8s.Config     `yaml:"k8s"`
	Server     ServerConfig   `yaml:"server"`
	Controller service.Config `yaml:"controller"`
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
