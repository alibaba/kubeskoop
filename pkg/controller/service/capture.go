package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
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

func (c *controller) storeCaptureFile(ctx context.Context, id int, result *rpc.CaptureResult) (string, error) {
	captureFileName := fmt.Sprintf("capture_task_%d_%d.pcap", id, time.Now().Unix())
	err := os.WriteFile("/tmp/"+captureFileName, result.Message, 0644)
	if err != nil {
		return "", err
	}
	return captureFileName, nil
}

func (c *controller) DownloadCaptureFile(ctx context.Context, id int) (string, int64, io.ReadCloser, error) {
	var filename string
	captureTasks.Range(func(key, value interface{}) bool {
		capture := value.(*CaptureTaskResult)
		if capture.TaskID == id {
			filename = capture.Result
			return false
		}
		return true
	})
	if filename == "" {
		return "", 0, nil, fmt.Errorf("capture file for %v not found", id)
	}
	captureFD, err := os.Open("/tmp/" + filename)
	if err != nil {
		return filename, 0, nil, fmt.Errorf("open capture file %s failed: %v", filename, err)
	}
	fs, err := captureFD.Stat()
	if err != nil {
		return "", 0, nil, fmt.Errorf("stat capture file %s failed: %v", filename, err)
	}
	return filename, fs.Size(), captureFD, nil
}
