package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/samber/lo"
	"io"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
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

type Pod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type Node struct {
	Name string `json:"name"`
}

func (c *controller) PodList(ctx context.Context) ([]*Pod, error) {
	pods, err := c.k8sClient.CoreV1().Pods("").List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods failed: %v", err)
	}
	return lo.Map[corev1.Pod, *Pod](pods.Items, func(pod corev1.Pod, idx int) *Pod {
		return &Pod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}
	}), nil
}

func (c *controller) NodeList(ctx context.Context) ([]*Node, error) {
	nodes, err := c.k8sClient.CoreV1().Nodes().List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods failed: %v", err)
	}
	return lo.Map[corev1.Node, *Node](nodes.Items, func(node corev1.Node, idx int) *Node {
		return &Node{
			Name: node.Name,
		}
	}), nil
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

	captureInfo := &rpc.CaptureInfo{
		CaptureHostNs: capture.CaptureHostNs,
	}

	if capture.Pod.Name != "" {
		p, err := c.k8sClient.CoreV1().Pods(capture.Pod.Namespace).Get(ctx, capture.Pod.Name, v1.GetOptions{})
		if err != nil {
			return 0, fmt.Errorf("get pod %s/%s failed: %v", capture.Pod.Namespace, capture.Pod.Name, err)
		}
		if p.Status.Phase != corev1.PodRunning {
			return 0, fmt.Errorf("pod %s/%s is not running", capture.Pod.Namespace, capture.Pod.Name)
		}
		var (
			v4 net.IP
			v6 net.IP
		)
		for _, ip := range p.Status.PodIPs {
			if netIP := net.ParseIP(ip.IP); netIP != nil {
				if netIP.To4() != nil {
					v4 = netIP
				} else if netIP.To16() != nil {
					v6 = netIP
				}
			}
		}
		if v4 == nil && v6 == nil {
			return 0, fmt.Errorf("pod %s/%s has no ip", capture.Pod.Namespace, capture.Pod.Name)
		}
		captureInfo.Pod = &rpc.PodInfo{
			Name:      capture.Pod.Name,
			Namespace: capture.Pod.Namespace,
			Ipv4:      v4.String(),
			Ipv6:      v6.String(),
		}
		captureInfo.Node = &rpc.NodeInfo{
			Name: p.Spec.NodeName,
		}
	} else if capture.Node.Name != "" {
		captureInfo.Node = &rpc.NodeInfo{
			Name: capture.Node.Name,
		}
	} else {
		return 0, fmt.Errorf("invalid capture args: %+v", capture)
	}

	_, err := c.commitTask(captureInfo.Node.Name, &rpc.Task{
		Type: rpc.TaskType_Capture,
		Id:   strconv.Itoa(taskID),
		TaskInfo: &rpc.Task_Capture{
			Capture: captureInfo,
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
