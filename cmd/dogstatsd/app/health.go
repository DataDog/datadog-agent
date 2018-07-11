// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/api"
	statusapi "github.com/DataDog/datadog-agent/pkg/status/api"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

func init() {
	DogstatsdCmd.AddCommand(healthCmd)
}

var healthCmd = &cobra.Command{
	Use:          "health",
	Short:        "Print the current dogstatsd health",
	Long:         ``,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := new(health.Status)
		err := api.RetrieveJSON("/dogstatsd/status/health", s)
		if err != nil {
			return err
		}
		return statusapi.PrintHealth(s, "Dogstatsd")
	},
}
