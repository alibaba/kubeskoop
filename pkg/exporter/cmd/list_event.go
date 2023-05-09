/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// eventCmd represents the event command
var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "list all available metrics",
	Run: func(cmd *cobra.Command, args []string) {
		events := probe.ListEvents()

		sliceMapTextOutput("events", events)
	},
}

func init() {
	listCmd.AddCommand(eventCmd)
}

func sliceMapTextOutput(title string, data map[string][]string) {
	tree := pterm.TreeNode{
		Text:     title,
		Children: []pterm.TreeNode{},
	}

	for p, unit := range data {
		parent := pterm.TreeNode{
			Text:     p,
			Children: []pterm.TreeNode{},
		}
		for i := range unit {
			parent.Children = append(parent.Children, pterm.TreeNode{Text: unit[i]})
		}
		tree.Children = append(tree.Children, parent)
	}
	pterm.DefaultTree.WithRoot(tree).Render()
}
