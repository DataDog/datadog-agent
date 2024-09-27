// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package run

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
)

// Params contains the flags of the run subcommand.
type Params struct {
	*subcommands.GlobalParams

	// PIDFilePath contains the value of the --pidfile flag.
	PIDFilePath string
	// CPUProfile contains the value for the --cpu-profile flag.
	CPUProfile string
	// MemProfile contains the value for the --mem-profile flag.
	MemProfile string
}

func setOSSpecificParamFlags(_ *cobra.Command, _ *Params) {}

func runTraceAgentCommand(cliParams *Params, defaultConfPath string) error {
	return runTraceAgentProcess(context.Background(), cliParams, defaultConfPath)
}
