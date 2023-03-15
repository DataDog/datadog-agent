// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"
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

func Start(cliParams *RunParams, config config.Component) error {
	// Entrypoint here

	ctx, cancelFunc := context.WithCancel(context.Background())

	// prepare go runtime
	runtime.SetMaxProcs()
	if err := runtime.SetGoMemLimit(pkgconfig.IsContainerized()); err != nil {
		log.Debugf("Couldn't set Go memory limit: %s", err)
	}

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	return runAgent(ctx, cliParams, config)

}
