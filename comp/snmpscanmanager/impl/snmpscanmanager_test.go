// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package snmpscanmanagerimpl

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	snmpscanmock "github.com/DataDog/datadog-agent/comp/snmpscan/mock"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewComponent(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("run_path", testDir)

	mockLifecycle := compdef.NewTestLifecycle(t)
	mockLogger := logmock.New(t)
	mockIPC := ipcmock.New(t)
	mockScanner := snmpscanmock.Mock(t)

	reqs := Requires{
		Lifecycle:  mockLifecycle,
		Logger:     mockLogger,
		Config:     mockConfig,
		HTTPClient: mockIPC.GetClient(),
		Scanner:    mockScanner,
	}

	provides, err := NewComponent(reqs)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)

	mockLifecycle.AssertHooksNumber(1)

	err = mockLifecycle.Start(context.Background())
	assert.NoError(t, err)

	err = mockLifecycle.Stop(context.Background())
	assert.NoError(t, err)
}

func TestRequestScan(t *testing.T) {
	tests := []struct {
		name                    string
		configContent           map[string]interface{}
		buildMockConfigProvider func() *snmpConfigProviderMock
		buildMockScanner        func() *snmpscanmock.SnmpScanMock
		scanReqs                []snmpscanmanager.ScanRequest
		expectedDeviceScans     deviceScansByIP
	}{
		{
			name:          "default scan is disabled by default",
			configContent: map[string]interface{}{},
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()
				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				return mockScanner
			},
			scanReqs: []snmpscanmanager.ScanRequest{
				{
					DeviceIP: "192.168.0.1",
				},
				{
					DeviceIP: "192.168.0.2",
				},
			},
			expectedDeviceScans: deviceScansByIP{},
		},
		{
			name: "default scan is enabled via config",
			configContent: map[string]interface{}{
				"network_devices.default_scan.enabled": true,
			},
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()

				mockConfigProvider.On("GetDeviceConfig",
					"192.168.0.1", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "192.168.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				mockConfigProvider.On("GetDeviceConfig",
					"192.168.0.2", mock.Anything, mock.Anything).
					Return(nil, "", errors.New("some error")).
					Once()

				mockConfigProvider.On("GetDeviceConfig",
					"10.0.0.1", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "10.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				mockConfigProvider.On("GetDeviceConfig",
					"10.0.0.2", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "10.0.0.2",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, &snmpparse.SNMPConfig{
						IPAddress:       "192.168.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", mock.Anything, mock.Anything).
					Return(nil).
					Once()

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, &snmpparse.SNMPConfig{
						IPAddress:       "10.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", mock.Anything, mock.Anything).
					Return(nil).
					Once()

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, &snmpparse.SNMPConfig{
						IPAddress:       "10.0.0.2",
						Port:            161,
						CommunityString: "public",
					}, "namespace", mock.Anything, mock.Anything).
					Return(gosnmplib.NewConnectionError(errors.New("some error"))).
					Once()

				return mockScanner
			},
			scanReqs: []snmpscanmanager.ScanRequest{
				{
					DeviceIP: "192.168.0.1",
				},
				{
					DeviceIP: "192.168.0.2",
				},
				{
					DeviceIP: "192.168.0.2",
				},
				{
					DeviceIP: "10.0.0.1",
				},
				{
					DeviceIP: "10.0.0.2",
				},
			},
			expectedDeviceScans: deviceScansByIP{
				"192.168.0.1": {
					DeviceIP:   "192.168.0.1",
					ScanStatus: successScan,
				},
				"192.168.0.2": {
					DeviceIP:   "192.168.0.2",
					ScanStatus: failedScan,
					Failures:   -1,
				},
				"10.0.0.1": {
					DeviceIP:   "10.0.0.1",
					ScanStatus: successScan,
				},
				"10.0.0.2": {
					DeviceIP:   "10.0.0.2",
					ScanStatus: failedScan,
					Failures:   1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)
			for key, value := range tt.configContent {
				mockConfig.SetWithoutSource(key, value)
			}

			mockLifecycle := compdef.NewTestLifecycle(t)
			mockLogger := logmock.New(t)
			mockIPC := ipcmock.New(t)
			mockScanner := tt.buildMockScanner()

			reqs := Requires{
				Lifecycle:  mockLifecycle,
				Logger:     mockLogger,
				Config:     mockConfig,
				HTTPClient: mockIPC.GetClient(),
				Scanner:    mockScanner,
			}

			provides, err := NewComponent(reqs)
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)

			mockConfigProvider := tt.buildMockConfigProvider()
			scanManager.snmpConfigProvider = mockConfigProvider

			err = mockLifecycle.Start(context.Background())
			assert.NoError(t, err)

			for _, req := range tt.scanReqs {
				provides.Comp.RequestScan(req, false)
			}

			assert.EventuallyWithT(t, func(t *assert.CollectT) {
				assertDeviceScans(t, tt.expectedDeviceScans, scanManager)
			}, 2*time.Second, 100*time.Millisecond)

			err = mockLifecycle.Stop(context.Background())
			assert.NoError(t, err)

			mockScanner.AssertExpectations(t)
			mockConfigProvider.AssertExpectations(t)
		})
	}
}

