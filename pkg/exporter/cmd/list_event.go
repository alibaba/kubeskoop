/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"

	probe2 "github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// eventCmd represents the event command
var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "list all available metrics",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := slog.NewContext(context.Background(), slog.Default())
		if len(listprobe) == 0 {
			listprobe = probe2.ListMetricProbes(ctx, true)
		}

		events := probe2.ListEvents()
		pterm.Print(events)
	},
}

func init() {
	listCmd.AddCommand(eventCmd)
}
