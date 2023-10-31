package service

import (
	"context"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"golang.org/x/exp/slices"
	"sync"
	"time"
)

// todo reflect to generic task definition
type DiagnoseTaskResult struct {
	TaskID     int                      `json:"task_id"`
	TaskConfig *skoopContext.TaskConfig `json:"task_config"`
	StartTime  string                   `json:"start_time"`
	FinishTime string                   `json:"finish_time"`
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
	startTime := time.Now().Format("2006-01-02 15:04:05")
	tasks.Store(taskID, DiagnoseTaskResult{TaskID: taskID, TaskConfig: args, StartTime: startTime, Status: "running"})
	go func() {
		result, err := c.diagnosor.Diagnose(context.TODO(), args)
		finishTime := time.Now().Format("2006-01-02 15:04:05")
		if err != nil {
			tasks.Store(taskID,
				DiagnoseTaskResult{TaskID: taskID, TaskConfig: args, StartTime: startTime, FinishTime: finishTime, Status: "failed", Message: err.Error()})
		} else {
			tasks.Store(taskID,
				DiagnoseTaskResult{TaskID: taskID, TaskConfig: args, StartTime: startTime, FinishTime: finishTime, Status: "success", Result: result})
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
	slices.SortFunc(taskSlice, func(a, b DiagnoseTaskResult) bool {
		return a.TaskID < b.TaskID
	})
	return taskSlice, nil
}
