/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// probeCmd represents the probe command
var (
	probeCmd = &cobra.Command{
		Use:   "probe",
		Short: "list supported probe with metric exporting",
		Run: func(cmd *cobra.Command, args []string) {
			res := map[string][]string{
				"metric": {},
				"event":  {},
			}
			res["metric"] = probe.ListMetricProbes()

			els := probe.ListEvents()
			for ep := range els {
				res["event"] = append(res["event"], ep)
			}

			switch output {
			case "json":
				text, err := json.MarshalIndent(res, "", "  ")
				if err != nil {
					slog.Warn("json marshal failed", "err", err)
					return
				}
				fmt.Println(string(text))
			default:
				sliceMapTextOutput("probes", res)
			}
		},
	}
)

func init() {
	listCmd.AddCommand(probeCmd)
}
