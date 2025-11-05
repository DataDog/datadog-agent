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
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	snmpscanmock "github.com/DataDog/datadog-agent/comp/snmpscan/mock"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
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
		name                string
		configContent       map[string]interface{}
		mockConfigProvider  snmpConfigProvider
		buildMockScanner    func() (snmpscan.Component, func(t mock.TestingT) bool)
		scanReqs            []snmpscanmanager.ScanRequest
		expectedDeviceScans deviceScansByIP
		assertTsNotNilByIP  map[string]bool
	}{
		{
			name: "default scan is disabled via config",
			configContent: map[string]interface{}{
				"network_devices.default_scan.disabled": true,
			},
			mockConfigProvider: newSnmpConfigProviderMock(
				map[string]*snmpparse.SNMPConfig{},
				map[string]error{},
			),
			buildMockScanner: func() (snmpscan.Component, func(t mock.TestingT) bool) {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				return mockScanner, mockScanner.AssertExpectations
			},
			scanReqs: []snmpscanmanager.ScanRequest{
				{
					DeviceIP:  "192.168.0.1",
					Namespace: "default",
				},
				{
					DeviceIP:  "192.168.0.2",
					Namespace: "default",
				},
			},
			expectedDeviceScans: deviceScansByIP{},
			assertTsNotNilByIP:  map[string]bool{},
		},
		{
			name:          "default scan is enabled",
			configContent: map[string]interface{}{},
			mockConfigProvider: newSnmpConfigProviderMock(
				map[string]*snmpparse.SNMPConfig{
					"192.168.0.1": {
						IPAddress:       "192.168.0.1",
						Port:            161,
						CommunityString: "public",
					},
					"10.0.0.1": {
						IPAddress:       "10.0.0.1",
						Port:            161,
						CommunityString: "public",
					},
				},
				map[string]error{
					"192.168.0.2": errors.New("some error"),
				},
			),
			buildMockScanner: func() (snmpscan.Component, func(t mock.TestingT) bool) {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, &snmpparse.SNMPConfig{
						IPAddress:       "192.168.0.1",
						Port:            161,
						CommunityString: "public",
					}, mock.Anything, mock.Anything).
					Return(nil).
					Once()

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, &snmpparse.SNMPConfig{
						IPAddress:       "10.0.0.1",
						Port:            161,
						CommunityString: "public",
					}, mock.Anything, mock.Anything).
					Return(nil).
					Once()

				return mockScanner, mockScanner.AssertExpectations
			},
			scanReqs: []snmpscanmanager.ScanRequest{
				{
					DeviceIP:  "192.168.0.1",
					Namespace: "default",
				},
				{
					DeviceIP:  "192.168.0.2",
					Namespace: "default",
				},
				{
					DeviceIP:  "192.168.0.2",
					Namespace: "default",
				},
				{
					DeviceIP:  "10.0.0.1",
					Namespace: "default",
				},
			},
			expectedDeviceScans: deviceScansByIP{
				"192.168.0.1": {
					DeviceIP:   "192.168.0.1",
					ScanStatus: successStatus,
				},
				"192.168.0.2": {
					DeviceIP:   "192.168.0.2",
					ScanStatus: failedStatus,
				},
				"10.0.0.1": {
					DeviceIP:   "10.0.0.1",
					ScanStatus: successStatus,
				},
			},
			assertTsNotNilByIP: map[string]bool{
				"192.168.0.1": true,
				"10.0.0.1":    true,
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
			mockScanner, assertExpectations := tt.buildMockScanner()

			reqs := Requires{
				Lifecycle:  mockLifecycle,
				Logger:     mockLogger,
				Config:     mockConfig,
				HTTPClient: mockIPC.GetClient(),
				Scanner:    mockScanner,
			}

			provides, err := NewComponent(reqs)
			assert.NoError(t, err)

			err = mockLifecycle.Start(context.Background())
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)

			scanManager.snmpConfigProvider = tt.mockConfigProvider

			for _, req := range tt.scanReqs {
				provides.Comp.RequestScan(req)
			}

			assert.EventuallyWithT(t, func(t *assert.CollectT) {
				assert.Equal(t, len(tt.expectedDeviceScans), len(scanManager.deviceScans))
				for _, actualScan := range scanManager.deviceScans {
					expectedScan, exists := tt.expectedDeviceScans[actualScan.DeviceIP]
					assert.True(t, exists)

					if tt.assertTsNotNilByIP[actualScan.DeviceIP] {
						assert.NotNil(t, actualScan.ScanEndTs)
					}
					actualScan.ScanEndTs = nil

					assert.Equal(t, expectedScan, actualScan)
				}
			}, 4*time.Second, 200*time.Millisecond)

			err = mockLifecycle.Stop(context.Background())
			assert.NoError(t, err)

			assertExpectations(t)
		})
	}
}

