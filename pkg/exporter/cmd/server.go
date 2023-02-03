/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	gops "github.com/google/gops/agent"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
)

// serverCmd represents the server command
var (
	serverCmd = &cobra.Command{
		Use:   "server",
		Short: "start inspector server",
		Run: func(cmd *cobra.Command, args []string) {
			insp := &inspServer{
				v:   *viper.New(),
				ctx: slog.NewContext(context.Background(), slog.Default()),
			}

			insp.v.SetConfigFile(configPath)
			err := insp.MergeConfig()
			if err != nil {
				slog.Ctx(insp.ctx).Info("merge config", "err", err)
				return
			}

			if insp.config.DebugMode {
				opts := slog.HandlerOptions{
					AddSource: true,
					Level:     slog.DebugLevel,
				}
				insp.ctx = slog.NewContext(context.Background(), slog.New(opts.NewJSONHandler(os.Stderr)))
			}

			go nettop.StartCache(insp.ctx)
			defer nettop.StopCache()

			// config hot reload process
			// insp.v.OnConfigChange(func(e fsnotify.Event) {

			// })
			// insp.v.WatchConfig()

			// block here
			err = insp.start()
			if err != nil {
				slog.Ctx(insp.ctx).Info("start server", "err", err)
				return
			}
		},
	}

	configPath = "/etc/config/config.yaml"
)

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "/etc/config/config.yaml", "config file path")
}

type inspServerConfig struct {
	DebugMode bool         `mapstructure:"debugmode"`
	Mconfig   MetricConfig `mapstructure:"metric_config"`
	Econfig   EventConfig  `mapstructure:"event_config"`
}

type MetricConfig struct {
	Interval int      `mapstructure:"interval"`
	Port     int      `mapstructure:"port"`
	Probes   []string `mapstructure:"probes"`
	Verbose  bool     `mapstructure:"verbose"`
}

type EventConfig struct {
	Port        int      `mapstructure:"port"`
	LokiAddress string   `mapstructure:"loki_address"`
	LokiEnable  bool     `mapstructure:"loki_enable"`
	Probes      []string `mapstructure:"probes"`
}

type inspServer struct {
	v      viper.Viper
	config inspServerConfig
	ctx    context.Context
}

func (i *inspServer) MergeConfig() error {
	err := i.v.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Ctx(i.ctx).Info("validate config", "path", configPath, "err", err)
			return errors.Wrapf(err, "no such config")
		}
		slog.Ctx(i.ctx).Info("validate config", "err", err)
		return err
	}

	cfg := &inspServerConfig{}
	err = i.v.Unmarshal(cfg)
	if err != nil {
		slog.Ctx(i.ctx).Info("validate unmarshal config", "err", err)
		return err
	}

	i.config = *cfg

	return nil
}

func (i *inspServer) start() error {
	if err := gops.Listen(gops.Options{}); err != nil {
		slog.Ctx(i.ctx).Info("start gops", "err", err)
	}

	go func() {
		ms := NewMServer(i.ctx, i.config.Mconfig)
		defer ms.Close()

		r := prometheus.NewRegistry()
		r.MustRegister(ms)
		handler := promhttp.HandlerFor(prometheus.Gatherers{
			r,
		}, promhttp.HandlerOpts{})
		http.Handle("/metrics", handler)
		http.Handle("/", http.HandlerFunc(defaulPage))
		http.Handle("/config", http.HandlerFunc(i.configPage))
		listenaddr := fmt.Sprintf(":%d", i.config.Mconfig.Port)
		slog.Ctx(i.ctx).Info("inspector start metric server", "listenaddr", listenaddr)
		srv := &http.Server{Addr: listenaddr}
		if err := srv.ListenAndServe(); err != nil {
			slog.Ctx(i.ctx).Info("inspector start metric server", "err", err, "listenaddr", listenaddr)
		}
	}()

	go func() {
		s := grpc.NewServer()
		e := NewEServer(i.ctx, i.config.Econfig)
		proto.RegisterInspectorServer(s, e)
		listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", i.config.Econfig.Port))
		if err != nil {
			slog.Ctx(i.ctx).Warn("inspector start event server", "port", i.config.Econfig.Port, "err", err)
			return
		}
		slog.Ctx(i.ctx).Info("inspector eserver serve", "port", i.config.Econfig.Port)
		// grpc server block there, handle it with goroutine
		if err := s.Serve(listener); err != nil {
			slog.Ctx(i.ctx).Warn("inspector eserver serve", "port", i.config.Econfig.Port, "err", err)
			return
		}
	}()

	WaitSignals(i.ctx, syscall.SIGHUP, syscall.SIGINT)
	return nil
}

func WaitSignals(ctx context.Context, sgs ...os.Signal) {
	slog.Ctx(ctx).Info("keep running and start waiting for signals")
	s := make(chan os.Signal, 1)
	signal.Notify(s, sgs...)
	<-s
}

func defaulPage(w http.ResponseWriter, _ *http.Request) {
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
