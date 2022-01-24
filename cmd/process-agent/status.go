package main

import "github.com/spf13/cobra"

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the current status",
	Long:  ``,
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	return nil
}
