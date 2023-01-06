package framework

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/alibaba/kubeskoop/pkg/skoop/ui"

	"k8s.io/apimachinery/pkg/util/json"
)

type DiagnoseArgs struct {
	Src            string
	Dst            string
	Port           uint16
	Protocol       string
	CloudProvider  string
	KubeConfig     string
	CollectorImage string
	ExtraArgs      []string
}

type DiagnoseResult struct {
	Succeed bool
	Error   string
	Summary *ui.DiagnoseSummary
}

type skoopExecutor interface {
	Diagnose(args DiagnoseArgs) (DiagnoseResult, error)
}

type directExecutor struct {
	path string
}

func newDirectExecutor(path string) *directExecutor {
	// todo: check path exists
	return &directExecutor{path: path}
}

func (r *directExecutor) Diagnose(args DiagnoseArgs) (DiagnoseResult, error) {
	var stdout, stderr bytes.Buffer

	argString := r.generateArgs(args)
	cmd := exec.Command(r.path, argString...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return DiagnoseResult{}, err
		}
	}

	succeed := err == nil

	result := DiagnoseResult{
		Succeed: succeed,
		Error:   stderr.String(),
	}

	if succeed {
		err = json.Unmarshal(stdout.Bytes(), &result.Summary)
		if err != nil {
			return DiagnoseResult{}, err
		}
	}

	return result, nil
}

func (r *directExecutor) generateArgs(args DiagnoseArgs) []string {
	argString := []string{
		"-s",
		args.Src,
		"-d",
		args.Dst,
		"-p",
		fmt.Sprintf("%d", args.Port),
		"--collector-image",
		args.CollectorImage,
		"--format",
		"json",
		"--output",
		"-",
	}

	if args.Protocol != "" {
		argString = append(argString, "--protocol", args.Protocol)
	}

	if args.CloudProvider != "" {
		argString = append(argString, "--cloud-provider", args.CloudProvider)
	}

	if args.KubeConfig != "" {
		argString = append(argString, "--kube-config", args.KubeConfig)
	}

	argString = append(argString, args.ExtraArgs...)

	return argString
}
