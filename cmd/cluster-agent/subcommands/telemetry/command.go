// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package telemetry implements 'cluster-agent telemetry'.
package telemetry

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	clusterAgentFlare "github.com/DataDog/datadog-agent/pkg/flare/clusteragent"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
//
//nolint:revive // TODO(CINT) Fix revive linter
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Print the telemetry metrics exposed by the cluster agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := clusterAgentFlare.QueryDCAMetrics()
			if err != nil {
				return err
			}
			fmt.Print(string(payload))
			return nil
		},
	}

	return []*cobra.Command{cmd}
}
