package diagnose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
)

type Controller interface {
	Diagnose(ctx context.Context, taskConfig *skoopContext.TaskConfig) (string, error)
}

func NewDiagnoseController() Controller {
	// 1. build skoop global context
	return &diagnoser{}
}

type diagnoser struct {
}

func (d *diagnoser) Diagnose(ctx context.Context, taskConfig *skoopContext.TaskConfig) (string, error) {
	tempDir, err := os.MkdirTemp("/tmp", "skoop")
	if err != nil {
		return "", fmt.Errorf("failed create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	resultStorage := fmt.Sprintf("%v/result.json", tempDir)
	cmd := exec.CommandContext(ctx, "skoop", "--output", resultStorage, "--format", "json",
		"-s", taskConfig.Source,
		"-d", taskConfig.Destination.Address,
		"--dport", strconv.FormatUint(uint64(taskConfig.Destination.Port), 10))
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
