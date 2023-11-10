/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	task_agent "github.com/alibaba/kubeskoop/pkg/exporter/task-agent"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"

	"github.com/alibaba/kubeskoop/pkg/exporter/sink"

	"github.com/fsnotify/fsnotify"

	_ "net/http"       //for golangci-lint
	_ "net/http/pprof" //for golangci-lint once more

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"

	gops "github.com/google/gops/agent"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serverCmd represents the server command
var (
	serverCmd = &cobra.Command{
		Use:   "server",
		Short: "start inspector server",
		Run: func(cmd *cobra.Command, args []string) {
			insp := &inspServer{
				v:   *viper.New(),
				ctx: context.Background(),
			}

			log.Infof("start with config file %s", configPath)
			insp.v.SetConfigFile(configPath)
			err := insp.MergeConfig()
			if err != nil {
				log.Errorf("merge config err: %v", err)
				return
			}

			if insp.config.DebugMode {
				log.SetLevel(log.DebugLevel)
			}

			// nolint
			if err := nettop.StartCache(insp.ctx, sidecar); err != nil {
				log.Errorf("failed start cache: %v", err)
				return
			}

			defer nettop.StopCache()

			// config hot reload process
			insp.v.OnConfigChange(func(e fsnotify.Event) {
				log.Info("Start reload config")
				if err := insp.reload(); err != nil {
					log.Warnf("Reload config error: %v", err)
				}
				log.Info("Config reload succeed.")
			})
			insp.v.WatchConfig()

			if insp.config.EnableController {
				err = task_agent.NewTaskAgent().Run()
				if err != nil {
					log.Errorf("start task agent err: %v", err)
					return
				}
			}
			// block here
			err = insp.start()
			if err != nil {
				log.Infof("start server err: %v", err)
				return
			}
		},
	}

	configPath = "/etc/config/config.yaml"
)

type ProbeManager[T probe.Probe] interface {
	CreateProbe(config ProbeConfig) (T, error)
	StartProbe(ctx context.Context, probe T) error
	StopProbe(ctx context.Context, probe T) error
}

type DynamicProbeServer[T probe.Probe] struct {
	lock         sync.Mutex
	probeManager ProbeManager[T]
	lastConfig   []ProbeConfig
	probes       map[string]T
}

func NewDynamicProbeServer[T probe.Probe](probeManager ProbeManager[T]) *DynamicProbeServer[T] {
	return &DynamicProbeServer[T]{
		probeManager: probeManager,
		probes:       make(map[string]T),
	}
}

func (s *DynamicProbeServer[T]) probeChanges(config []ProbeConfig) (toAdd []ProbeConfig, toClose []string) {
	toMap := func(configs []ProbeConfig) map[string]ProbeConfig {
		ret := make(map[string]ProbeConfig)
		for _, probeConfig := range configs {
			ret[probeConfig.Name] = probeConfig
		}
		return ret
	}
	lastConfigMap := toMap(s.lastConfig)
	configMap := toMap(config)

	for name := range lastConfigMap {
		if _, ok := configMap[name]; !ok {
			toClose = append(toClose, name)
		}
	}

	for name, probeConf := range configMap {
		lastConf, ok := lastConfigMap[name]
		if !ok {
			toAdd = append(toAdd, probeConf)
		} else {
			if !reflect.DeepEqual(lastConf, probeConf) {
				toAdd = append(toAdd, probeConf)
				toClose = append(toClose, name)
			}
		}
	}

	return toAdd, toClose
}

func (s *DynamicProbeServer[T]) Start(ctx context.Context, config []ProbeConfig) error {
	return s.Reload(ctx, config)
}

func (s *DynamicProbeServer[T]) Stop(ctx context.Context) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, probe := range s.probes {
		if err := s.probeManager.StopProbe(ctx, probe); err != nil {
			return err
		}
	}
	return nil
}

func marshalProbeConfig(config []ProbeConfig) string {
	s, _ := json.Marshal(config)
	return string(s)
}

