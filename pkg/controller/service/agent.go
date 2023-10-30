package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	log "k8s.io/klog/v2"
	"strconv"
)

type taskWatcher struct {
	taskChan chan *rpc.ServerTask
	filter   *rpc.TaskFilter
}

func (c *controller) RegisterAgent(ctx context.Context, info *rpc.AgentInfo) (*rpc.ControllerInfo, error) {
	return nil, nil
}

func (c *controller) ReportEvents(server rpc.ControllerRegisterService_ReportEventsServer) error {
	return nil
}

func (c *controller) WatchTasks(filter *rpc.TaskFilter, server rpc.ControllerRegisterService_WatchTasksServer) error {
	w := &taskWatcher{
		taskChan: make(chan *rpc.ServerTask, 1),
		filter:   filter,
	}
	log.Infof("watch tasks for filter: %v", filter)
	c.taskWatcher.Store(filter, w)
	defer c.taskWatcher.Delete(filter)
	for {
		select {
		case task := <-w.taskChan:
			log.Infof("send task to agent %v: %v", filter, task)
			err := server.Send(task)
			if err != nil {
				log.Errorf("send task to agent failed: %v", err)
				return err
			}
		case <-server.Context().Done():
			log.Infof("task watcher closed: %v", filter)
			return nil
		}
	}
}

func (c *controller) UploadTaskResult(ctx context.Context, result *rpc.TaskResult) (*rpc.TaskResultReply, error) {
	id, _ := strconv.Atoi(result.Id)
	value, ok := captureTasks.Load(id)
	if ok && result.GetType() == rpc.TaskType_Capture {
		captureResult := value.(*CaptureTaskResult)
		captureResult.Message = result.GetMessage()
		captureResult.Status = "success"
		captureFile, err := c.storeCaptureFile(ctx, id, result.GetCapture())
		if err != nil {
			return nil, fmt.Errorf("store capture file failed: %v", err)
		}
		captureResult.Result = captureFile
		captureTasks.Store(id, captureResult)
	}

	return &rpc.TaskResultReply{
		Success: true,
		Message: "",
	}, nil
}

func (c *controller) GetAgentList() []*rpc.AgentInfo {
	//TODO implement me
	panic("implement me")
}

func (c *controller) commitTask(node string, task *rpc.Task) ([]string, error) {
	var commitedNode []string
	c.taskWatcher.Range(func(key, value interface{}) bool {
		filter := key.(*rpc.TaskFilter)
		if filter.GetNodeName() == node && filter.Type == task.Type {
			valueChan := value.(*taskWatcher)
			valueChan.taskChan <- &rpc.ServerTask{
				Server: &rpc.ControllerInfo{Version: ""},
				Task:   task,
			}
			commitedNode = append(commitedNode, node)
			return false
		}
		return true
	})
	if len(commitedNode) == 1 {
		return commitedNode, nil
	} else {
		return nil, fmt.Errorf("there is no client to process task: %v", task)
	}
}
