package service

import (
	"context"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"strconv"
	"sync"
)

type CaptureArgs struct {
	Pod struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		IPv4      string `json:"ipv4"`
		IPv6      string `json:"ipv6"`
	} `json:"pod"`
	Node struct {
		Name string `json:"name"`
	} `json:"node"`
	CaptureHostNs bool `json:"capture_host_ns"`
}

// todo reflect to generic task definition
type CaptureTaskResult struct {
	TaskID     int          `json:"task_id"`
	TaskConfig *CaptureArgs `json:"task_config"`
	Status     string       `json:"status"`
	Result     string       `json:"result"`
	Message    string       `json:"message"`
}

var (
	captureTasks = sync.Map{}
	captureIdx   = 0
)

func (c *controller) Capture(ctx context.Context, capture *CaptureArgs) (int, error) {
	taskID := captureIdx
	captureIdx++
	_, err := c.commitTask(capture.Node.Name, &rpc.Task{
		Type: rpc.TaskType_Capture,
		Id:   strconv.Itoa(taskID),
		TaskInfo: &rpc.Task_Capture{
			Capture: &rpc.CaptureInfo{
				Pod: &rpc.PodInfo{
					Name:      capture.Pod.Name,
					Namespace: capture.Pod.Namespace,
					Ipv4:      capture.Pod.IPv4,
					Ipv6:      capture.Pod.IPv6,
				},
				Node: &rpc.NodeInfo{
					Name: capture.Node.Name,
				},
				CaptureHostNs: capture.CaptureHostNs,
			},
		},
	})
	if err != nil {
		return 0, err
	}
	captureTasks.Store(taskID, &CaptureTaskResult{
		TaskID:     taskID,
		TaskConfig: capture,
		Status:     "running",
		Result:     "",
		Message:    "",
	})

	return taskID, nil
}

func (c *controller) CaptureList(ctx context.Context) ([]*CaptureTaskResult, error) {
	var results []*CaptureTaskResult
	captureTasks.Range(func(key, value interface{}) bool {
		capture := value.(*CaptureTaskResult)
		results = append(results, capture)
		return true
	})
	return results, nil
}