func (s *DynamicProbeServer[T]) Reload(ctx context.Context, config []ProbeConfig) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	log.Infof("reload config, old config: %s, new config: %s", marshalProbeConfig(s.lastConfig), marshalProbeConfig(config))
	toAdd, toClose := s.probeChanges(config)
	var toAddProbes []T
	for _, probeConfig := range toAdd {
		probe, err := s.probeManager.CreateProbe(probeConfig)
		if err != nil {
			return fmt.Errorf("error create probe %s: %w", probeConfig.Name, err)
		}
		toAddProbes = append(toAddProbes, probe)
	}

	for _, name := range toClose {
		probe, ok := s.probes[name]
		if !ok {
			continue
		}
		if err := s.probeManager.StopProbe(ctx, probe); err != nil {
			return fmt.Errorf("failed stop probe %s, %w", name, err)
		}
	}

	s.lastConfig = config

	for _, probe := range toAddProbes {
		s.probes[probe.Name()] = probe
		if err := s.probeManager.StartProbe(ctx, probe); err != nil {
			log.Errorf("failed start probe %s, err: %v", probe.Name(), err)
		}
	}

	return nil
}

type probeState struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

func (s *DynamicProbeServer[T]) listProbes() []probeState {
	var ret []probeState
	for name, probe := range s.probes {
		ret = append(ret, probeState{Name: name, State: probe.State().String()})
	}
	return ret
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "/etc/config/config.yaml", "config file path")
}

type inspServerConfig struct {
	DebugMode        bool          `yaml:"debugmode" mapstructure:"debugmode"`
	Port             uint16        `yaml:"port" mapstructure:"port"`
	EnableController bool          `yaml:"enablecontroller" mapstructure:"enablecontroller"`
	MetricsConfig    MetricsConfig `yaml:"metrics" mapstructure:"metrics"`
	EventConfig      EventConfig   `yaml:"event" mapstructure:"event"`
}

type MetricsConfig struct {
	Probes []ProbeConfig `yaml:"probes" mapstructure:"probes"`
}

type EventConfig struct {
	EventSinks []EventSinkConfig `yaml:"sinks" mapstructure:"sinks"`
	Probes     []ProbeConfig     `yaml:"probes" mapstructure:"probes"`
}

type EventSinkConfig struct {
	Name string      `yaml:"name" mapstructure:"name"`
	Args interface{} `yaml:"args" mapstructure:"args"`
}

type ProbeConfig struct {
	Name string                 `yaml:"name" mapstructure:"name"`
	Args map[string]interface{} `yaml:"args" mapstructure:"args"`
}

type inspServer struct {
	v             viper.Viper
	config        inspServerConfig
	ctx           context.Context
	metricsServer *MetricsServer
	eventServer   *EventServer
}

func (i *inspServer) MergeConfig() error {
	err := i.v.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Infof("validate config err: %v", err)
			return fmt.Errorf("config file %s not found", i.v.ConfigFileUsed())
		}
		return fmt.Errorf("config file err: %w", err)
	}

	cfg := &inspServerConfig{}
	err = i.v.Unmarshal(cfg)
	if err != nil {
		return fmt.Errorf("config file err: %w", err)
	}

	i.config = *cfg

	return nil
}

func (i *inspServer) reload() error {
	cfg := inspServerConfig{}
	err := i.v.Unmarshal(&cfg)
	if err != nil {
		return err
	}

	ctx := context.TODO()

	err = i.metricsServer.Reload(ctx, cfg.MetricsConfig.Probes)
	if err != nil {
		return fmt.Errorf("reload metric server error: %s", err)
	}

	err = i.eventServer.Reload(ctx, cfg.EventConfig.Probes)
	if err != nil {
		return fmt.Errorf("reload event server error: %s", err)
	}

	i.config = cfg
	return nil
}

