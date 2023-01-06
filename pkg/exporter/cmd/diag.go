/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// diagCmd represents the diag command
var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("diag called")
	},
}

func init() {
	rootCmd.AddCommand(diagCmd)
}
