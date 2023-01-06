/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// metricCmd represents the metric command
var (
	listmetricCmd = &cobra.Command{
		Use:   "metric",
		Short: "list all available metrics",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := slog.NewContext(context.Background(), slog.Default())
			if len(listprobe) == 0 {
				listprobe = probe.ListMetricProbes(ctx, true)
			}

			metriclist := []string{}
			metrics := probe.ListMetrics()
			for _, p := range listprobe {
				if mnames, ok := metrics[p]; ok {
					metriclist = append(metriclist, mnames...)
				}
			}
			pterm.Print(metriclist)

		},
	}

	listprobe []string
)

func init() {
	listCmd.AddCommand(listmetricCmd)

	listmetricCmd.PersistentFlags().StringSliceVarP(&listprobe, "probe", "p", []string{}, "probe to list, default show all available")
}
