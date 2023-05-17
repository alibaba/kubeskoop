/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"

	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// entityCmd represents the entity command
var (
	entityCmd = &cobra.Command{
		Use:   "entity",
		Short: "List all network entities, including all non-hostnetwork pods and the host itself.",
		Run: func(cmd *cobra.Command, args []string) {
			if LabelSelector != "" {
				slct, err := parseLabelSelector(LabelSelector)
				if err != nil {
					fmt.Printf("parse label %s failed:%s\n", LabelSelector, err.Error())
					return
				}
				listEntities(slct)
			} else {
				listEntities()
			}

		},
	}

	LabelSelector string
)

func init() {
	listCmd.AddCommand(entityCmd)
	entityCmd.PersistentFlags().StringVarP(&LabelSelector, "label", "l", "", "label filter")
}

func listEntities(slct ...selector) {
	err := nettop.SyncNetTopology()
	if err != nil {
		fmt.Printf("sync nettop failed:%s\n", err.Error())
		return
	}

	texts := pterm.TableData{
		{"POD", "APP", "IP", "NAMESPACE", "NETNS", "PID", "NSINUM"},
	}
	ets := nettop.GetAllEntity()
	for _, et := range ets {
		if len(slct) > 0 {
			labelvalue, ok := et.GetLabel(slct[0].key)
			if ok && (slct[0].value == "" || slct[0].value == labelvalue) {
				texts = append(texts, []string{
					et.GetPodName(),
					et.GetAppLabel(),
					et.GetIP(),
					et.GetNetnsMountPoint(),
					et.GetPodNamespace(),
					fmt.Sprintf("%d", et.GetPid()),
					fmt.Sprintf("%d", et.GetNetns()),
				})
			}
		} else {
			texts = append(texts, []string{
				et.GetPodName(),
				et.GetAppLabel(),
				et.GetIP(),
				et.GetNetnsMountPoint(),
				et.GetPodNamespace(),
				fmt.Sprintf("%d", et.GetPid()),
				fmt.Sprintf("%d", et.GetNetns()),
			})
		}
	}
	// nolint
	pterm.DefaultTable.WithHasHeader().WithData(texts).Render()

}

func parseLabelSelector(selector string) (s selector, err error) {
	err = errors.New("invalid label selector")
	ss := strings.Split(selector, "=")
	if len(ss) > 2 {
		return
	}
	s.key = ss[0]
	if len(ss) > 1 {
		s.value = ss[1]
	}

	return s, nil
}

type selector struct {
	key   string
	value string
}
