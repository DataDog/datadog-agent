// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	snmpscanmock "github.com/DataDog/datadog-agent/comp/snmpscan/mock"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/assert"
)

func TestFillFlare(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)
	mockConfig.SetInTest("network_devices.default_scan.enabled", true)

	mockLifecycle := compdef.NewTestLifecycle(t)
	mockLogger := logmock.New(t)
	mockIPC := ipcmock.New(t)

	scanner := snmpscanmock.Mock(t)
	mockScanner, ok := scanner.(*snmpscanmock.SnmpScanMock)
	assert.True(t, ok)
	mockScanner.On("ScanDeviceAndSendData",
		mock.Anything, &snmpparse.SNMPConfig{
			IPAddress:       "10.0.0.1",
			Port:            161,
			CommunityString: "public",
		}, "namespace", mock.Anything, mock.Anything).
		Return(nil).
		Once()

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

	mockConfigProvider := newSnmpConfigProviderMock()
	mockConfigProvider.On("GetDeviceConfig",
		"10.0.0.1", mock.Anything, mock.Anything).
		Return(&snmpparse.SNMPConfig{
			IPAddress:       "10.0.0.1",
			Port:            161,
			CommunityString: "public",
		}, "namespace", nil).
		Once()
	scanManager.snmpConfigProvider = mockConfigProvider

	err = mockLifecycle.Start(context.Background())
	assert.NoError(t, err)

	provides.Comp.RequestScan(snmpscanmanager.ScanRequest{
		DeviceIP: "10.0.0.1",
	}, false)

	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertDeviceScans(t, deviceScansByIP{
			"10.0.0.1": {
				DeviceIP:   "10.0.0.1",
				ScanStatus: successScan,
			},
		}, scanManager)
	}, 2*time.Second, 100*time.Millisecond)

	err = mockLifecycle.Stop(context.Background())
	assert.NoError(t, err)

	mockScanner.AssertExpectations(t)
	mockConfigProvider.AssertExpectations(t)

	flareBuilderMock := helpers.NewFlareBuilderMock(t, false)

	err = scanManager.fillFlare(flareBuilderMock)
	assert.NoError(t, err)

	filePath := filepath.Join(flareDirName, flareFileName)
	flareBuilderMock.AssertFileExists(filePath)
	flareBuilderMock.AssertFileContentMatch(
		`{"device_ip":"10\.0\.0\.1","scan_status":"success","scan_end_ts":".+?","failures":0}`,
		filePath)
}

func TestFillFlare_NoCache(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)

	mockLifecycle := compdef.NewTestLifecycle(t)
	mockLogger := logmock.New(t)
	mockIPC := ipcmock.New(t)
	scanner := snmpscanmock.Mock(t)

	reqs := Requires{
		Lifecycle:  mockLifecycle,
		Logger:     mockLogger,
		Config:     mockConfig,
		HTTPClient: mockIPC.GetClient(),
		Scanner:    scanner,
	}

	provides, err := NewComponent(reqs)
	assert.NoError(t, err)

	scanManager, ok := provides.Comp.(*snmpScanManagerImpl)
	assert.True(t, ok)

	err = mockLifecycle.Start(context.Background())
	assert.NoError(t, err)

	err = mockLifecycle.Stop(context.Background())
	assert.NoError(t, err)

	flareBuilderMock := helpers.NewFlareBuilderMock(t, false)

	err = scanManager.fillFlare(flareBuilderMock)
	assert.NoError(t, err)

	filePath := filepath.Join(flareDirName, flareFileName)
	flareBuilderMock.AssertNoFileExists(filePath)
}
