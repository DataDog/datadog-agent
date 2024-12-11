// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package health implements 'cluster-agent health'.
package health

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/health"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := health.MakeCommand(func() health.GlobalParams {
		return health.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
			ConfigName:   command.ConfigName,
			LoggerName:   command.LoggerName,
		}
	})

	return []*cobra.Command{cmd}
}
