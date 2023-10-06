// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements 'agent config'.
package config

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	configcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/config"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := configcmd.MakeCommand(func() configcmd.GlobalParams {
		return configcmd.GlobalParams{
			ConfFilePath:   globalParams.ConfFilePath,
			ConfigName:     command.ConfigName,
			LoggerName:     command.LoggerName,
			SettingsClient: common.NewSettingsClient,
		}
	})

	return []*cobra.Command{cmd}
}
