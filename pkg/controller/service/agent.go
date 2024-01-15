package service

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	"github.com/samber/lo"
	log "k8s.io/klog/v2"
)

type taskWatcher struct {
	taskChan chan *rpc.ServerTask
	filter   *rpc.TaskFilter
}

func (c *controller) RegisterAgent(_ context.Context, _ *rpc.AgentInfo) (*rpc.ControllerInfo, error) {
	return nil, nil
}

func (c *controller) ReportEvents(_ rpc.ControllerRegisterService_ReportEventsServer) error {
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
	id := result.Id
	if watcher, ok := c.resultWatchers.Load(id); ok {
		wchan := watcher.(chan *rpc.TaskResult)
		wchan <- result
		close(wchan)
		return &rpc.TaskResultReply{
			Success: true,
			Message: "",
		}, nil
	}

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

var (
	taskIdx int64
)

func getTaskIdx() int64 {
	return atomic.AddInt64(&taskIdx, 1)
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
	}

	return nil, fmt.Errorf("there is no client to process task: %v", task)
}

func (c *controller) waitTaskResult(ctx context.Context, id string) (*rpc.TaskResult, error) {
	resultChan := make(chan *rpc.TaskResult, 1)
	c.resultWatchers.Store(id, resultChan)
	defer c.resultWatchers.Delete(id)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result, ok := <-resultChan:
		if !ok {
			return nil, fmt.Errorf("task result channel closed")
		}
		return result, nil
	}
}
