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

// probeCmd represents the probe command
var (
	probeCmd = &cobra.Command{
		Use:   "probe",
		Short: "A brief description of your command",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := slog.NewContext(context.Background(), slog.Default())
			pls := probe.ListMetricProbes(ctx, avail)
			pterm.Println(pls)
		},
	}

	avail bool
)

func init() {
	listCmd.AddCommand(probeCmd)
	probeCmd.PersistentFlags().BoolVarP(&avail, "avail", "a", false, "list all available probes")
}
