package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/samber/lo"
	log "k8s.io/klog/v2"
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
	switch result.GetType() {
	case rpc.TaskType_Capture:
		return c.storeCaptureResult(ctx, result)
	}
	return nil, nil
}

func (c *controller) GetAgentList() []*rpc.AgentInfo {
	//TODO implement me
	panic("implement me")
}

func (c *controller) commitTask(node string, task *rpc.Task) ([]string, error) {
	var commitedNode []string
	c.taskWatcher.Range(func(key, value interface{}) bool {
		filter := key.(*rpc.TaskFilter)
		if filter.GetNodeName() == node {
			if lo.Reduce[rpc.TaskType, bool](filter.GetType(), func(acc bool, t rpc.TaskType, _ int) bool { return acc || t == task.Type }, false) {
				valueChan := value.(*taskWatcher)
				valueChan.taskChan <- &rpc.ServerTask{
					Server: &rpc.ControllerInfo{Version: ""},
					Task:   task,
				}
				commitedNode = append(commitedNode, node)
				return false
			}
		}
		return true
	})
	if len(commitedNode) > 0 {
		return commitedNode, nil
	} else {
		return nil, fmt.Errorf("there is no client to process task: %v", task)
	}
}
