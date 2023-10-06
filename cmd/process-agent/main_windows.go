// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/messagestrings"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"

	"golang.org/x/sys/windows/svc"
)

const (
	// ServiceName is the service name used for the process-agent
	ServiceName  = "datadog-process-agent"
	useWinParams = true
)

type service struct {
	globalParams *command.GlobalParams
}

func (s *service) Name() string {
	return ServiceName
}

func (s *service) Init() error {
	// Nothing to do, kept empty for compatibility with previous implementation.
	return nil
}

func (s *service) Run(ctx context.Context) error {
	err := runAgent(ctx, s.globalParams)
	if err != nil {
		// For compatibility with the previous cleanupAndExitHandler implementation, call os.Exit() on error.
		// Since we won't be returning, SCM will put the service into failure/recovery and automatically restart the service.
		// If this behavior is no longer desired then simply return the error and let SERVICE_STOPPED be set.
		winutil.LogEventViewer(ServiceName, messagestrings.MSG_SERVICE_FAILED, err.Error())
		os.Exit(1)
	}

	return err
}

func rootCmdRun(globalParams *command.GlobalParams) {
	var err error

	if !globalParams.WinParams.Foreground {
		if servicemain.RunningAsWindowsService() {
			servicemain.Run(&service{globalParams: globalParams})
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
			if err = winutil.StartService(ServiceName); err != nil {
				fmt.Printf("Error starting service %v\n", err)
			}
			return
		}
		if globalParams.WinParams.StopService {
			if err = winutil.StopService(ServiceName); err != nil {
				fmt.Printf("Error stopping service %v\n", err)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err = winutil.WaitForState(ctx, ServiceName, svc.Stopped); err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						fmt.Printf("Timed out waiting for %s service to stop", ServiceName)
					} else {
						fmt.Printf("Error stopping service %v\n", err)
					}
				}
			}
			return
		}
	}

	// Invoke the Agent
	err = runAgent(context.Background(), globalParams)
	if err != nil {
		// For compatibility with the previous cleanupAndExitHandler implementation, os.Exit() on error.
		// This prevents runcmd.Run() from displaying the error.
		os.Exit(1)
	}
}
