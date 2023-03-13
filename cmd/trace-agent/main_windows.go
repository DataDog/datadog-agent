// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/internal/flags"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

var elog debug.Log

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	ctx, cancelFunc := context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case c := <-r:
				switch c.Cmd {
				case svc.Interrogate:
					changes <- c.CurrentStatus
					// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
					time.Sleep(100 * time.Millisecond)
					changes <- c.CurrentStatus
				case svc.Stop, svc.PreShutdown, svc.Shutdown:
					elog.Info(0x40000006, config.ServiceName)
					changes <- svc.Status{State: svc.StopPending}
					cancelFunc()
					return
				default:
					elog.Warning(0xc000000A, fmt.Sprint(c.Cmd))
				}
			}
		}
	}()
	elog.Info(0x40000003, config.ServiceName)
	Run(ctx)

	changes <- svc.Status{State: svc.Stopped}
	return
}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(config.ServiceName)
	} else {
		elog, err = eventlog.Open(config.ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	elog.Info(0x40000007, config.ServiceName)
	err = run(config.ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, config.ServiceName)
}

// main is the main application entry point
func main() {
	flag.Parse()

	if !flags.Win.Foreground {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			runService(false)
			return
		}
		// sigh.  Go doesn't have boolean xor operator.  The options are mutually exclusive,
		// make sure more than one wasn't specified
		optcount := 0
		if flags.Win.StartService {
			optcount++
		}
		if flags.Win.StopService {
			optcount++
		}
		if optcount > 1 {
			fmt.Println("Incompatible options chosen")
			return
		}
		if flags.Win.StartService {
			if err = startService(); err != nil {
				fmt.Printf("Error starting service %v\n", err)
			}
			return
		}
		if flags.Win.StopService {
			if err = stopService(); err != nil {
				fmt.Printf("Error stopping service %v\n", err)
			}
			return
		}
	}

	// prepare go runtime
	runtime.SetMaxProcs()

	// if we are an interactive session, then just invoke the agent on the command line.
	ctx, cancelFunc := context.WithCancel(context.Background())
	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	// Invoke the Agent
	Run(ctx)
}

func startService() error {
}

func stopService() error {
}