func TestProcessScanRequest(t *testing.T) {
	tests := []struct {
		name                 string
		mockConfigProvider   snmpConfigProvider
		buildMockScanner     func() (snmpscan.Component, func(t mock.TestingT) bool)
		scanRequest          snmpscanmanager.ScanRequest
		expectedDeviceScans  deviceScansByIP
		expectedCacheContent []deviceScan
		expectError          bool
		assertTsNotNilByIP   map[string]bool
	}{
		{
			name: "config provider returns an error",
			mockConfigProvider: newSnmpConfigProviderMock(
				map[string]*snmpparse.SNMPConfig{},
				map[string]error{
					"127.0.0.1": errors.New("some error"),
				},
			),
			buildMockScanner: func() (snmpscan.Component, func(t mock.TestingT) bool) {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				return mockScanner, mockScanner.AssertExpectations
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP:  "127.0.0.1",
				Namespace: "default",
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedStatus,
				},
			},
			expectedCacheContent: []deviceScan{
				{
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedStatus,
				},
			},
			expectError: true,
		},
		{
			name: "scan returns an error",
			mockConfigProvider: newSnmpConfigProviderMock(
				map[string]*snmpparse.SNMPConfig{
					"127.0.0.1": {
						IPAddress:       "127.0.0.1",
						Port:            161,
						CommunityString: "public",
					},
				},
				map[string]error{},
			),
			buildMockScanner: func() (snmpscan.Component, func(t mock.TestingT) bool) {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(errors.New("some error")).
					Once()

				return mockScanner, mockScanner.AssertExpectations
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP:  "127.0.0.1",
				Namespace: "default",
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedStatus,
				},
			},
			expectedCacheContent: []deviceScan{
				{
					DeviceIP:   "127.0.0.1",
					ScanStatus: failedStatus,
				},
			},
			expectError: true,
		},
		{
			name: "scan ok",
			mockConfigProvider: newSnmpConfigProviderMock(
				map[string]*snmpparse.SNMPConfig{
					"127.0.0.1": {
						IPAddress:       "127.0.0.1",
						Port:            161,
						CommunityString: "public",
					},
				},
				map[string]error{},
			),
			buildMockScanner: func() (snmpscan.Component, func(t mock.TestingT) bool) {
				scanner := snmpscanmock.Mock(t)
				mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
				assert.True(t, ok)

				mockScanner.On("ScanDeviceAndSendData",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil).
					Once()

				return mockScanner, mockScanner.AssertExpectations
			},
			scanRequest: snmpscanmanager.ScanRequest{
				DeviceIP:  "127.0.0.1",
				Namespace: "default",
			},
			expectedDeviceScans: deviceScansByIP{
				"127.0.0.1": {
					DeviceIP:   "127.0.0.1",
					ScanStatus: successStatus,
				},
			},
			expectedCacheContent: []deviceScan{
				{
					DeviceIP:   "127.0.0.1",
					ScanStatus: successStatus,
				},
			},
			expectError: false,
			assertTsNotNilByIP: map[string]bool{
				"127.0.0.1": true,
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
			mockScanner, assertExpectations := tt.buildMockScanner()

			reqs := Requires{
				Lifecycle:  mockLifecycle,
				Logger:     mockLogger,
				Config:     mockConfig,
				HTTPClient: mockIPC.GetClient(),
				Scanner:    mockScanner,
			}

			provides, err := NewComponent(reqs)
			assert.NoError(t, err)

			err = mockLifecycle.Start(context.Background())
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)

			scanManager.snmpConfigProvider = tt.mockConfigProvider
			scanErr := scanManager.processScanRequest(tt.scanRequest)

			err = mockLifecycle.Stop(context.Background())
			assert.NoError(t, err)

			if tt.expectError {
				assert.Error(t, scanErr)
			} else {
				assert.NoError(t, scanErr)

				assert.Equal(t, len(tt.expectedDeviceScans), len(scanManager.deviceScans))
				for _, actualScan := range scanManager.deviceScans {
					expectedScan, exists := tt.expectedDeviceScans[actualScan.DeviceIP]
					assert.True(t, exists)

					if tt.assertTsNotNilByIP[actualScan.DeviceIP] {
						assert.NotNil(t, actualScan.ScanEndTs)
					}
					actualScan.ScanEndTs = nil

					assert.Equal(t, expectedScan, actualScan)
				}

				cacheContent, err := persistentcache.Read(cacheKey)
				assert.NoError(t, err)
				var actualCacheContent []deviceScan
				assert.NoError(t, json.Unmarshal([]byte(cacheContent), &actualCacheContent))
				for i := range actualCacheContent {
					if tt.assertTsNotNilByIP[actualCacheContent[i].DeviceIP] {
						assert.NotNil(t, actualCacheContent[i].ScanEndTs)
					}
					actualCacheContent[i].ScanEndTs = nil
				}
				assert.ElementsMatch(t, tt.expectedCacheContent, actualCacheContent)
			}

			assertExpectations(t)
		})
	}
}

