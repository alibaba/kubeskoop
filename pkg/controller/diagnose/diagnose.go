package diagnose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
)

type Config struct {
	KubeConfig    string   `yaml:"kubeConfig"`
	CloudProvider string   `yaml:"cloudProvider"`
	NetworkPlugin string   `yaml:"networkPlugin"`
	ProxyModel    string   `yaml:"proxyModel"`
	ClusterCidr   string   `yaml:"clusterCidr"`
	ExtraArgs     []string `yaml:"extraArgs"`
}

type Controller interface {
	Diagnose(ctx context.Context, taskConfig *skoopContext.TaskConfig) (string, error)
}

func NewDiagnoseController(namespace string, config *Config) Controller {
	// 1. build skoop global context
	return &Diagnostor{
		namespace: namespace,
		config:    config,
	}
}

type Diagnostor struct {
	namespace string
	config    *Config
}

func (d *Diagnostor) Diagnose(ctx context.Context, taskConfig *skoopContext.TaskConfig) (string, error) {
	tempDir, err := os.MkdirTemp("/tmp", "skoop")
	if err != nil {
		return "", fmt.Errorf("failed create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	resultStorage := fmt.Sprintf("%v/result.json", tempDir)
	args := []string{"--output", resultStorage, "--format", "json",
		"-s", taskConfig.Source,
		"-d", taskConfig.Destination.Address,
		"--dport", strconv.FormatUint(uint64(taskConfig.Destination.Port), 10),
		"--protocol", taskConfig.Protocol,
		"--collector-namespace", d.namespace,
	}
	if d.config != nil {
		args = append(args, buildArgsFromConfig(d.config)...)
	}
	cmd := exec.CommandContext(ctx, "skoop", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to diagnose: %v, output: %v", err, string(output))
	}
	diagnoseResult, err := os.ReadFile(resultStorage)
	if err != nil {
		return "", fmt.Errorf("failed to read diagnose result: %v", err)
	}
	return string(diagnoseResult), nil
}

func buildArgsFromConfig(config *Config) []string {
	var args []string
	m := map[string]string{
		"--cloud-provider": config.CloudProvider,
		"--network-plugin": config.NetworkPlugin,
		"--proxy-mode":     config.ProxyModel,
		"--cluster-cidr":   config.ClusterCidr,
	}

	for k, v := range m {
		if v != "" {
			args = append(args, k, v)
		}
	}
	return append(args, config.ExtraArgs...)
}
