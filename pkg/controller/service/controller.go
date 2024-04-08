package service

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/alibaba/kubeskoop/pkg/controller/db"
	exporter "github.com/alibaba/kubeskoop/pkg/exporter/cmd"
	lokiwrapper "github.com/alibaba/kubeskoop/pkg/exporter/loki"

	"github.com/alibaba/kubeskoop/pkg/controller/diagnose"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	skoopContext "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/client-go/kubernetes"
)

const (
	Namespace         = "kubeskoop"
	ExporterConfigMap = "kubeskoop-config"
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
	GetExporterConfig(ctx context.Context) (*exporter.InspServerConfig, error)
	UpdateExporterConfig(ctx context.Context, cfg *exporter.InspServerConfig) error
}

type Config struct {
	Namespace  string          `yaml:"namespace"`
	KubeConfig string          `yaml:"kubeConfig"`
	Prometheus string          `yaml:"prometheus"`
	Loki       string          `yaml:"loki"`
	DB         db.Config       `yaml:"database"`
	Diagnose   diagnose.Config `yaml:"diagnose"`
}

func NewControllerService(k8sClient *kubernetes.Clientset, config *Config) (ControllerService, error) {
	if config.Namespace == "" {
		config.Namespace = Namespace
	}

	ctrl := &controller{
		taskWatcher:    sync.Map{},
		resultWatchers: sync.Map{},
		Namespace:      config.Namespace,
		ConfigMapName:  ExporterConfigMap,
	}

	ctrl.k8sClient = k8sClient

	//init db
	if err := db.InitializeDB(&config.DB); err != nil {
		return nil, err
	}

	if config.Prometheus != "" {
		promClient, err := api.NewClient(api.Config{
			Address: config.Prometheus,
		})
		if err != nil {
			return nil, err
		}
		ctrl.promClient = promClient
	}

	if config.Loki != "" {
		lokiClient, err := lokiwrapper.NewClient(config.Loki)
		if err != nil {
			return nil, err
		}
		ctrl.lokiClient = lokiClient
	}

	//if diagnose kubeconfig is not set, use controller's kubeconfig as default
	if config.Diagnose.KubeConfig == "" {
		config.Diagnose.KubeConfig = config.KubeConfig
	}
	ctrl.diagnostor = diagnose.NewDiagnoseController(ctrl.Namespace, &config.Diagnose)

	return ctrl, nil
}

type controller struct {
	rpc.UnimplementedControllerRegisterServiceServer
	diagnostor     diagnose.Controller
	k8sClient      *kubernetes.Clientset
	taskWatcher    sync.Map
	resultWatchers sync.Map
	promClient     api.Client
	lokiClient     *lokiwrapper.Client
	Namespace      string
	ConfigMapName  string
}
