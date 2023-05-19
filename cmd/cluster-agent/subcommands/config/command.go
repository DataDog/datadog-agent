// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package config implements 'cluster-agent config'.
package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := config.MakeCommand(func() config.GlobalParams {
		return config.GlobalParams{
			ConfFilePath:   globalParams.ConfFilePath,
			ConfigName:     command.ConfigName,
			LoggerName:     command.LoggerName,
			SettingsClient: newSettingsClient,
		}
	})

	return []*cobra.Command{cmd}
}

func newSettingsClient() (settings.Client, error) {
	c := util.GetClient(false)

	apiConfigURL := fmt.Sprintf(
		"https://localhost:%v/config",
		pkgconfig.Datadog.GetInt("cluster_agent.cmd_port"),
	)

	return settingshttp.NewClient(c, apiConfigURL, "datadog-cluster-agent"), nil
}