func TestProcessScanRequest(t *testing.T) {
	tests := []struct {
		name                    string
		buildMockConfigProvider func() *snmpConfigProviderMock
		buildMockScanner        func() *snmpscanmock.SnmpScanMock
		scanRequest             snmpscanmanager.ScanRequest
		expectedDeviceScans     deviceScansByIP
		expectedCacheContent    []deviceScan
		expectedScanTasks       []*scanTask
		expectError             bool
	}{
		{
			name: "config provider returns an error",
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()

				mockConfigProvider.On("GetDeviceConfig",
					"127.0.0.1", mock.Anything, mock.Anything).
					Return(nil, "", errors.New("some error")).
					Once()

				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				return mockScanner
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP: "127.0.0.1",
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedScan,
					Failures:   -1,
				},
			},
			expectedCacheContent: []deviceScan{
				{
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedScan,
					Failures:   -1,
				},
			},
			expectedScanTasks: []*scanTask{},
			expectError:       true,
		},
		{
			name: "scan returns a context canceled error",
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()

				mockConfigProvider.On("GetDeviceConfig",
					"127.0.0.1", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "127.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(context.Canceled).
					Once()

				return mockScanner
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP: "127.0.0.1",
			},
			expectedDeviceScans:  deviceScansByIP{},
			expectedCacheContent: []deviceScan{},
			expectedScanTasks:    []*scanTask{},
			expectError:          false,
		},
		{
			name: "scan returns a connection error (retryable)",
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()

				mockConfigProvider.On("GetDeviceConfig",
					"127.0.0.1", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "127.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(gosnmplib.NewConnectionError(errors.New("some error"))).
					Once()

				return mockScanner
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP: "127.0.0.1",
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedScan,
					Failures:   1,
				},
			},
			expectedCacheContent: []deviceScan{
				{
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedScan,
					Failures:   1,
				},
			},
			expectedScanTasks: []*scanTask{
				{
					req: snmpscanmanager.ScanRequest{
						DeviceIP: "127.0.0.1",
					},
				},
			},
			expectError: true,
		},
		{
			name: "scan returns a normal error (not retryable)",
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()

				mockConfigProvider.On("GetDeviceConfig",
					"127.0.0.1", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "127.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(errors.New("some error")).
					Once()

				return mockScanner
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP: "127.0.0.1",
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedScan,
					Failures:   -1,
				},
			},
			expectedCacheContent: []deviceScan{
				{
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedScan,
					Failures:   -1,
				},
			},
			expectedScanTasks: []*scanTask{},
			expectError:       true,
		},
		{
			name: "scan ok",
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()

				mockConfigProvider.On("GetDeviceConfig",
					"127.0.0.1", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "127.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil).
					Once()

				return mockScanner
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP: "127.0.0.1",
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: successScan,
				},
			},
			expectedCacheContent: []deviceScan{
				{
					DeviceIP:   "127.0.0.1",
					ScanStatus: successScan,
				},
			},
			expectedScanTasks: []*scanTask{
				{
					req: snmpscanmanager.ScanRequest{
						DeviceIP: "127.0.0.1",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)

			mockLifecycle := compdef.NewTestLifecycle(t)
			mockLogger := logmock.New(t)
			mockIPC := ipcmock.New(t)
			mockScanner := tt.buildMockScanner()

			reqs := Requires{
				Lifecycle:  mockLifecycle,
				Logger:     mockLogger,
				Config:     mockConfig,
				HTTPClient: mockIPC.GetClient(),
				Scanner:    mockScanner,
			}

			provides, err := NewComponent(reqs)
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)

			mockConfigProvider := tt.buildMockConfigProvider()
			scanManager.snmpConfigProvider = mockConfigProvider

			err = mockLifecycle.Start(context.Background())
			assert.NoError(t, err)

			scanErr := scanManager.processScanRequest(tt.scanRequest)

			err = mockLifecycle.Stop(context.Background())
			assert.NoError(t, err)

			if tt.expectError {
				assert.Error(t, scanErr)
			} else {
				assert.NoError(t, scanErr)
			}

			assertDeviceScans(t, tt.expectedDeviceScans, scanManager)
			assertCacheContent(t, tt.expectedCacheContent, cacheKey)
			assertScanTasks(t, tt.expectedScanTasks, scanManager)

			mockScanner.AssertExpectations(t)
			mockConfigProvider.AssertExpectations(t)
		})
	}
}

