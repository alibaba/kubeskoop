package taskagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var validFilterRegex = regexp.MustCompile(`^[a-zA-Z0-9 ._:/\(\)\[\]!=<>&|,+*^%\-]+$`)

type capture struct {
	args        []string
	captureFile string
	timeout     time.Duration
}

func validateCaptureFilter(filter string) error {
	if filter == "" {
		return nil
	}
	if !validFilterRegex.MatchString(filter) {
		return fmt.Errorf("invalid capture filter %q: contains disallowed characters", filter)
	}
	for _, token := range strings.Fields(filter) {
		if strings.HasPrefix(token, "-") {
			return fmt.Errorf("invalid capture filter: token %q looks like a flag, not a BPF expression", token)
		}
	}
	return nil
}

func buildTcpdumpArgs(file, filter string) []string {
	args := []string{"-i", "any", "-C", "100", "-w", file}
	if filter != "" {
		args = append(args, "--")
		args = append(args, strings.Fields(filter)...)
	}
	return args
}

func (a *Agent) generateCaptures(id string, task *rpc.CaptureInfo) ([]capture, error) {
	if err := validateCaptureFilter(task.GetFilter()); err != nil {
		return nil, err
	}

	if task.Pod != nil && !task.Pod.HostNetwork {
		var podEntry *nettop.Entity
		entries := nettop.GetAllUniqueNetnsEntity()
		for _, e := range entries {
			if e.GetPodNamespace() == task.Pod.Namespace && e.GetPodName() == task.Pod.Name {
				podEntry = e
			}
		}
		if podEntry == nil {
			return nil, fmt.Errorf("pod not found on nettop cache")
		}
		file := fmt.Sprintf("/tmp/%s_%s_%s_pod.pcap", id, task.Pod.Namespace, task.Pod.Name)
		tcpdumpArgs := buildTcpdumpArgs(file, task.GetFilter())
		nsenterArgs := []string{"-t", fmt.Sprintf("%d", podEntry.GetPid()), "-n", "--", "tcpdump"}
		nsenterArgs = append(nsenterArgs, tcpdumpArgs...)
		return []capture{
			{
				args:        append([]string{"nsenter"}, nsenterArgs...),
				captureFile: file,
				timeout:     time.Duration(task.CaptureDurationSeconds) * time.Second,
			},
		}, nil
	}

	file := fmt.Sprintf("/tmp/%s_%s_host.pcap", id, task.Node.Name)
	tcpdumpArgs := buildTcpdumpArgs(file, task.GetFilter())
	return []capture{
		{
			args:        append([]string{"tcpdump"}, tcpdumpArgs...),
			captureFile: file,
			timeout:     time.Duration(task.CaptureDurationSeconds) * time.Second,
		},
	}, nil
}

func (a *Agent) execute(captures []capture) (string, []byte, error) {
	log.Infof("start capture: %v", captures)
	wg := errgroup.Group{}
	for _, c := range captures {
		task := c
		wg.Go(func() error {
			var (
				output []byte
				err    error
				cmd    = exec.Command(task.args[0], task.args[1:]...)
			)
			go func() {
				output, err = cmd.CombinedOutput()
			}()
			time.Sleep(task.timeout)
			_ = cmd.Process.Signal(syscall.SIGTERM)
			time.Sleep(1 * time.Second)
			if err != nil {
				if strings.Contains(err.Error(), "no child processes") {
					return nil
				}
				return fmt.Errorf("error running command: %v, output: %v", err, string(output))
			}
			return nil
		})
	}
	err := wg.Wait()
	if err != nil {
		return "", nil, err
	}
	defer func() {
		lo.Map(captures, func(c capture, _ int) error {
			return os.Remove(c.captureFile)
		})
	}()

	fileType := "pcap"
	var outputCmd *exec.Cmd
	if len(captures) > 1 {
		fileType = "tar.gz"
		outputCmd = exec.Command("tar", append([]string{"-czf", "-"}, lo.Map(captures, func(c capture, _ int) string { return c.captureFile })...)...)
	} else {
		outputCmd = exec.Command("cat", captures[0].captureFile)
	}
	output, err := outputCmd.Output()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get capture result: %v", err)
	}
	return fileType, output, nil
}

func (a *Agent) ProcessCapture(task *rpc.ServerTask) error {
	captures, err := a.generateCaptures(task.Task.Id, task.GetTask().GetCapture())
	var (
		fileType       string
		captureContent []byte
	)
	if err == nil {
		fileType, captureContent, err = a.execute(captures)
	}

	if err != nil {
		log.Errorf("failed to run command: %v", err)
		_, err = a.grpcClient.UploadTaskResult(context.TODO(), &rpc.TaskResult{
			Id:      task.Task.Id,
			Type:    task.Task.Type,
			Success: false,
			Task:    task.GetTask().GetCapture(),
			Message: fmt.Sprintf("failed to run command: %v", err),
		})
		if err != nil {
			log.Errorf("failed to upload task result: %v", err)
		}
		return err
	}

	_, err = a.grpcClient.UploadTaskResult(context.TODO(), &rpc.TaskResult{
		Id:      task.Task.Id,
		Type:    task.Task.Type,
		Success: true,
		Message: "success",
		Task:    task.GetTask().GetCapture(),
		TaskResultInfo: &rpc.TaskResult_Capture{Capture: &rpc.CaptureResult{
			FileType: fileType,
			Message:  captureContent,
		}},
	})
	if err != nil {
		log.Errorf("failed to upload task result: %v", err)
	}

	return nil
}
