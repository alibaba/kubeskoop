package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/exporter/loki"
	"io"
	"os"
	"sync"
	"time"

	"github.com/alibaba/kubeskoop/pkg/controller/diagnose"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"
	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ControllerService interface {
	rpc.ControllerRegisterServiceServer
	GetAgentList() []*rpc.AgentInfo
	Capture(ctx context.Context, capture *CaptureArgs) (int, error)
	CaptureList(ctx context.Context) (map[int][]*CaptureTaskResult, error)
	QueryRangeEvent(ctx context.Context, start, end time.Time, filters map[string][]string, limit int) ([]Event, error)
	Diagnose(ctx context.Context, args *skoopContext.TaskConfig) (int64, error)
	DiagnoseList(ctx context.Context) ([]DiagnoseTaskResult, error)
	DownloadCaptureFile(ctx context.Context, id int) (string, int64, io.ReadCloser, error)
	PodList(ctx context.Context) ([]*Pod, error)
	NodeList(ctx context.Context) ([]*Node, error)
	NamespaceList(ctx context.Context) ([]string, error)
	QueryPrometheus(ctx context.Context, query string, ts time.Time) (model.Value, promv1.Warnings, error)
	GetPodNodeInfoFromMetrics(ctx context.Context, ts time.Time) (model.Vector, model.Vector, error)
	PingMesh(ctx context.Context, pingmesh *PingMeshArgs) (*PingMeshResult, error)
}

func NewControllerService() (ControllerService, error) {
	ctrl := &controller{
		diagnosor:      diagnose.NewDiagnoseController(),
		taskWatcher:    sync.Map{},
		resultWatchers: sync.Map{},
	}
	var (
		restConfig *rest.Config
		err        error
	)
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		restConfig, _, err = utils.NewConfig("~/.kube/config")
	} else {
		restConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("error get incluster config, err: %v", err)
	}
	ctrl.k8sClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("error create k8s client, err: %v", err)
	}

	if promEndpoint, ok := os.LookupEnv("PROMETHEUS_ENDPOINT"); ok {
		promClient, err := api.NewClient(api.Config{
			Address: promEndpoint,
		})
		if err != nil {
			return nil, err
		}
		ctrl.promClient = promClient
	}

	if lokiEndpoint, ok := os.LookupEnv("LOKI_ENDPOINT"); ok {
		lokiClient, err := lokiwrapper.NewClient(lokiEndpoint)
		if err != nil {
			return nil, err
		}
		ctrl.lokiClient = lokiClient
	}

	return ctrl, nil
}

type controller struct {
	rpc.UnimplementedControllerRegisterServiceServer
	diagnosor      diagnose.DiagnoseController
	k8sClient      *kubernetes.Clientset
	taskWatcher    sync.Map
	resultWatchers sync.Map
	promClient     api.Client
	lokiClient     *lokiwrapper.Client
}
