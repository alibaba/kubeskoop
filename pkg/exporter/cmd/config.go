package cmd

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type InspServerConfig struct {
	DebugMode        bool          `yaml:"debugMode" mapstructure:"debugMode" json:"debugMode"`
	Port             uint16        `yaml:"port" mapstructure:"port" json:"port"`
	EnableController bool          `yaml:"enableController" mapstructure:"enableController" json:"enableController"`
	MetricsConfig    MetricsConfig `yaml:"metrics" mapstructure:"metrics" json:"metrics"`
	EventConfig      EventConfig   `yaml:"event" mapstructure:"event" json:"event"`
}

type MetricsConfig struct {
	Probes           []ProbeConfig `yaml:"probes" mapstructure:"probes" json:"probes"`
	AdditionalLabels string        `yaml:"additionalLabels" mapstructure:"additionalLabels" json:"additionalLabels"`
}

type EventConfig struct {
	EventSinks []EventSinkConfig `yaml:"sinks" mapstructure:"sinks" json:"sinks"`
	Probes     []ProbeConfig     `yaml:"probes" mapstructure:"probes" json:"probes"`
}

type EventSinkConfig struct {
	Name string      `yaml:"name" mapstructure:"name" json:"name"`
	Args interface{} `yaml:"args" mapstructure:"args" json:"args"`
}

type ProbeConfig struct {
	Name string                 `yaml:"name" mapstructure:"name" json:"name"`
	Args map[string]interface{} `yaml:"args" mapstructure:"args" json:"args"`
}

func loadConfig(path string) (*InspServerConfig, error) {
	cfg := InspServerConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed read config file %s: %w", path, err)
	}

	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed parse config file %s: %w", path, err)
	}

	return &cfg, nil

}
