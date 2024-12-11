// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package flare implements 'cluster-agent flare'.
package flare

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/dcaflare"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := dcaflare.MakeCommand(func() dcaflare.GlobalParams {
		return dcaflare.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
		}
	})

	return []*cobra.Command{cmd}
}