func TestCacheIsLoaded(t *testing.T) {
	tests := []struct {
		name                     string
		cacheContent             string
		buildExpectedDeviceScans func() deviceScansByIP
		expectedScanTasks        []*scanTask
	}{
		{
			name:                     "empty cache",
			cacheContent:             "",
			buildExpectedDeviceScans: func() deviceScansByIP { return deviceScansByIP{} },
			expectedScanTasks:        []*scanTask{},
		},
		{
			name: "cache with multiple device scans",
			cacheContent: `[
    {
        "device_ip":"127.0.0.1",
        "scan_status":"success",
        "scan_end_ts":"2025-11-04T13:21:20.365221+01:00"
    },
    {
        "device_ip":"127.0.0.2",
        "scan_status":"failed",
        "scan_end_ts":"2025-11-04T13:21:20.365221+01:00",
        "failures":2
    }
]`,
			buildExpectedDeviceScans: func() deviceScansByIP {
				scanEndTs, err := time.Parse(time.RFC3339Nano, "2025-11-04T13:21:20.365221+01:00")
				assert.NoError(t, err)

				return deviceScansByIP{
					"127.0.0.1": {
						DeviceIP:   "127.0.0.1",
						ScanStatus: successScan,
						ScanEndTs:  scanEndTs,
					},
					"127.0.0.2": {
						DeviceIP:   "127.0.0.2",
						ScanStatus: failedScan,
						ScanEndTs:  scanEndTs,
						Failures:   2,
					},
				}
			},
			expectedScanTasks: []*scanTask{
				{
					req: snmpscanmanager.ScanRequest{
						DeviceIP: "127.0.0.1",
					},
				},
				{
					req: snmpscanmanager.ScanRequest{
						DeviceIP: "127.0.0.2",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)

			err := persistentcache.Write(cacheKey, tt.cacheContent)
			assert.NoError(t, err)

			mockLifecycle := compdef.NewTestLifecycle(t)
			mockLogger := logmock.New(t)
			mockIPC := ipcmock.New(t)
			mockScanner := snmpscanmock.Mock(t)

			reqs := Requires{
				Lifecycle:  mockLifecycle,
				Logger:     mockLogger,
				Config:     mockConfig,
				HTTPClient: mockIPC.GetClient(),
				Scanner:    mockScanner,
			}

			provides, err := NewComponent(reqs)
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)
			assert.Equal(t, tt.buildExpectedDeviceScans(), scanManager.cloneDeviceScans())

			assertScanTasks(t, tt.expectedScanTasks, scanManager)
		})
	}
}

