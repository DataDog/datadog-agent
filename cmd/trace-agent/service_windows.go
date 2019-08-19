// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows

package main

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "install-service",
		Short: "Installs the trace-agent in the service control manager",
		RunE: func(_ *cobra.Command, _ []string) error {
			return installService()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "uninstall-service",
		Short: "Uninstalls the trace-agent from the service control manager",
		RunE: func(_ *cobra.Command, _ []string) error {
			return removeService()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "start-service",
		Short: "Starts the trace-agent service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return startService()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stop-service",
		Short: "Stops the trace-agent service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return stopService()
		},
	})
}
