/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/spf13/cobra"
)

// probeCmd represents the probe command
var (
	probeCmd = &cobra.Command{
		Use:   "probe",
		Short: "list supported probe with metric exporting",
		Run: func(cmd *cobra.Command, args []string) {
			res := make(map[string][]string)
			res["metrics"] = probe.ListMetricsProbes()
			res["event"] = probe.ListEventProbes()

			for key, l := range res {
				fmt.Println(key)
				indent := "    "
				for _, s := range l {
					fmt.Printf("%s%s\n", indent, s)

				}
			}
		},
	}
)

func init() {
	listCmd.AddCommand(probeCmd)
}
