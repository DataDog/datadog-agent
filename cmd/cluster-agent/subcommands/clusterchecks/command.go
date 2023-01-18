// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

// Package clusterchecks implements 'cluster-agent clusterchecks'.
package clusterchecks

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/clusterchecks"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := clusterchecks.MakeCommand(func() clusterchecks.GlobalParams {
		return clusterchecks.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
		}
	})

	return []*cobra.Command{cmd}
}
