// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windows

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// systemAPI abstracts system-level API calls
type systemAPI interface {
	GetServiceProcessID(serviceName string) (uint32, error)
	GetServiceState(serviceName string) (svc.State, error)
	StopService(serviceName string) error
	StartService(serviceName string) error
	OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error)
	TerminateProcess(handle windows.Handle, exitCode uint32) error
	WaitForSingleObject(handle windows.Handle, timeoutMs uint32) (uint32, error)
	CloseHandle(handle windows.Handle) error
}

// Real implementations of the interfaces

// winSystemAPI implements SystemAPI using winutil and Windows API
type winSystemAPI struct{}

func (api *winSystemAPI) queryServiceStatus(serviceName string) (svc.Status, error) {
	manager, err := winutil.OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return svc.Status{}, err
	}
	defer manager.Disconnect()

	service, err := winutil.OpenService(manager, serviceName, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return svc.Status{}, err
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return svc.Status{}, fmt.Errorf("could not query service %s: %w", serviceName, err)
	}

	return status, nil
}

func (api *winSystemAPI) GetServiceProcessID(serviceName string) (uint32, error) {
	status, err := api.queryServiceStatus(serviceName)
	if err != nil {
		return 0, err
	}

	return status.ProcessId, nil
}

// GetServiceState returns the current state of the service.
func (api *winSystemAPI) GetServiceState(serviceName string) (svc.State, error) {
	status, err := api.queryServiceStatus(serviceName)
	if err != nil {
		return svc.Stopped, err
	}

	return status.State, nil
}

func (api *winSystemAPI) StopService(serviceName string) error {
	return winutil.StopService(serviceName)
}

func (api *winSystemAPI) StartService(serviceName string) error {
	return winutil.StartService(serviceName)
}

func (api *winSystemAPI) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error) {
	handle, err := windows.OpenProcess(desiredAccess, inheritHandle, processID)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

func (api *winSystemAPI) TerminateProcess(handle windows.Handle, exitCode uint32) error {
	return windows.TerminateProcess(handle, exitCode)
}

func (api *winSystemAPI) WaitForSingleObject(handle windows.Handle, timeoutMs uint32) (uint32, error) {
	return windows.WaitForSingleObject(handle, timeoutMs)
}

func (api *winSystemAPI) CloseHandle(handle windows.Handle) error {
	return windows.CloseHandle(handle)
}
