// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
	"github.com/spf13/cobra"
)

//nolint:revive // TODO(APM) Fix revive linter
type RunParams struct {
	*subcommands.GlobalParams

	// PIDFilePath contains the value of the --pidfile flag.
	PIDFilePath string
	// CPUProfile contains the value for the --cpu-profile flag.
	CPUProfile string
	// MemProfile contains the value for the --mem-profile flag.
	MemProfile string
	// Foreground contains the value for the --foreground flag.
	Foreground bool
	// Debug contains the value for the --debug flag.
	Debug bool
}

func setOSSpecificParamFlags(cmd *cobra.Command, cliParams *RunParams) {
	cmd.PersistentFlags().BoolVarP(&cliParams.Foreground, "foreground", "f", false,
		"runs the trace-agent in the foreground.")
	cmd.PersistentFlags().BoolVarP(&cliParams.Debug, "debug", "d", false,
		"runs the trace-agent in debug mode.")
}

func runTraceAgent(cliParams *RunParams, defaultConfPath string) error {
	if !cliParams.Foreground {
		if servicemain.RunningAsWindowsService() {
			servicemain.Run(&service{cliParams: cliParams, defaultConfPath: defaultConfPath})
			return nil
		}
	}
	return runFx(context.Background(), cliParams, defaultConfPath)
}
