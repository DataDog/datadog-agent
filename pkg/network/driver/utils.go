// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package driver

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	AgentRegistryKey       = `SOFTWARE\DataDog\Datadog Agent`
	ClosedSourceKeyName    = "AllowClosedSource"
	ClosedSourceAllowed    = 1
	ClosedSourceNotAllowed = 0
	DriverServiceName      = "ddnpm"
	SystemProbeServiceName = "datadog-system-probe"
)

func IsClosedSourceAllowed() bool {
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		log.Warnf("unable to open registry key %s: %v", AgentRegistryKey, err)
		return false
	}
	defer regKey.Close()

	val, _, err := regKey.GetIntegerValue(ClosedSourceKeyName)
	if err != nil {
		log.Warnf("unable to get value for %s: %v", ClosedSourceKeyName, err)
		return false
	}

	if val == ClosedSourceAllowed {
		log.Info("closed-source software allowed")
		return true
	} else if val == ClosedSourceNotAllowed {
		log.Info("closed-source software not allowed")
		return false
	} else {
		log.Infof("unexpected value set for %s: %d", ClosedSourceKeyName, val)
		return false
	}
}

func IsDriverEnabled(driverServiceName string) (enabled bool, err error) {
	return winutil.IsServiceEnabled(driverServiceName)
}

func IsDriverRunning(driverServiceName string) (running bool, err error) {
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
	return winutil.StartService(driverServiceName)
}
