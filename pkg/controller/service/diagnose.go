package service

import (
	"context"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"sync"
)

// todo reflect to generic task definition
type DiagnoseTaskResult struct {
	TaskID     int                      `json:"task_id"`
	TaskConfig *skoopContext.TaskConfig `json:"task_config"`
	Status     string                   `json:"status"`
	Result     string                   `json:"result"`
	Message    string                   `json:"message"`
}

var (
	tasks = sync.Map{}
	idx   = 0
)

func (c *controller) Diagnose(ctx context.Context, args *skoopContext.TaskConfig) (int, error) {
	taskID := idx
	idx++
	tasks.Store(taskID, DiagnoseTaskResult{TaskID: taskID, TaskConfig: args, Status: "running"})
	go func() {
		result, err := c.diagnosor.Diagnose(context.TODO(), args)
		if err != nil {
			tasks.Store(taskID, DiagnoseTaskResult{TaskID: taskID, TaskConfig: args, Status: "failed", Message: err.Error()})
		} else {
			tasks.Store(taskID, DiagnoseTaskResult{TaskID: taskID, TaskConfig: args, Status: "success", Result: result})
		}
	}()
	return taskID, nil
}

func (c *controller) DiagnoseList(ctx context.Context) ([]DiagnoseTaskResult, error) {
	taskSlice := make([]DiagnoseTaskResult, 0)
	tasks.Range(func(key, value interface{}) bool {
		taskSlice = append(taskSlice, value.(DiagnoseTaskResult))
		return true
	})
	return taskSlice, nil
}
