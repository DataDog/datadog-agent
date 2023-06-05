// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package main

import (
	"fmt"
	_ "net/http/pprof"
	"time"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

var elog debug.Log

const (
	// ServiceName is the service name used for the process-agent
	ServiceName  = "datadog-process-agent"
	useWinParams = true
)

type myservice struct {
	globalParams *command.GlobalParams
}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	exit := make(chan struct{})

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
					elog.Info(0x40000006, ServiceName)
					changes <- svc.Status{State: svc.StopPending}
					///// FIXME:  Need a way to indicate to rest of service to shut
					////  down
					close(exit)
					break
				default:
					elog.Warning(0xc000000A, fmt.Sprint(c.Cmd))
				}
			}
		}
	}()
	elog.Info(0x40000003, ServiceName)

	// On Windows, the SCM will require that dependent services stop. This means that when running
	// `Restart-Service datadogagent`, windows will try to stop the Process Agent, and then to be helpful it will immediately start it again.
	// However, if Process Agent is not configured to be running it will exit immediately, which `Restart-Service` will report as an error.
	// To avoid the error on a successful exit we ensure that we are in the RUNNING state long enough for `Restart-Service` or other tools to consider
	// the restart successful.
	exitGate := time.After(5 * time.Second)
	defer func() { <-exitGate }()

	runAgent(m.globalParams, exit)

	changes <- svc.Status{State: svc.Stopped}
	return
}

func runService(globalParams *command.GlobalParams, isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(ServiceName)
	} else {
		elog, err = eventlog.Open(ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	elog.Info(0x40000007, ServiceName)

	service := &myservice{
		globalParams: globalParams,
	}

	err = run(ServiceName, service)
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, ServiceName)
}

func rootCmdRun(globalParams *command.GlobalParams) {
	if !globalParams.WinParams.Foreground {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}

		if !isIntSess {
			runService(globalParams, false)
			return
		}
		// sigh.  Go doesn't have boolean xor operator.  The options are mutually exclusive,
		// make sure more than one wasn't specified
		optcount := 0
		if globalParams.WinParams.StartService {
			optcount++
		}
		if globalParams.WinParams.StopService {
			optcount++
		}
		if optcount > 1 {
			fmt.Println("Incompatible options chosen")
			return
		}
		if globalParams.WinParams.StartService {
			if err = startService(); err != nil {
				fmt.Printf("Error starting service %v\n", err)
			}
			return
		}
		if globalParams.WinParams.StopService {
			if err = stopService(); err != nil {
				fmt.Printf("Error stopping service %v\n", err)
			}
			return
		}
	}

	// Invoke the Agent
	runAgent(globalParams, make(chan struct{}))
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func stopService() error {
	return controlService(svc.Stop, svc.Stopped)
}

func controlService(c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}
