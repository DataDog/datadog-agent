// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"os"

	"golang.org/x/sys/windows/svc"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/agent/windows/service"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	// set the Agent flavor
	flavor.SetFlavor(flavor.IotAgent)

	common.EnableLoggingToFile()
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			common.EnableLoggingToFile()
			service.RunService(false)
			return
		}
	}
	defer log.Flush()

	if err := command.MakeCommand(subcommands.AgentSubcommands()).Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
