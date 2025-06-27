//go:build windows

package windows

import (
	"github.com/stretchr/testify/mock"
)

// Mock implementations

type mockSystemAPI struct {
	mock.Mock
}

func (m *mockSystemAPI) GetServiceProcessID(serviceName string) (uint32, error) {
	args := m.Called(serviceName)
	return args.Get(0).(uint32), args.Error(1)
}

func (m *mockSystemAPI) IsServiceRunning(serviceName string) (bool, error) {
	args := m.Called(serviceName)
	return args.Bool(0), args.Error(1)
}

func (m *mockSystemAPI) StopService(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

func (m *mockSystemAPI) StartService(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

func (m *mockSystemAPI) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (processHandle, error) {
	args := m.Called(desiredAccess, inheritHandle, processID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(processHandle), args.Error(1)
}

func (m *mockSystemAPI) TerminateProcess(handle processHandle, exitCode uint32) error {
	args := m.Called(handle, exitCode)
	return args.Error(0)
}

func (m *mockSystemAPI) WaitForSingleObject(handle processHandle, timeoutMs uint32) (uint32, error) {
	args := m.Called(handle, timeoutMs)
	return args.Get(0).(uint32), args.Error(1)
}

func (m *mockSystemAPI) CloseHandle(handle processHandle) error {
	args := m.Called(handle)
	return args.Error(0)
}

type mockProcessHandle struct {
	mock.Mock
}
