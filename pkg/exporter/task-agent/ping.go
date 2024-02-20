package taskagent

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	log "github.com/sirupsen/logrus"
)

var (
	pingRegex *regexp.Regexp
)

func init() {
	pingRegex = regexp.MustCompile(`round-trip min/avg/max = ([0-9]*\.[0-9]+|[0-9]+)/([0-9]*\.[0-9]+|[0-9]+)/([0-9]*\.[0-9]+|[0-9]+) ms`)
}

func getLatency(pingResult string) (float64, float64, float64, error) {
	latencies := pingRegex.FindStringSubmatch(pingResult)
	if len(latencies) < 4 {
		return 0, 0, 0, fmt.Errorf("failed to parse ping result: %v", pingResult)
	}
	var latencyNums []float64
	for _, latency := range latencies[1:] {
		latencyNum, err := strconv.ParseFloat(latency, 64)
		if err != nil {
			return 0, 0, 0, err
		}
		latencyNums = append(latencyNums, latencyNum)
	}

	return latencyNums[0], latencyNums[1], latencyNums[2], nil
}

func (a *Agent) ping(task *rpc.PingInfo) (string, error) {
	var pingCmd string
	if task.Pod != nil && !task.Pod.HostNetwork {
		var podEntry *nettop.Entity
		entries := nettop.GetAllUniqueNetnsEntity()
		for _, e := range entries {
			if e.GetPodNamespace() == task.Pod.Namespace && e.GetPodName() == task.Pod.Name {
				podEntry = e
			}
		}
		if podEntry == nil {
			return "", fmt.Errorf("pod not found on nettop cache")
		}
		pingCmd = fmt.Sprintf("nsenter -t %v -n -- ping -A -c 100 -q -n %v", podEntry.GetPid(), task.GetDestination())
	} else {
		pingCmd = fmt.Sprintf("ping -A -c 100 -q -n %v", task.GetDestination())
	}
	log.Infof("running command: %v", pingCmd)
	cmd := exec.Command("sh", "-c", pingCmd)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error running command: %v, output: %v", err, string(output))
	}
	return string(output), nil
}

func (a *Agent) ProcessPing(task *rpc.ServerTask) error {
	var (
		min, avg, max float64
		output        string
		err           error
	)
	output, err = a.ping(task.GetTask().GetPing())
	if err == nil {
		min, avg, max, err = getLatency(output)
	}

	if err != nil {
		log.Errorf("failed to run command: %v", err)
		_, err = a.grpcClient.UploadTaskResult(context.TODO(), &rpc.TaskResult{
			Id:      task.Task.Id,
			Type:    task.Task.Type,
			Success: false,
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
		TaskResultInfo: &rpc.TaskResult_Ping{Ping: &rpc.PingResult{
			Max:     float32(max),
			Avg:     float32(avg),
			Min:     float32(min),
			Message: nil,
		}},
	})
	if err != nil {
		log.Errorf("failed to upload task result: %v", err)
	}

	return nil
}
