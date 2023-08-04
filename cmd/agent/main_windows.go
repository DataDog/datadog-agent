// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	_ "expvar"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

func runService(ctx context.Context) error {
	_ = common.CheckAndUpgradeConfig()
	// ignore config upgrade error, continue running with what we have.
	return run.StartAgentWithDefaults(ctx)
}

func main() {
	common.EnableLoggingToFile()
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 && servicemain.RunningAsWindowsService() {
		servicemain.RunAsWindowsService(common.ServiceName, runService)
		return
	}
	defer log.Flush()

	os.Exit(runcmd.Run(command.MakeCommand(subcommands.AgentSubcommands())))
}