func TestWriteCache(t *testing.T) {
	tests := []struct {
		name                      string
		buildDeviceScans          func() deviceScansByIP
		buildExpectedCacheContent func() []deviceScan
	}{
		{
			name:                      "empty",
			buildDeviceScans:          func() deviceScansByIP { return deviceScansByIP{} },
			buildExpectedCacheContent: func() []deviceScan { return []deviceScan{} },
		},
		{
			name: "multiple device scans",
			buildDeviceScans: func() deviceScansByIP {
				scanEndTs, err := time.Parse(time.RFC3339Nano, "2025-11-04T13:21:20.365221+01:00")
				assert.NoError(t, err)

				return deviceScansByIP{
					"127.0.0.1": {
						DeviceIP:   "127.0.0.1",
						ScanStatus: successScan,
						ScanEndTs:  scanEndTs,
					},
					"10.0.0.1": {
						DeviceIP:   "10.0.0.1",
						ScanStatus: successScan,
						ScanEndTs:  scanEndTs,
					},
					"10.0.0.2": {
						DeviceIP:   "10.0.0.2",
						ScanStatus: failedScan,
						ScanEndTs:  scanEndTs,
					},
				}
			},
			buildExpectedCacheContent: func() []deviceScan {
				scanEndTs, err := time.Parse(time.RFC3339Nano, "2025-11-04T13:21:20.365221+01:00")
				assert.NoError(t, err)

				return []deviceScan{
					{
						DeviceIP:   "127.0.0.1",
						ScanStatus: successScan,
						ScanEndTs:  scanEndTs,
					},
					{
						DeviceIP:   "10.0.0.1",
						ScanStatus: successScan,
						ScanEndTs:  scanEndTs,
					},
					{
						DeviceIP:   "10.0.0.2",
						ScanStatus: failedScan,
						ScanEndTs:  scanEndTs,
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)

			mockLifecycle := compdef.NewTestLifecycle(t)
			mockLogger := logmock.New(t)
			mockIPC := ipcmock.New(t)
			mockScanner := snmpscanmock.Mock(t)

			reqs := Requires{
				Lifecycle:  mockLifecycle,
				Logger:     mockLogger,
				Config:     mockConfig,
				HTTPClient: mockIPC.GetClient(),
				Scanner:    mockScanner,
			}

			provides, err := NewComponent(reqs)
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)

			scanManager.deviceScans = tt.buildDeviceScans()
			scanManager.writeCache()

			cacheContent, err := persistentcache.Read(cacheKey)
			assert.NoError(t, err)

			var actualCacheContent []deviceScan
			assert.NoError(t, json.Unmarshal([]byte(cacheContent), &actualCacheContent))
			assert.ElementsMatch(t, tt.buildExpectedCacheContent(), actualCacheContent)
		})
	}
}

