// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"bytes"
	"strings"
	"testing"
	"time"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	snmpscanmock "github.com/DataDog/datadog-agent/comp/snmpscan/mock"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"

	"github.com/stretchr/testify/assert"
)

func TestStatus(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		scanReqs     []snmpscanmanager.ScanRequest
		deviceScans  deviceScansByIP
		expectedJSON map[string]interface{}
		expectedText string
		expectedHTML string
	}{
		{
			name:        "no devices",
			scanReqs:    []snmpscanmanager.ScanRequest{},
			deviceScans: deviceScansByIP{},
			expectedJSON: map[string]interface{}{
				"pendingScanCount": 0,
				"successScanCount": 0,
				"failedScanIPs":    []string{},
			},
			expectedText: ``,
			expectedHTML: ``,
		},
		{
			name: "multiple devices without failures",
			scanReqs: []snmpscanmanager.ScanRequest{
				{
					DeviceIP: "192.168.0.1",
				},
				{
					DeviceIP: "192.168.0.2",
				},
			},
			deviceScans: deviceScansByIP{
				"10.0.0.1": deviceScan{
					DeviceIP:   "10.0.0.1",
					ScanStatus: successScan,
					ScanEndTs:  now,
				},
				"10.0.0.2": deviceScan{
					DeviceIP:   "10.0.0.2",
					ScanStatus: successScan,
					ScanEndTs:  now,
				},
			},
			expectedJSON: map[string]interface{}{
				"pendingScanCount": 2,
				"successScanCount": 2,
				"failedScanIPs":    []string{},
			},
			expectedText: `
  Device Scans
  ============
  Pending scans count: 2
  Successful scans count: 2
  No failed scans.`,
			expectedHTML: `
<div class="stat">
  <span class="stat_title">SNMP Device Scans</span>
  <span class="stat_data">
    Pending scans count: 2</br>
    Successful scans count: 2</br>
    No failed scans.</br>
  </span>
</div>`,
		},
		{
			name: "multiple devices with failures",
			scanReqs: []snmpscanmanager.ScanRequest{
				{
					DeviceIP: "192.168.0.1",
				},
				{
					DeviceIP: "192.168.0.2",
				},
				{
					DeviceIP: "192.168.0.3",
				},
				{
					DeviceIP: "192.168.0.4",
				},
			},
			deviceScans: deviceScansByIP{
				"10.0.0.1": deviceScan{
					DeviceIP:   "10.0.0.1",
					ScanStatus: successScan,
					ScanEndTs:  now,
				},
				"10.0.0.2": deviceScan{
					DeviceIP:   "10.0.0.2",
					ScanStatus: successScan,
					ScanEndTs:  now,
				},
				"10.0.0.3": deviceScan{
					DeviceIP:   "10.0.0.3",
					ScanStatus: failedScan,
					ScanEndTs:  now,
					Failures:   1,
				},
				"10.0.0.4": deviceScan{
					DeviceIP:   "10.0.0.4",
					ScanStatus: failedScan,
					ScanEndTs:  now,
					Failures:   2,
				},
				"10.0.0.5": deviceScan{
					DeviceIP:   "10.0.0.5",
					ScanStatus: failedScan,
					ScanEndTs:  now,
					Failures:   3,
				},
				"10.0.0.6": deviceScan{
					DeviceIP:   "10.0.0.6",
					ScanStatus: failedScan,
					ScanEndTs:  now,
					Failures:   4,
				},
			},
			expectedJSON: map[string]interface{}{
				"pendingScanCount": 4,
				"successScanCount": 2,
				"failedScanIPs": []string{
					"10.0.0.3",
					"10.0.0.4",
					"10.0.0.5",
					"10.0.0.6",
				},
			},
			expectedText: `
  Device Scans
  ============
  Pending scans count: 4
  Successful scans count: 2
  Failed scans IPs:
    - 10.0.0.3
    - 10.0.0.4
    - 10.0.0.5
    - 10.0.0.6`,
			expectedHTML: `
<div class="stat">
  <span class="stat_title">SNMP Device Scans</span>
  <span class="stat_data">
    Pending scans count: 4</br>
    Successful scans count: 2</br>
    Failed scans IPs:</br>
      - 10.0.0.3</br>
      - 10.0.0.4</br>
      - 10.0.0.5</br>
      - 10.0.0.6</br>
  </span>
</div>`,
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

			for _, req := range tt.scanReqs {
				scanManager.scanQueue <- req
			}
			scanManager.deviceScans = tt.deviceScans

			stats := map[string]interface{}{}
			err = scanManager.JSON(false, stats)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedJSON, stats)

			bText := new(bytes.Buffer)
			err = scanManager.Text(false, bText)
			expectedText := strings.ReplaceAll(tt.expectedText, "\r\n", "\n")
			outText := strings.ReplaceAll(bText.String(), "\r\n", "\n")
			assert.NoError(t, err)
			assert.Equal(t, expectedText, outText)

			bHTML := new(bytes.Buffer)
			err = scanManager.HTML(false, bHTML)
			expectedHTML := strings.ReplaceAll(tt.expectedHTML, "\r\n", "\n")
			outHTML := strings.ReplaceAll(bHTML.String(), "\r\n", "\n")
			assert.NoError(t, err)
			assert.Equal(t, expectedHTML, outHTML)
		})
	}
}
