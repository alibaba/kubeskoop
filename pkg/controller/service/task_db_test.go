package service

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDB(t *testing.T) {
	startTime := time.Now().Format("2006-01-02 15:04:05")
	task := DiagnoseTaskResult{TaskConfig: "", StartTime: startTime, Status: "running"}
	id, err := saveTask(&task)
	if err != nil {
		t.Fatalf("failed save task, err:%v", err)
	}
	task.TaskID = id

	task.Status = "success"
	updateTask(&task)

	tasks, err := listTasks()
	if err != nil {
		t.Fatalf("failed list task, err:%v", err)
	}
	for _, task := range tasks {
		data, _ := json.Marshal(task)
		t.Logf("task: %s", string(data))
	}

}
