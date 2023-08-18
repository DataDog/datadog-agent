// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package compliance implements 'cluster-agent compliance'.
package compliance

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/check"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	complianceCmd := &cobra.Command{
		Use:   "compliance",
		Short: "compliance utility commands",
	}

	bundleParams := core.BundleParams{
		ConfigParams: config.NewClusterAgentParams(""),
		LogParams:    log.LogForOneShot(command.LoggerName, command.DefaultLogLevel, true),
	}

	complianceCmd.AddCommand(check.ClusterAgentCommands(bundleParams)...)

	return []*cobra.Command{complianceCmd}
}
