//go:build windows

package packages

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/windows"
)

// Mock implementations

type MockSystemAPI struct {
	mock.Mock
}

func (m *MockSystemAPI) GetServiceProcessID(serviceName string) (uint32, error) {
	args := m.Called(serviceName)
	return args.Get(0).(uint32), args.Error(1)
}

func (m *MockSystemAPI) IsServiceRunning(serviceName string) (bool, error) {
	args := m.Called(serviceName)
	return args.Bool(0), args.Error(1)
}

func (m *MockSystemAPI) StopService(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

func (m *MockSystemAPI) StartService(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

func (m *MockSystemAPI) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (ProcessHandle, error) {
	args := m.Called(desiredAccess, inheritHandle, processID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ProcessHandle), args.Error(1)
}

func (m *MockSystemAPI) TerminateProcess(handle ProcessHandle, exitCode uint32) error {
	args := m.Called(handle, exitCode)
	return args.Error(0)
}

func (m *MockSystemAPI) WaitForSingleObject(handle ProcessHandle, timeoutMs uint32) (uint32, error) {
	args := m.Called(handle, timeoutMs)
	return args.Get(0).(uint32), args.Error(1)
}

func (m *MockSystemAPI) CloseHandle(handle ProcessHandle) error {
	args := m.Called(handle)
	return args.Error(0)
}

type MockProcessHandle struct {
	mock.Mock
}

// Test functions

func TestWinServiceManager_TerminateServiceProcess(t *testing.T) {
	tests := []struct {
		name          string
		serviceName   string
		expectError   bool
		errorContains string
		setupMocks    func(*MockSystemAPI, *MockProcessHandle)
	}{
		{
			name:        "service not running",
			serviceName: "testservice",
			expectError: false,
			setupMocks: func(api *MockSystemAPI, proc *MockProcessHandle) {
				api.On("GetServiceProcessID", "testservice").Return(uint32(0), nil)
			},
		},
		{
			name:        "service does not exist",
			serviceName: "nonexistent",
			expectError: false,
			setupMocks: func(api *MockSystemAPI, proc *MockProcessHandle) {
				api.On("GetServiceProcessID", "nonexistent").Return(uint32(0), windows.ERROR_SERVICE_DOES_NOT_EXIST)
			},
		},
		{
			name:        "successful termination",
			serviceName: "testservice",
			expectError: false,
			setupMocks: func(api *MockSystemAPI, proc *MockProcessHandle) {
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(uint32(1234), nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, uint32(1234)).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (same PID)
				api.On("GetServiceProcessID", "testservice").Return(uint32(1234), nil).Once()
				// Terminate and wait
				api.On("TerminateProcess", proc, uint32(1)).Return(nil)
				api.On("WaitForSingleObject", proc, mock.Anything).Return(uint32(windows.WAIT_OBJECT_0), nil)
			},
		},
		{
			name:          "process termination fails",
			serviceName:   "testservice",
			expectError:   true,
			errorContains: "could not terminate process",
			setupMocks: func(api *MockSystemAPI, proc *MockProcessHandle) {
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(uint32(1234), nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, uint32(1234)).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (same PID)
				api.On("GetServiceProcessID", "testservice").Return(uint32(1234), nil).Once()
				// Terminate fails
				api.On("TerminateProcess", proc, uint32(1)).Return(errors.New("access denied"))
			},
		},
		{
			name:          "service PID changes during termination",
			serviceName:   "testservice",
			expectError:   true,
			errorContains: "process ID for service testservice changed from 1234 to 5678, aborting termination",
			setupMocks: func(api *MockSystemAPI, proc *MockProcessHandle) {
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(uint32(1234), nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, uint32(1234)).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (PID has changed - race condition)
				api.On("GetServiceProcessID", "testservice").Return(uint32(5678), nil).Once()
				// Should not proceed with termination since PID changed
			},
		},
		{
			name:          "process termination succeeds but wait times out",
			serviceName:   "testservice",
			expectError:   true,
			errorContains: "timeout waiting for process 1234 for service testservice to exit",
			setupMocks: func(api *MockSystemAPI, proc *MockProcessHandle) {
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(uint32(1234), nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, uint32(1234)).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (same PID)
				api.On("GetServiceProcessID", "testservice").Return(uint32(1234), nil).Once()
				// Terminate succeeds
				api.On("TerminateProcess", proc, uint32(1)).Return(nil)
				// Wait times out
				api.On("WaitForSingleObject", proc, mock.Anything).Return(uint32(windows.WAIT_TIMEOUT), nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &MockSystemAPI{}
			mockProc := &MockProcessHandle{}

			tt.setupMocks(mockAPI, mockProc)

			manager := NewWinServiceManagerWithAPI(mockAPI)

			ctx := context.Background()
			err := manager.TerminateServiceProcess(ctx, tt.serviceName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockAPI.AssertExpectations(t)
			mockProc.AssertExpectations(t)
		})
	}
}

func TestWinServiceManager_StopAllAgentServices(t *testing.T) {
	tests := []struct {
		name          string
		expectError   bool
		errorContains string
		setupMocks    func(*MockSystemAPI)
	}{
		{
			name:        "main service does not exist",
			expectError: false,
			setupMocks: func(api *MockSystemAPI) {
				api.On("StopService", "datadogagent").Return(windows.ERROR_SERVICE_DOES_NOT_EXIST)
			},
		},
		{
			name:        "main service stop fails but continues with termination loop",
			expectError: false,
			setupMocks: func(api *MockSystemAPI) {
				// Initial StopService fails with a generic error (not "does not exist")
				api.On("StopService", "datadogagent").Return(errors.New("access denied"))

				// Function should continue and check all services in termination loop
				serviceNames := []string{
					"datadog-trace-agent",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-system-probe",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range serviceNames {
					api.On("IsServiceRunning", name).Return(false, nil)
				}
			},
		},
		{
			name:        "successful stop all services",
			expectError: false,
			setupMocks: func(api *MockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// All services are not running
				serviceNames := []string{
					"datadog-trace-agent",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-system-probe",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range serviceNames {
					api.On("IsServiceRunning", name).Return(false, nil)
				}
			},
		},
		{
			name:        "successful termination of running services",
			expectError: false,
			setupMocks: func(api *MockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// Two services are running and need termination
				runningServices := []string{"datadog-trace-agent", "datadog-system-probe"}
				for _, serviceName := range runningServices {
					// Service is initially running
					api.On("IsServiceRunning", serviceName).Return(true, nil).Once()

					// Successful termination process
					api.On("GetServiceProcessID", serviceName).Return(uint32(1234), nil).Once() // First call
					api.On("OpenProcess", mock.Anything, false, uint32(1234)).Return(&MockProcessHandle{}, nil)
					api.On("CloseHandle", &MockProcessHandle{}).Return(nil)
					api.On("GetServiceProcessID", serviceName).Return(uint32(1234), nil).Once() // Verification call
					api.On("TerminateProcess", &MockProcessHandle{}, uint32(1)).Return(nil)
					api.On("WaitForSingleObject", &MockProcessHandle{}, mock.Anything).Return(uint32(windows.WAIT_OBJECT_0), nil)
				}

				// Other services are not running
				otherServices := []string{
					"datadog-process-agent",
					"datadog-security-agent",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range otherServices {
					api.On("IsServiceRunning", name).Return(false, nil)
				}
			},
		},
		{
			name:          "some services running but termination fails",
			expectError:   true,
			errorContains: "failed to stop services",
			setupMocks: func(api *MockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// First service is running but termination fails
				api.On("IsServiceRunning", "datadog-trace-agent").Return(true, nil)
				// Mock the GetServiceProcessID calls within TerminateServiceProcess
				api.On("GetServiceProcessID", "datadog-trace-agent").Return(uint32(1234), errors.New("access denied"))

				// Other services are not running
				serviceNames := []string{
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-system-probe",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range serviceNames {
					api.On("IsServiceRunning", name).Return(false, nil)
				}
				// Still check if first service is running after failed termination
				api.On("IsServiceRunning", "datadog-trace-agent").Return(true, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &MockSystemAPI{}
			tt.setupMocks(mockAPI)

			manager := NewWinServiceManagerWithAPI(mockAPI)

			ctx := context.Background()
			err := manager.StopAllAgentServices(ctx)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockAPI.AssertExpectations(t)
		})
	}
}

func TestWinServiceManager_StartAndRestartServices(t *testing.T) {
	t.Run("StartAgentServices success", func(t *testing.T) {
		mockAPI := &MockSystemAPI{}
		mockAPI.On("StartService", "datadogagent").Return(nil)

		manager := NewWinServiceManagerWithAPI(mockAPI)

		ctx := context.Background()
		err := manager.StartAgentServices(ctx)

		assert.NoError(t, err)
		mockAPI.AssertExpectations(t)
	})

	t.Run("StartAgentServices fails", func(t *testing.T) {
		mockAPI := &MockSystemAPI{}
		mockAPI.On("StartService", "datadogagent").Return(errors.New("service start failed"))

		manager := NewWinServiceManagerWithAPI(mockAPI)

		ctx := context.Background()
		err := manager.StartAgentServices(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start datadogagent service")
		mockAPI.AssertExpectations(t)
	})

	t.Run("RestartAgentServices success", func(t *testing.T) {
		mockAPI := &MockSystemAPI{}
		// Stop services
		mockAPI.On("StopService", "datadogagent").Return(nil)
		serviceNames := []string{
			"datadog-trace-agent",
			"datadog-process-agent",
			"datadog-security-agent",
			"datadog-system-probe",
			"Datadog Installer",
			"datadogagent",
		}
		for _, name := range serviceNames {
			mockAPI.On("IsServiceRunning", name).Return(false, nil)
		}
		// Start services
		mockAPI.On("StartService", "datadogagent").Return(nil)

		manager := NewWinServiceManagerWithAPI(mockAPI)

		ctx := context.Background()
		err := manager.RestartAgentServices(ctx)

		assert.NoError(t, err)
		mockAPI.AssertExpectations(t)
	})

	t.Run("RestartAgentServices continues with start even when stop fails", func(t *testing.T) {
		mockAPI := &MockSystemAPI{}

		// Stop services - set up a failure scenario
		mockAPI.On("StopService", "datadogagent").Return(nil)
		// First service is running but termination fails
		mockAPI.On("IsServiceRunning", "datadog-trace-agent").Return(true, nil)
		// Mock the GetServiceProcessID to fail, causing termination to fail
		mockAPI.On("GetServiceProcessID", "datadog-trace-agent").Return(uint32(1234), errors.New("access denied"))
		// Check if service is still running after failed termination
		mockAPI.On("IsServiceRunning", "datadog-trace-agent").Return(true, nil)

		// Other services are not running
		serviceNames := []string{
			"datadog-process-agent",
			"datadog-security-agent",
			"datadog-system-probe",
			"Datadog Installer",
			"datadogagent",
		}
		for _, name := range serviceNames {
			mockAPI.On("IsServiceRunning", name).Return(false, nil)
		}

		// Start services - this should still be called and succeed
		mockAPI.On("StartService", "datadogagent").Return(nil)

		manager := NewWinServiceManagerWithAPI(mockAPI)

		ctx := context.Background()
		err := manager.RestartAgentServices(ctx)

		// Should succeed overall because start succeeded, even though stop failed
		assert.NoError(t, err, "RestartAgentServices should succeed when start succeeds, even if stop fails")
		mockAPI.AssertExpectations(t)
	})
}
