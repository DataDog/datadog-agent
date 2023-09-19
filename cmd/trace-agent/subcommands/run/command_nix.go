// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package run

import (
	"github.com/DataDog/datadog-agent/comp/trace/agent"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
)

type RunParams struct {
	*subcommands.GlobalParams
	*agent.Params
}

func setOSSpecificParamFlags(cmd *cobra.Command, cliParams *RunParams) {}

func Run(cliParams *RunParams, defaultConfPath string) error {
	return runFx(cliParams, defaultConfPath)
}
