// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the current status",
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load(agent.ConfigPath)
		if err != nil {
			return err
		}
		if err := info.InitInfo(cfg); err != nil {
			return err
		}
		return info.Info(os.Stdout, cfg)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
