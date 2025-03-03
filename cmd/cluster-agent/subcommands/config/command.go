// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package config implements 'cluster-agent config'.
package config

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/config"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := config.MakeCommand(func() config.GlobalParams {
		return config.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
			ConfigName:   command.ConfigName,
			LoggerName:   command.LoggerName,
		}
	})

	return []*cobra.Command{cmd}
}
