package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"sync"

	"github.com/alibaba/kubeskoop/pkg/controller/k8s"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	log "k8s.io/klog/v2"
)

const (
	typePod  = "Pod"
	typeNode = "Node"

	statusSuccess = "success"
	statusFailed  = "failed"
)

type CaptureArgs struct {
	CaptureList []struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Nodename  string `json:"nodename"`
		Namespace string `json:"namespace"`
	} `json:"capture_list"`
	CaptureDurationSeconds int    `json:"capture_duration_seconds"`
	Filter                 string `json:"filter"`
}

type Pod struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Nodename  string            `json:"nodename"`
	Labels    map[string]string `json:"labels"`
}

type Node struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

func podListWithInformer() ([]*Pod, error) {
	pods, err := k8s.PodInformer.Lister().Pods("").List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list pods failed: %v", err)
	}
	return lo.Map[*corev1.Pod, *Pod](pods, func(pod *corev1.Pod, idx int) *Pod {
		return &Pod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Nodename:  pod.Spec.NodeName,
			Labels:    pod.Labels,
		}
	}), nil

}

func (c *controller) PodList(ctx context.Context) ([]*Pod, error) {
	if k8s.PodInformer != nil {
		return podListWithInformer()
	}

	pods, err := c.k8sClient.CoreV1().Pods("").List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods failed: %v", err)
	}
	return lo.Map[corev1.Pod, *Pod](pods.Items, func(pod corev1.Pod, idx int) *Pod {
		return &Pod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Nodename:  pod.Spec.NodeName,
			Labels:    pod.Labels,
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
			Name:   node.Name,
			Labels: node.Labels,
		}
	}), nil
}

func (c *controller) NamespaceList(ctx context.Context) ([]string, error) {
	namespaces, err := c.k8sClient.CoreV1().Namespaces().List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods failed: %v", err)
	}
	return lo.Map[corev1.Namespace, string](namespaces.Items, func(namespace corev1.Namespace, idx int) string {
		return namespace.Name
	}), nil
}

type TaskSpec struct {
	TaskType  string `json:"task_type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// todo reflect to generic task definition
type CaptureTaskResult struct {
	TaskID  int       `json:"task_id"`
	Spec    *TaskSpec `json:"spec"`
	Status  string    `json:"status"`
	Result  string    `json:"result"`
	Message string    `json:"message"`
}

var (
	captureTasks = sync.Map{}
)

func (c *controller) Capture(ctx context.Context, capture *CaptureArgs) (int, error) {
	taskID := int(getTaskIdx())

	var tasksToCommit []*rpc.CaptureInfo
	for _, captureItem := range capture.CaptureList {
		task := &rpc.CaptureInfo{
			CaptureDurationSeconds: int32(capture.CaptureDurationSeconds),
			Filter:                 capture.Filter,
		}
		switch captureItem.Type {
		case typePod:
			var err error
			task.Pod, task.Node, _, err = c.getPodInfo(ctx, captureItem.Namespace, captureItem.Name)
			if err != nil {
				return 0, err
			}
			task.CaptureType = typePod
		case typeNode:
			task.Node = &rpc.NodeInfo{
				Name: captureItem.Name,
			}
			task.CaptureType = typeNode
		default:
			return 0, fmt.Errorf("invalid capture type: %v", captureItem.Type)
		}
		tasksToCommit = append(tasksToCommit, task)
	}

	var resultList []*CaptureTaskResult
	for _, captureInfo := range tasksToCommit {
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
		var spec *TaskSpec
		if captureInfo.GetCaptureType() == typePod {
			spec = &TaskSpec{
				TaskType:  captureInfo.CaptureType,
				Name:      captureInfo.GetPod().Name,
				Namespace: captureInfo.GetPod().Namespace,
			}
		} else {
			spec = &TaskSpec{
				TaskType: captureInfo.CaptureType,
				Name:     captureInfo.GetNode().Name,
			}
		}
		resultList = append(resultList, &CaptureTaskResult{
			TaskID:  taskID,
			Spec:    spec,
			Status:  "running",
			Result:  "",
			Message: "",
		})
	}

	captureTasks.Store(taskID, resultList)

	return taskID, nil
}

func (c *controller) CaptureList(_ context.Context) (map[int][]*CaptureTaskResult, error) {
	results := map[int][]*CaptureTaskResult{}
	captureTasks.Range(func(key, value interface{}) bool {
		id := key.(int)
		capture := value.([]*CaptureTaskResult)
		results[id] = capture
		return true
	})
	return results, nil
}

func (c *controller) storeCaptureFile(_ context.Context, spec *TaskSpec, id int, result *rpc.CaptureResult) (string, error) {
	taskPath := fmt.Sprintf("/tmp/task_%d/", id)
	err := os.MkdirAll(taskPath, 0755)
	if err != nil {
		return "", err
	}
	captureFileName := ""
	if spec.TaskType == typePod {
		captureFileName = fmt.Sprintf("capture_task_%d_%s_%s", id, spec.Namespace, spec.Name)
	} else {
		captureFileName = fmt.Sprintf("capture_task_%d_%s_%s", id, "node", spec.Name)
	}
	captureFileName = captureFileName + "." + result.GetFileType()
	err = os.WriteFile(taskPath+captureFileName, result.Message, 0644)
	if err != nil {
		return "", err
	}
	return captureFileName, nil
}

func (c *controller) DownloadCaptureFile(ctx context.Context, id int) (string, int64, io.ReadCloser, error) {
	filename := fmt.Sprintf("/tmp/capture_task_%d.tar.gz", id)
	compressResults := exec.CommandContext(ctx, "tar", "-czf", filename, fmt.Sprintf("/tmp/task_%d/", id))
	output, err := compressResults.CombinedOutput()
	if err != nil {
		return "", 0, nil, fmt.Errorf("error compress capture file: %v, output: %s", err, string(output))
	}
	captureFD, err := os.Open(filename)
	if err != nil {
		return filename, 0, nil, fmt.Errorf("open capture file %s failed: %v", filename, err)
	}
	fs, err := captureFD.Stat()
	if err != nil {
		return "", 0, nil, fmt.Errorf("stat capture file %s failed: %v", filename, err)
	}
	_, filename = path.Split(filename)
	return filename, fs.Size(), captureFD, nil
}

func (c *controller) storeCaptureResult(ctx context.Context, result *rpc.TaskResult) (*rpc.TaskResultReply, error) {
	id, _ := strconv.Atoi(result.Id)
	value, ok := captureTasks.Load(id)
	if ok && result.GetType() == rpc.TaskType_Capture {
		log.Infof("store capture result for %v, %v", id, result.GetMessage())
		captureResults := value.([]*CaptureTaskResult)
		for _, captureResult := range captureResults {
			if (result.GetTask().GetPod() != nil && result.GetTask().GetPod().GetNamespace() == captureResult.Spec.Namespace && result.GetTask().GetPod().GetName() == captureResult.Spec.Name) ||
				(result.GetTask().GetPod() == nil && result.GetTask().GetNode().GetName() == captureResult.Spec.Name) {
				captureResult.Message = result.GetMessage()
				if result.GetSuccess() {
					captureResult.Status = statusSuccess
					captureFile, err := c.storeCaptureFile(ctx, captureResult.Spec, id, result.GetCapture())
					if err != nil {
						return nil, fmt.Errorf("store capture file failed: %v", err)
					}
					captureResult.Result = captureFile
				} else {
					captureResult.Status = statusFailed
				}
			}
		}
	}

	return &rpc.TaskResultReply{
		Success: true,
		Message: "",
	}, nil
}
