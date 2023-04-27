// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

// Package closedsourceconsent implements 'agent closedsourceconsent set [true|false]'.
package closedsourceconsent

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	consentcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/closedsourceconsent"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := consentcmd.MakeCommand(func() consentcmd.GlobalParams {
		return consentcmd.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
			ConfigName:   command.ConfigName,
			LoggerName:   command.LoggerName,
		}
	})

	return []*cobra.Command{cmd}
}
