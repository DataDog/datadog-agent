// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package driver

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	driverServiceName = "ddnpm"
)

func isDriverServiceDisabled(driverServiceName string) (enabled bool, err error) {
	return winutil.IsServiceDisabled(driverServiceName)
}

func isDriverRunning(driverServiceName string) (running bool, err error) {
	log.Debugf("Checking if %s is running", driverServiceName)
	return winutil.IsServiceRunning(driverServiceName)
}

func enableDriverService(driverServiceName string) (err error) {
	// connect to SCM
	manager, err := winutil.OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer manager.Disconnect()

	// connect to service
	driverAccess := windows.SERVICE_QUERY_STATUS | windows.SERVICE_QUERY_CONFIG | windows.SERVICE_CHANGE_CONFIG
	service, err := winutil.OpenService(manager, driverServiceName, uint32(driverAccess))
	if err != nil {
		return err
	}
	defer service.Close()

	noChange := uint32(windows.SERVICE_NO_CHANGE)
	err = windows.ChangeServiceConfig(service.Handle, noChange, windows.SERVICE_DEMAND_START, noChange, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("Unable to update config: %v", err)
	}
	return nil
}

func startDriverService(driverServiceName string) (err error) {
	running, err := isDriverRunning(driverServiceName)
	if err != nil {
		return err
	}

	if running {
		log.Debugf("Service %s already running, nothing to do", driverServiceName)
		return nil
	}

	log.Debugf("Checking if %s is disabled", driverServiceName)
	disabled, err := isDriverServiceDisabled(driverServiceName)
	if err != nil {
		return err
	}
	if disabled {
		log.Debugf("%s is disabled, enabling it", driverServiceName)
		err = enableDriverService(driverServiceName)
		if err != nil {
			return err
		}
	}
	var serviceArgs []string
	log.Infof("Starting %s", driverServiceName)
	return winutil.StartService(driverServiceName, serviceArgs...)
}

func stopDriverService(driverServiceName string, disable bool) (err error) {
	// connect to SCM
	manager, err := winutil.OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer manager.Disconnect()

	// connect to service
	driverAccess := windows.SERVICE_QUERY_STATUS | windows.SERVICE_QUERY_CONFIG | windows.SERVICE_CHANGE_CONFIG
	service, err := winutil.OpenService(manager, driverServiceName, uint32(driverAccess))
	if err != nil {
		return err
	}
	defer service.Close()

	// check if running already
	status, err := service.Query()
	if err != nil {
		return err
	}

	// already issued a STOP command
	if status.State == windows.SERVICE_STOP_PENDING {
		return nil
	}

	// RUNNING, so stop it
	if status.State == windows.SERVICE_RUNNING {
		log.Infof("Stopping %s service via SCM", driverServiceName)
		err := winutil.StopService(driverServiceName)
		if err != nil {
			return fmt.Errorf("Unable to stop service: %v", err)
		}
	}

	// if needed, disable it too
	if disable {
		noChange := uint32(windows.SERVICE_NO_CHANGE)
		log.Infof("Setting %s service to disabled", driverServiceName)
		err := windows.ChangeServiceConfig(service.Handle, noChange, windows.SERVICE_DISABLED, noChange, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			return fmt.Errorf("Unable to update config: %v", err)
		}
	}
	return nil
}