func TestQueueDueScans(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                    string
		scanTasks               []scanTask
		buildMockConfigProvider func() *snmpConfigProviderMock
		buildMockScanner        func() *snmpscanmock.SnmpScanMock
		expectedDeviceScans     deviceScansByIP
	}{
		{
			name: "due scans are queued",
			scanTasks: []scanTask{
				{
					req: snmpscanmanager.ScanRequest{
						DeviceIP: "10.0.0.1",
					},
					nextScanTs: now.Add(999 * time.Hour), // Not a due scan
				},
				{
					req: snmpscanmanager.ScanRequest{
						DeviceIP: "127.0.0.1",
					},
					nextScanTs: now.Add(-1 * time.Minute), // Due scan
				},
				{
					req: snmpscanmanager.ScanRequest{
						DeviceIP: "127.0.0.2",
					},
					nextScanTs: now.Add(-2 * time.Minute), // Due scan
				},
			},
			buildMockConfigProvider: func() *snmpConfigProviderMock {
				mockConfigProvider := newSnmpConfigProviderMock()

				mockConfigProvider.On("GetDeviceConfig",
					"127.0.0.1", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "127.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				mockConfigProvider.On("GetDeviceConfig",
					"127.0.0.2", mock.Anything, mock.Anything).
					Return(&snmpparse.SNMPConfig{
						IPAddress:       "127.0.0.2",
						Port:            161,
						CommunityString: "public",
					}, "namespace", nil).
					Once()

				return mockConfigProvider
			},
			buildMockScanner: func() *snmpscanmock.SnmpScanMock {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil).
					Once()

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil).
					Once()

				return mockScanner
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: successScan,
				},
				"127.0.0.2": {
					DeviceIP:   "127.0.0.2",
					ScanStatus: successScan,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)
			mockConfig.SetWithoutSource("network_devices.default_scan.enabled", true)

			mockLifecycle := compdef.NewTestLifecycle(t)
			mockLogger := logmock.New(t)
			mockIPC := ipcmock.New(t)
			mockScanner := tt.buildMockScanner()

			reqs := Requires{
				Lifecycle:  mockLifecycle,
				Logger:     mockLogger,
				Config:     mockConfig,
				HTTPClient: mockIPC.GetClient(),
				Scanner:    mockScanner,
			}

			provides, err := NewComponent(reqs)
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)

			mockConfigProvider := tt.buildMockConfigProvider()
			scanManager.snmpConfigProvider = mockConfigProvider

			for _, st := range tt.scanTasks {
				scanManager.scanScheduler.QueueScanTask(st)
			}

			scanManager.queueDueScans()

			err = mockLifecycle.Start(context.Background())
			assert.NoError(t, err)

			assert.EventuallyWithT(t, func(t *assert.CollectT) {
				assertDeviceScans(t, tt.expectedDeviceScans, scanManager)
			}, 2*time.Second, 100*time.Millisecond)

			err = mockLifecycle.Stop(context.Background())
			assert.NoError(t, err)

			mockScanner.AssertExpectations(t)
			mockConfigProvider.AssertExpectations(t)
		})
	}
}

func assertDeviceScans(t assert.TestingT, expectedDeviceScans deviceScansByIP, scanManager *snmpScanManagerImpl) {
	actualDeviceScans := scanManager.cloneDeviceScans()

	assert.Equal(t, len(expectedDeviceScans), len(actualDeviceScans))
	for _, actualScan := range actualDeviceScans {
		expectedScan, exists := expectedDeviceScans[actualScan.DeviceIP]
		assert.True(t, exists)

		assert.NotEmpty(t, actualScan.ScanEndTs)
		actualScan.ScanEndTs = time.Time{}

		assert.Equal(t, expectedScan, actualScan)
	}
}

func assertCacheContent(t assert.TestingT, expectedCacheContent []deviceScan, cacheKey string) {
	cacheContent, err := persistentcache.Read(cacheKey)
	assert.NoError(t, err)

	var actualCacheContent []deviceScan
	if len(cacheContent) > 0 {
		assert.NoError(t, json.Unmarshal([]byte(cacheContent), &actualCacheContent))
	}

	assert.Equal(t, len(expectedCacheContent), len(actualCacheContent))
	for _, actualScan := range actualCacheContent {
		assert.NotEmpty(t, actualScan.ScanEndTs)
		actualScan.ScanEndTs = time.Time{}

		assert.Contains(t, expectedCacheContent, actualScan)
	}
}

func assertScanTasks(t assert.TestingT, expectedScanTasks []*scanTask, scanManager *snmpScanManagerImpl) {
	sc, ok := scanManager.scanScheduler.(*scanSchedulerImpl)
	assert.True(t, ok)

	assert.Equal(t, len(expectedScanTasks), len(sc.taskQueue))
	for _, actualTask := range sc.taskQueue {
		assert.NotEmpty(t, actualTask.nextScanTs)
		actualTask.nextScanTs = time.Time{}

		assert.Contains(t, expectedScanTasks, actualTask)
	}
}
