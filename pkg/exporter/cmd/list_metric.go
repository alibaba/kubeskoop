/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// metricCmd represents the metric command
var (
	listmetricCmd = &cobra.Command{
		Use:   "metric",
		Short: "list available metrics of probe",
		Run: func(cmd *cobra.Command, args []string) {
			showprobes := []string{}
			allprobes := probe.ListMetricProbes()
			for idx := range listprobe {
				for _, probe := range allprobes {
					if strings.Contains(probe, listprobe[idx]) {
						showprobes = append(showprobes, probe)
						break
					}
				}
			}

			if len(showprobes) == 0 {
				showprobes = allprobes
			}

			tree := pterm.TreeNode{
				Text:     "metrics",
				Children: []pterm.TreeNode{},
			}
			metrics := probe.ListMetrics()
			for _, p := range showprobes {
				if mnames, ok := metrics[p]; ok {
					parent := pterm.TreeNode{
						Text:     p,
						Children: []pterm.TreeNode{},
					}
					for i := range mnames {
						if !strings.HasPrefix(mnames[i], p) {
							continue
						}
						children := pterm.TreeNode{
							Text: mnames[i],
						}
						parent.Children = append(parent.Children, children)
					}
					tree.Children = append(tree.Children, parent)
				}
			}

			pterm.DefaultTree.WithRoot(tree).Render()

		},
	}

	listprobe []string
)

func init() {
	listCmd.AddCommand(listmetricCmd)

	listmetricCmd.PersistentFlags().StringSliceVarP(&listprobe, "probe", "p", []string{}, "probe to list, default show all available")
}
