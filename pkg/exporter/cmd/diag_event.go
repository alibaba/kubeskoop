/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"
	"os/signal"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

var (
	diagEventCmd = &cobra.Command{
		Use:   "event",
		Short: "diagnose specific event probe",
		Run: func(cmd *cobra.Command, args []string) {
			if len(probeName) == 0 {
				cmd.Help()
				return
			}

			nettop.SyncNetTopology()
			go nettop.StartCache(cmd.Context())
			defer nettop.StopCache()

			for _, p := range probeName {
				pb := probe.GetEventProbe(p)
				if pb == nil {
					slog.Ctx(cmd.Context()).Info("ignore unsupported probe", "probe", p)
					continue
				}

				ch := make(chan proto.RawEvent)
				if err := pb.Register(ch); err != nil {
					slog.Ctx(cmd.Context()).Info("register failed", "err", err, "probe", p)
					continue
				}

				go pb.Start(cmd.Context())
				go func() {
					for evt := range ch {
						ets, err := nettop.GetEntityByNetns(int(evt.Netns))
						if err != nil && ets == nil {
							slog.Ctx(cmd.Context()).Info("ignore event", "err", err, "netns", evt.Netns)
							continue
						}
						pterm.Info.Printf("%s %s %s %s\n", time.Now().Format(time.Stamp), evt.EventType, ets.GetPodName(), evt.EventBody)
					}
				}()
			}

			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)

			<-c
		},
	}
)

func init() {
	diagCmd.AddCommand(diagEventCmd)
	diagEventCmd.PersistentFlags().StringSliceVarP(&probeName, "probe", "p", []string{}, "probe name to diag")
}
