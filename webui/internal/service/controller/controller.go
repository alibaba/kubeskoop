package controller

import (
	"fmt"
	"github.com/imroc/req/v3"
	"github.com/kubeskoop/webconsole/internal/config"
	"net/http"
)

const (
	createDiagnosisURL = "%s/diagnose"
	listDiagnosisURL   = "%s/diagnoses"
)

var Service Svc

type DiagnosisTask struct {
	Source      string `json:"source"`
	Destination struct {
		Address string `json:"address"`
		Port    uint16 `json:"port"`
	} `json:"destination"`
	Protocol string `json:"protocol"`
}

type DiagnosisTaskResult struct {
	TaskID     int           `json:"task_id"`
	TaskConfig DiagnosisTask `json:"task_config"`
	Status     string        `json:"status"`
	Result     string        `json:"result"`
	Message    string        `json:"message"`
}

type responseError struct {
	Error string `json:"error"`
}

type Svc interface {
	CreateDiagnosis(task DiagnosisTask) (string, error)
	ListDiagnosisResult() ([]DiagnosisTaskResult, error)
	GetDiagnosisResult(taskID int) (DiagnosisTaskResult, error)
}

func init() {
	svc, err := newDefaultService()
	if err != nil {
		panic(err)
	}
	Service = svc
}

func newDefaultService() (Svc, error) {
	return &defaultService{}, nil
}

type defaultService struct {
}

func (d *defaultService) CreateDiagnosis(task DiagnosisTask) (string, error) {
	url := fmt.Sprintf(createDiagnosisURL, config.Global.Controller.Endpoint)
	r := req.NewRequest()
	r.SetURL(url)
	r.SetBodyJsonMarshal(task)
	resp, err := r.Send(http.MethodPost, url)
	if err != nil {
		return "", fmt.Errorf("error sending request: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		var e responseError
		_ = resp.UnmarshalJson(&e)
		return "", fmt.Errorf("error %d in response: %s", resp.StatusCode, e.Error)

	}

	var response struct {
		TaskID string `json:"taskID"`
	}

	err = resp.UnmarshalJson(&response)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling result: %s", err.Error())
	}

	return response.TaskID, nil
}

func (d *defaultService) ListDiagnosisResult() ([]DiagnosisTaskResult, error) {
	url := fmt.Sprintf(listDiagnosisURL, config.Global.Controller.Endpoint)
	resp, err := req.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		var e responseError
		_ = resp.UnmarshalJson(&e)
		return nil, fmt.Errorf("error %d in response: %s", resp.StatusCode, e.Error)

	}

	var ret []DiagnosisTaskResult
	err = resp.UnmarshalJson(&ret)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling result: %s", err.Error())
	}

	return ret, nil
}

func (d *defaultService) GetDiagnosisResult(taskID int) (DiagnosisTaskResult, error) {
	diagnoses, err := d.ListDiagnosisResult()
	if err != nil {
		return DiagnosisTaskResult{}, err
	}

	for _, d := range diagnoses {
		if d.TaskID == taskID {
			return d, nil
		}
	}

	return DiagnosisTaskResult{}, fmt.Errorf("cannot find diagnosis result by id %d", taskID)
}
