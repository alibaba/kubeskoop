package taskagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	//fixme: increase capture size by grpc
	dumpCommand = "%v tcpdump -i any -C 100 -w %v %v" // size limit 100M
)

type capture struct {
	captureCommand string
	captureFile    string
	timeout        time.Duration
}

func (a *Agent) generateCaptures(id string, task *rpc.CaptureInfo) ([]capture, error) {
	if task.Pod != nil && !task.Pod.HostNetwork {
		var podEntry *nettop.Entity
		entries := nettop.GetAllEntity()
		for _, e := range entries {
			if e.GetPodNamespace() == task.Pod.Namespace && e.GetPodName() == task.Pod.Name {
				podEntry = e
			}
		}
		if podEntry == nil {
			return nil, fmt.Errorf("pod not found on nettop cache")
		}
		file := fmt.Sprintf("/tmp/%s_%s_%s_pod.pcap", id, task.Pod.Namespace, task.Pod.Name)
		files := []capture{
			{
				captureCommand: fmt.Sprintf(dumpCommand, fmt.Sprintf("nsenter -t %v -n --", podEntry.GetPid()), file, task.GetFilter()),
				captureFile:    file,
				timeout:        time.Duration(task.CaptureDurationSeconds) * time.Second,
			},
		}
		return files, nil
	}

	file := fmt.Sprintf("/tmp/%s_%s_host.pcap", id, task.Node.Name)
	return []capture{
		{
			captureCommand: fmt.Sprintf(dumpCommand, "", file, task.GetFilter()),
			captureFile:    file,
			timeout:        time.Duration(task.CaptureDurationSeconds) * time.Second,
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
				cmd    = exec.Command("sh", "-c", task.captureCommand)
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
	outputCmd := exec.Command("sh", "-c", fmt.Sprintf("cat %v", captures[0].captureFile))
	if len(captures) > 1 {
		fileType = "tar.gz"
		outputCmd = exec.Command("tar", append([]string{"-czf", "-"}, lo.Map(captures, func(c capture, _ int) string { return c.captureFile })...)...)
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
