/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

// entityCmd represents the entity command
var (
	entityCmd = &cobra.Command{
		Use:   "entity",
		Short: "list network entity, aka no-hostNetwork pod",
		Run: func(cmd *cobra.Command, args []string) {
			listEntities()
		},
	}
)

func init() {
	listCmd.AddCommand(entityCmd)
}

func listEntities() {
	err := nettop.SyncNetTopology()
	if err != nil {
		slog.Warn("sync nettop", "err", err)
		return
	}

	ets := nettop.GetAllEntity()
	if verbose {
		texts := pterm.TableData{
			{"POD", "APP", "IP", "NAMESPACE", "NETNS", "PID", "SANDBOX", "NSINUM"},
		}
		for _, et := range ets {
			texts = append(texts, []string{
				et.GetPodName(),
				et.GetAppLabel(),
				et.GetIP(),
				et.GetNetnsMountPoint(),
				et.GetPodNamespace(),
				fmt.Sprintf("%d", et.GetPid()),
				et.GetPodSandboxId(), // nolint
				fmt.Sprintf("%d", et.GetNetns()),
			})
		}
		pterm.DefaultTable.WithHasHeader().WithData(texts).Render() // nolint
	} else {
		texts := pterm.TableData{
			{"NETNS", "POD", "NAMESPACE", "PID"},
		}
		for _, et := range ets {
			texts = append(texts, []string{
				et.GetNetnsMountPoint(),
				et.GetPodName(),
				et.GetPodNamespace(),
				fmt.Sprintf("%d", et.GetPid()),
			})
		}
		pterm.DefaultTable.WithHasHeader().WithData(texts).Render() // nolint
	}

}
