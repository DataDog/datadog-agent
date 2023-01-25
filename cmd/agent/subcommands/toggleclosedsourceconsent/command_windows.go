// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package toggleclosedsourceconsent implements 'agent AllowClosedSource'.
package toggleclosedsourceconsent

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	consentcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/toggleclosedsourceconsent"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := consentcmd.MakeCommand(func() consentcmd.GlobalParams {
		return consentcmd.GlobalParams{
			ConfFilePath:   globalParams.ConfFilePath,
			ConfigName:     "datadog",
			LoggerName:     "CORE",
			SettingsClient: common.NewSettingsClient,
		}
	})

	return []*cobra.Command{cmd}
}
