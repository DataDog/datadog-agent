// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//go:generate go run ../../pkg/config/render_config.go agent ../../pkg/config/config_template.yaml ./dist/datadog.yaml

package main

import (
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

var elog debug.Log

func main() {
	common.EnableLoggingToFile()
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v", err)
		}
		if !isIntSess {
			common.EnableLoggingToFile()
			runService(false)
			return
		}
	}
	defer log.Flush()

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	if err := common.ImportRegistryConfig(); err != nil {
		elog.Warning(0x80000001, err.Error())
		// continue running agent with existing config
	}
	if err := common.CheckAndUpgradeConfig(); err != nil {
		elog.Warning(0x80000002, err.Error())
		// continue running with what we have.
	}
	app.StartAgent()
	elog.Info(0x40000003, app.ServiceName)
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				app.StopAgent()
				break loop
			default:
				log.Warnf("unexpected control request #%d", c)
				elog.Warning(0xc0000009, string(c.Cmd))
			}
		case <-signals.Stopper:
			elog.Info(0x4000000a, app.ServiceName)
			app.StopAgent()
			break loop

		}
	}
	elog.Info(1, fmt.Sprintf("prestopping %s service", app.ServiceName))
	changes <- svc.Status{State: svc.StopPending}
	return
}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(app.ServiceName)
	} else {
		elog, err = eventlog.Open(app.ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	elog.Info(0x40000007, app.ServiceName)
	run := svc.Run

	err = run(app.ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, app.ServiceName)
}