func TestCacheIsLoaded(t *testing.T) {
	tests := []struct {
		name                     string
		cacheContent             string
		buildExpectedDeviceScans func() deviceScansByIP
	}{
		{
			name:                     "empty cache",
			cacheContent:             "",
			buildExpectedDeviceScans: func() deviceScansByIP { return deviceScansByIP{} },
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
        "scan_status":"failed"
    }
]`,
			buildExpectedDeviceScans: func() deviceScansByIP {
				scanEndTs, err := time.Parse(time.RFC3339Nano, "2025-11-04T13:21:20.365221+01:00")
				assert.NoError(t, err)

				return deviceScansByIP{
					"127.0.0.1": {
						DeviceIP:   "127.0.0.1",
						ScanStatus: successStatus,
						ScanEndTs:  &scanEndTs,
					},
					"127.0.0.2": {
						DeviceIP:   "127.0.0.2",
						ScanStatus: failedStatus,
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

			err = mockLifecycle.Start(context.Background())
			assert.NoError(t, err)

			err = mockLifecycle.Stop(context.Background())
			assert.NoError(t, err)

			scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
			assert.True(t, ok)
			assert.Equal(t, tt.buildExpectedDeviceScans(), scanManager.deviceScans)
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
						ScanStatus: successStatus,
						ScanEndTs:  &scanEndTs,
					},
					"10.0.0.1": {
						DeviceIP:   "10.0.0.1",
						ScanStatus: successStatus,
						ScanEndTs:  &scanEndTs,
					},
					"10.0.0.2": {
						DeviceIP:   "10.0.0.2",
						ScanStatus: failedStatus,
					},
				}
			},
			buildExpectedCacheContent: func() []deviceScan {
				scanEndTs, err := time.Parse(time.RFC3339Nano, "2025-11-04T13:21:20.365221+01:00")
				assert.NoError(t, err)

				return []deviceScan{
					{
						DeviceIP:   "127.0.0.1",
						ScanStatus: successStatus,
						ScanEndTs:  &scanEndTs,
					},
					{
						DeviceIP:   "10.0.0.1",
						ScanStatus: successStatus,
						ScanEndTs:  &scanEndTs,
					},
					{
						DeviceIP:   "10.0.0.2",
						ScanStatus: failedStatus,
					},
				}
			},
		},
		{
			name: "pending device scans are not written",
			buildDeviceScans: func() deviceScansByIP {
				return deviceScansByIP{
					"127.0.0.1": {
						DeviceIP:   "127.0.0.1",
						ScanStatus: failedStatus,
					},
					"127.0.0.2": {
						DeviceIP:   "127.0.0.2",
						ScanStatus: pendingStatus,
					},
					"10.0.0.1": {
						DeviceIP:   "10.0.0.1",
						ScanStatus: pendingStatus,
					},
				}
			},
			buildExpectedCacheContent: func() []deviceScan {
				return []deviceScan{
					{
						DeviceIP:   "127.0.0.1",
						ScanStatus: failedStatus,
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
