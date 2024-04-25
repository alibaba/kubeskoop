/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	task_agent "github.com/alibaba/kubeskoop/pkg/exporter/task-agent"
	"github.com/fsnotify/fsnotify"

	"github.com/alibaba/kubeskoop/pkg/exporter/sink"

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
)

// serverCmd represents the server command
var (
	serverCmd = &cobra.Command{
		Use:   "server",
		Short: "start inspector server",
		Run: func(cmd *cobra.Command, args []string) {
			insp := &inspServer{
				configPath: configPath,
				ctx:        context.Background(),
			}

			log.Infof("start with config file %s", configPath)

			cfg, err := loadConfig(insp.configPath)
			if err != nil {
				log.Errorf("merge config err: %v", err)
				return
			}

			if debug {
				cfg.DebugMode = true
			}

			if cfg.DebugMode {
				log.SetLevel(log.DebugLevel)
			}

			if err = nettop.StartCache(insp.ctx, sidecar); err != nil {
				log.Errorf("failed start cache: %v", err)
				return
			}

			defer nettop.StopCache()

			if cfg.EnableController {
				if err := task_agent.NewTaskAgent().Run(); err != nil {
					log.Errorf("failed start agent: %v", err)
					return
				}
			}
			// block here
			err = insp.start(cfg)
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

type inspServer struct {
	configPath    string
	ctx           context.Context
	metricsServer *MetricsServer
	eventServer   *EventServer
}

func (i *inspServer) WatchConfig(done <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err = watcher.Add(i.configPath); err != nil {
		return err
	}

	var delaying atomic.Bool

	go func() {
		for {
			select {
			case <-watcher.Events:
				if delaying.CompareAndSwap(false, true) {
					time.AfterFunc(1*time.Second, func() {
						delaying.Store(false)
						if err = i.reload(); err != nil {
							log.Errorf("failed reload config %s: %v", i.configPath, err)
						}
					})
				}
			case err = <-watcher.Errors:
				log.Errorf("error watch %s: %v", i.configPath, err)
			case <-done:
				_ = watcher.Close()
				return
			}
		}
	}()

	return nil
}

func (i *inspServer) reload() error {
	cfg, err := loadConfig(i.configPath)
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

	return nil
}

func (i *inspServer) start(cfg *InspServerConfig) error {
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

		if err := i.metricsServer.Start(ctx, cfg.MetricsConfig.Probes); err != nil {
			log.Errorf("failed start metrics server: %v", err)
			return
		}

		//sink
		sinks, err := createSink(cfg.EventConfig.EventSinks)
		if err != nil {
			log.Errorf("failed create sinks, err: %v", err)
		} else if len(sinks) != len(cfg.EventConfig.EventSinks) {
			log.Warnf("expected to create %d sinks , but %d were created", len(cfg.EventConfig.EventSinks), len(sinks))
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

		if err := i.eventServer.Start(ctx, cfg.EventConfig.Probes); err != nil {
			log.Errorf("failed start event server: %v", err)
			return
		}

		http.Handle("/metrics", i.metricsServer)
		http.Handle("/", http.HandlerFunc(defaultPage))
		http.Handle("/status", http.HandlerFunc(i.statusPage))
		if cfg.DebugMode {
			reg := prometheus.NewRegistry()

			reg.MustRegister(
				collectors.NewGoCollector(),
				collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			)
			http.Handle("/internal", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
		}
		listenAddr := fmt.Sprintf(":%d", cfg.Port)
		log.Infof("inspector start metric server, listenAddr: %s", listenAddr)
		srv := &http.Server{Addr: listenAddr}
		if err := srv.ListenAndServe(); err != nil {
			log.Errorf("inspector start metric server err: %v", err)
		}
	}()

	done := make(chan struct{})

	if err := i.WatchConfig(done); err != nil {
		log.Errorf("failed watch config, dynamic load would not work: %v", err)
	}

	WaitSignals(i, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	close(done)

	return nil
}

func createSink(sinkConfigs []EventSinkConfig) ([]sink.Sink, error) {
	var ret []sink.Sink
	for _, config := range sinkConfigs {
		s, err := sink.CreateSink(config.Name, config.Args)
		if err != nil {
			log.Errorf("failed create sink %s, err: %v", config.Name, err)
			continue
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
