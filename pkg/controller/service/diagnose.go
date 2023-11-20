package service

import (
	"context"
	"encoding/json"
	"github.com/alibaba/kubeskoop/pkg/controller/db"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"time"
)

// todo reflect to generic task definition
type DiagnoseTaskResult struct {
	TaskID     int64  `json:"task_id" db:"id"`
	TaskConfig string `json:"task_config" db:"config"`
	StartTime  string `json:"start_time" db:"start_time"`
	FinishTime string `json:"finish_time" db:"finish_time"`
	Status     string `json:"status" db:"status""`
	Result     string `json:"result" db:"result"`
	Message    string `json:"message" db:"message"`
}

func (c *controller) Diagnose(ctx context.Context, args *skoopContext.TaskConfig) (int64, error) {
	startTime := time.Now().Format("2006-01-02 15:04:05")
	data, _ := json.Marshal(args)
	task := DiagnoseTaskResult{TaskConfig: string(data), StartTime: startTime, Status: "running"}
	id, err := saveTask(&task)
	if err != nil {
		return 0, err
	}
	task.TaskID = id

	go func() {
		result, err := c.diagnosor.Diagnose(context.TODO(), args)

		task.FinishTime = time.Now().Format("2006-01-02 15:04:05")
		if err != nil {
			task.Status = "failed"
			task.Message = err.Error()
		} else {
			task.Status = "success"
			task.Message = result
		}

		updateTask(&task)
	}()
	return task.TaskID, nil
}

func (c *controller) DiagnoseList(ctx context.Context) ([]DiagnoseTaskResult, error) {
	//TODO paging, do not list all at once
	taskSlice, err := listTasks()
	if err != nil {
		return nil, err
	}

	slices.SortFunc(taskSlice, func(a, b DiagnoseTaskResult) bool {
		return a.TaskID < b.TaskID
	})
	return taskSlice, nil
}

func saveTask(result *DiagnoseTaskResult) (int64, error) {
	insertSQL := `insert into tasks(config, start_time, status) values (:config, :start_time, :status)`
	return db.NamedInsert(insertSQL, result)
}

func updateTask(result *DiagnoseTaskResult) {
	updateSQL := `update tasks set finish_time=:finish_time, status=:status, result=:result, message=:message where id=:id`
	if _, err := db.NamedUpdate(updateSQL, result); err != nil {
		log.Errorf("failed update task %d, err: %v", result.TaskID, err)
	}
}

func listTasks() ([]DiagnoseTaskResult, error) {
	selectSQL := `select id, config, start_time, finish_time, status, result, message from tasks`
	rows, err := db.Query(selectSQL)
	if err != nil {
		return nil, err
	}
	var ret []DiagnoseTaskResult
	for rows.Next() {
		result := DiagnoseTaskResult{}
		rows.StructScan(&result)
		ret = append(ret, result)
	}
	return ret, nil
}
