/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"strings"

	nettop2 "github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// metricCmd represents the tcp command
var (
	metricCmd = &cobra.Command{
		Use:   "metric",
		Short: "A brief description of your command",
		Run: func(cmd *cobra.Command, args []string) {
			if len(probeName) > 0 {
				ctx := slog.NewContext(context.Background(), slog.Default())
				err := nettop2.SyncNetTopology()
				if err != nil {
					slog.Ctx(ctx).Info("sync nettop", "err", err)
					return
				}
				texts := pterm.TableData{
					{"METRIC", "VALUE", "NETNS", "POD", "NAMESPACE", "PROBE"},
				}
				for _, p := range probeName {
					data, err := probe.CollectOnce(ctx, p)
					if err != nil && data == nil {
						slog.Ctx(ctx).Info("collect metric", "err", err)
						continue
					}
					for m, d := range data {
						slog.Ctx(ctx).Debug("raw metric msg", "metric", m, "data", d)
						// if a probe provide multi subject, only fetch relevant metric data
						if !strings.HasPrefix(m, p) {
							continue
						}
						for nsinum, v := range d {
							et, err := nettop2.GetEntityByNetns(int(nsinum))
							if err != nil {
								slog.Ctx(ctx).Info("get entity failed", "netns", nsinum, "err", err)
								continue
							}
							texts = append(texts, []string{
								m,
								fmt.Sprintf("%d", v),
								fmt.Sprintf("%d", nsinum),
								et.GetPodName(),
								et.GetPodNamespace(),
								p,
							})
						}

					}
				}
				pterm.DefaultTable.WithHasHeader().WithData(texts).Render() // nolint
			}
		},
	}

	probeName []string
)

func init() {
	diagCmd.AddCommand(metricCmd)
	metricCmd.PersistentFlags().StringSliceVarP(&probeName, "probe", "p", []string{}, "probe name to diag")
}
