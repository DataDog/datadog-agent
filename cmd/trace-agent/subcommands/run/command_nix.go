// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type RunParams struct {
	*subcommands.GlobalParams

	// PIDFilePath contains the value of the --pidfile flag.
	PIDFilePath string
	// CPUProfile contains the value for the --cpu-profile flag.
	CPUProfile string
	// MemProfile contains the value for the --mem-profile flag.
	MemProfile string
}

func setOSSpecificParamFlags(cmd *cobra.Command, cliParams *RunParams) {}

func runTraceAgent(cliParams *RunParams, defaultConfPath string) error {
	if cliParams.ConfPath == "" {
		cliParams.ConfPath = defaultConfPath
	}
	return fxutil.OneShot(func(cliParams *RunParams, config config.Component) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		return Run(ctx, cliParams, config)
	},
		fx.Supply(cliParams),
		config.Module,
		fx.Supply(coreconfig.NewAgentParamsWithSecrets(cliParams.ConfPath)),
		coreconfig.Module,
	)
}

func Run(ctx context.Context, cliParams *RunParams, config config.Component) error {
	ctx, cancelFunc := context.WithCancel(ctx)

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	return runAgent(ctx, cliParams, config)

}
