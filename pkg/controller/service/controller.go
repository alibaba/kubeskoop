package service

import (
	"context"
	"github.com/alibaba/kubeskoop/pkg/controller/diagnose"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"io"
	"sync"
)

type ControllerService interface {
	rpc.ControllerRegisterServiceServer
	GetAgentList() []*rpc.AgentInfo
	Capture(ctx context.Context, capture *CaptureArgs) (int, error)
	CaptureList(ctx context.Context) ([]*CaptureTaskResult, error)
	WatchEvents() <-chan *rpc.Event
	Diagnose(ctx context.Context, args *skoopContext.TaskConfig) (int, error)
	DiagnoseList(ctx context.Context) ([]DiagnoseTaskResult, error)
	DownloadCaptureFile(ctx context.Context, id int) (string, int64, io.ReadCloser, error)
}

func NewControllerService() ControllerService {
	return &controller{
		diagnosor:   diagnose.NewDiagnoseController(),
		taskWatcher: sync.Map{},
	}
}

type controller struct {
	rpc.UnimplementedControllerRegisterServiceServer
	diagnosor   diagnose.DiagnoseController
	taskWatcher sync.Map
}
