// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package driver

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	DriverServiceName      = "ddnpm"
	SystemProbeServiceName = "datadog-system-probe"
)

func IsDriverServiceDisabled(driverServiceName string) (enabled bool, err error) {
	return winutil.IsServiceDisabled(driverServiceName)
}

func IsDriverRunning(driverServiceName string) (running bool, err error) {
	log.Infof("Checking if %s is running", driverServiceName)
	return winutil.IsServiceRunning(driverServiceName)
}

func UpdateDriverService(driverServiceName string, newconfig mgr.Config) (err error) {
	return winutil.UpdateServicConfig(driverServiceName, newconfig)
}

func EnableDriverService(driverServiceName string) (err error) {
	newConfig := mgr.Config{
		StartType: windows.SERVICE_DEMAND_START,
	}
	return UpdateDriverService(driverServiceName, newConfig)
}

func StartDriverService(driverServiceName string) (err error) {

	log.Infof("Checking if %s is running", driverServiceName)
	running, err := IsDriverRunning(driverServiceName)
	if err != nil {
		return err
	}

	if running {
		log.Info("Service already running, nothing to do")
		return nil
	}

	log.Infof("Checking if %s is disabled", driverServiceName)
	disabled, err := IsDriverServiceDisabled(driverServiceName)
	if err != nil {
		return err
	}
	if disabled {
		log.Info("%s is disabled, enabling it", driverServiceName)
		newConfig := mgr.Config{StartType: windows.SERVICE_DEMAND_START}
		err = UpdateDriverService(driverServiceName, newConfig)
		if err != nil {
			return err
		}
	}
	servicArgs := []string{}
	log.Info("Starting %s", driverServiceName)
	err = winutil.StartService(driverServiceName, servicArgs...)
	return err
}

func StopDriverService(driverServiceName string, disable bool) (err error) {

	// connect to SCM
	manager, err := winutil.OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer manager.Disconnect()

	// connect to service
	driverAccess := windows.SERVICE_QUERY_STATUS
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
		err := winutil.StopService(driverServiceName)
		if err != nil {
			return fmt.Errorf("Unable to stop service: %v", err)
		}
	}

	// if needed, disable it, too
	if disable {
		config := mgr.Config{StartType: windows.SERVICE_DISABLED}
		err := service.UpdateConfig(config)
		if err != nil {
			return fmt.Errorf("Unable to update config: %v", err)
		}
	}
	return nil
}