func (i *inspServer) start() error {
	if err := gops.Listen(gops.Options{}); err != nil {
		log.Infof("start gops err: %v", err)
	}

	go func() {
		var err error
		ctx := context.TODO()

		log.Infof("start metrics server")
		i.metricsServer, err = NewMetricsServer()
		if err != nil {
			log.Errorf("failed create metrics server: %v", err)
			return
		}

		defer func() {
			_ = i.metricsServer.Stop(ctx)
		}()

		if err := i.metricsServer.Start(ctx, i.config.MetricsConfig.Probes); err != nil {
			log.Errorf("failed start metrics server: %v", err)
			return
		}

		//sink
		sinks, err := createSink(i.config.EventConfig.EventSinks)
		if err != nil {
			log.Errorf("failed create sinks, err: %v", err)
			return
		}

		log.Infof("start event server")
		//TODO create sinks from config
		i.eventServer, err = NewEventServer(sinks)
		if err != nil {
			log.Errorf("failed create event server: %v", err)
			return
		}

		defer func() {
			_ = i.eventServer.Stop(context.TODO())
		}()

		if err := i.eventServer.Start(ctx, i.config.EventConfig.Probes); err != nil {
			log.Errorf("failed start event server: %v", err)
			return
		}

		http.Handle("/metrics", i.metricsServer)
		http.Handle("/", http.HandlerFunc(defaultPage))
		http.Handle("/config", http.HandlerFunc(i.configPage))
		http.Handle("/status", http.HandlerFunc(i.statusPage))
		if i.config.DebugMode {
			reg := prometheus.NewRegistry()

			reg.MustRegister(
				collectors.NewGoCollector(),
				collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			)
			http.Handle("/internal", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
		}
		listenAddr := fmt.Sprintf(":%d", i.config.Port)
		log.Infof("inspector start metric server, listenAddr: %s", listenAddr)
		srv := &http.Server{Addr: listenAddr}
		if err := srv.ListenAndServe(); err != nil {
			log.Errorf("inspector start metric server err: %v", err)
		}
	}()

	WaitSignals(i, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	return nil
}

func createSink(sinkConfigs []EventSinkConfig) ([]sink.Sink, error) {
	var ret []sink.Sink
	for _, config := range sinkConfigs {
		s, err := sink.CreateSink(config.Name, config.Args)
		if err != nil {
			return nil, fmt.Errorf("failed create sink %s, err: %w", config.Name, err)
		}
		ret = append(ret, s)
	}
	return ret, nil
}

func WaitSignals(i *inspServer, sgs ...os.Signal) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, sgs...)
	sig := <-s
	log.Warnf("recive signal %s, stopping", sig.String())
	if err := i.metricsServer.Stop(i.ctx); err != nil {
		log.Errorf("failed stop metrics server, err: %v", err)
	}
	if err := i.eventServer.Stop(i.ctx); err != nil {
		log.Errorf("failed stop event server, err: %v", err)
	}
}

func defaultPage(w http.ResponseWriter, _ *http.Request) {
	// nolint
	w.Write([]byte(`<html>
		<head><title>Net Exporter</title></head>
		<body>
		<h1>Net Exporter</h1>
		<p><a href="/metrics">Metrics</a></p>
		</body>
		</html>`))
}

func (i *inspServer) configPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rawText, _ := json.MarshalIndent(i.config, " ", "    ")
	w.WriteHeader(http.StatusOK)
	w.Write(rawText) // nolint
}

func (i *inspServer) statusPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	res := map[string]interface{}{
		"inuse_probes": map[string][]probeState{
			"metrics": i.metricsServer.listProbes(),
			"event":   i.eventServer.listProbes(),
		},

		"available_probes": map[string][]string{
			"event":   probe.ListEventProbes(),
			"metrics": probe.ListMetricsProbes(),
		},
	}

	rawText, err := json.Marshal(res)
	if err != nil {
		log.Errorf("failed marshal probe status: %v", err)
	}
	w.Write(rawText) // nolint
}
