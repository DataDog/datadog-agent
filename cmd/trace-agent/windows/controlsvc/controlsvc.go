// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

// Package controlsvc contains shared code for controlling the Windows agent service.
package controlsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows/svc"
)

// StartService starts the agent service via the Service Control Manager
func StartService() error {
	return winutil.StartService(config.ServiceName)
}

// RestartService restarts the agent service by calling StopService and StartService
func RestartService() error {
	err := StopService()
	if err == nil {
		err = StartService()
	}
	return err
}

// StopService stops the agent service via the Service Control Manager
func StopService() error {
	var err error
	if err = winutil.StopService(config.ServiceName); err != nil {
		return fmt.Errorf("error stopping service %v", err)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err = winutil.WaitForState(ctx, config.ServiceName, svc.Stopped); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timed out waiting for %s service to stop", config.ServiceName)
			} else {
				return fmt.Errorf("error stopping service %v", err)
			}
		}
	}
	return nil
}
