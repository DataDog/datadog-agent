// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windows

import (
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Mock implementations

type mockSystemAPI struct {
	mock.Mock
}

func (m *mockSystemAPI) GetServiceProcessID(serviceName string) (uint32, error) {
	args := m.Called(serviceName)
	return args.Get(0).(uint32), args.Error(1)
}

func (m *mockSystemAPI) GetServiceState(serviceName string) (svc.State, error) {
	args := m.Called(serviceName)
	return args.Get(0).(svc.State), args.Error(1)
}

func (m *mockSystemAPI) StopService(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

func (m *mockSystemAPI) StartService(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

func (m *mockSystemAPI) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error) {
	args := m.Called(desiredAccess, inheritHandle, processID)
	return args.Get(0).(windows.Handle), args.Error(1)
}

func (m *mockSystemAPI) TerminateProcess(handle windows.Handle, exitCode uint32) error {
	args := m.Called(handle, exitCode)
	return args.Error(0)
}

func (m *mockSystemAPI) WaitForSingleObject(handle windows.Handle, timeoutMs uint32) (uint32, error) {
	args := m.Called(handle, timeoutMs)
	return args.Get(0).(uint32), args.Error(1)
}

func (m *mockSystemAPI) CloseHandle(handle windows.Handle) error {
	args := m.Called(handle)
	return args.Error(0)
}
