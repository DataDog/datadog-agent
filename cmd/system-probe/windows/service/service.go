// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"

	runcmd "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/run"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// last digit from window_resources/system-probe-msg.mc
	serviceStarted             = 0x40000003
	serviceStopped             = 0x40000004
	altServiceStarted          = 0x40000007
	receivedStopCommand        = 0x4000000b
	receivedShutdownCommand    = 0x4000000c
	agentShutdownStarting      = 0x4000000d
	receivedPreShutdownCommand = 0x40000010

	// errors
	serviceFailed            = 0xc0000008
	unexpectedControlMessage = 0xc000000a
	agentStartFailure        = 0xc000000e
)

var elog debug.Log

type sysprobeWindowService struct{}

func (m *sysprobeWindowService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	defer func() {
		changes <- svc.Status{State: svc.Stopped}
	}()

	if err := runcmd.StartSystemProbeWithDefaults(); err != nil {
		if err == runcmd.ErrNotEnabled {
			changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			return
		}

		log.Errorf("Failed to start system-probe %v", err)
		elog.Error(agentStartFailure, err.Error())
		errno = 1 // indicates non-successful return from handler.
		return
	}

	elog.Info(serviceStarted, config.ServiceName)
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
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
			case svc.Stop:
				log.Info("Received stop message from service control manager")
				elog.Info(receivedStopCommand, config.ServiceName)
				break loop
			case svc.PreShutdown:
				log.Info("Received pre-shutdown message from service control manager")
				elog.Info(receivedPreShutdownCommand, config.ServiceName)
				break loop
			case svc.Shutdown:
				log.Info("Received shutdown message from service control manager")
				elog.Info(receivedShutdownCommand, config.ServiceName)
				break loop
			default:
				log.Warnf("unexpected control request #%d", c)
				elog.Warning(unexpectedControlMessage, fmt.Sprint(c.Cmd))
			}
		case <-signals.Stopper:
			elog.Info(receivedStopCommand, config.ServiceName)
			break loop
		}
	}
	elog.Info(agentShutdownStarting, config.ServiceName)
	log.Infof("Initiating service shutdown")
	changes <- svc.Status{State: svc.StopPending}
	runcmd.StopSystemProbeWithDefaults()
	return
}

// RunService runs the System Probe as a Windows service
func RunService(isDebug bool) {
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
	elog.Info(altServiceStarted, config.ServiceName)
	err = run(config.ServiceName, &sysprobeWindowService{})
	if err != nil {
		elog.Error(serviceFailed, err.Error())
		return
	}
	elog.Info(serviceStopped, config.ServiceName)
}
