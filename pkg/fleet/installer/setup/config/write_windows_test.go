// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

// everyoneCanRead reports whether path's DACL contains an allow ACE granting read access
// to the Everyone group (S-1-1-0).
func everyoneCanRead(t *testing.T, path string) bool {
	t.Helper()
	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	assert.NoError(t, err)

	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	assert.NoError(t, err)
	dacl, _, err := sd.DACL()
	assert.NoError(t, err)

	const (
		genericRead  = 0x80000000 // GENERIC_READ, if not mapped to file-specific rights
		fileReadData = 0x1        // FILE_READ_DATA, set when GENERIC_READ is mapped to FILE_GENERIC_READ
	)
	for i := 0; i < int(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		assert.NoError(t, windows.GetAce(dacl, uint32(i), &ace))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(everyone) && ace.Mask&(genericRead|fileReadData) != 0 {
			return true
		}
	}
	return false
}

// TestWriteConfigsGrantsEveryoneRead verifies that setup-time config writes (the install-script /
// DJM path) grant Everyone read on application_monitoring.yaml only. Other config files stay
// admin/ddagentuser-only — including conf.d integration configs, which are also written 0644 but
// may contain credentials, so they must NOT be world-readable.
func TestWriteConfigsGrantsEveryoneRead(t *testing.T) {
	tempDir := t.TempDir()
	config := Config{
		ApplicationMonitoringYAML: &ApplicationMonitoringConfig{
			Default: APMConfigurationDefault{
				TraceDebug: BoolToPtr(true),
			},
		},
		IntegrationConfigs: map[string]IntegrationConfig{
			"mycheck.yaml": {InitConfig: nil, Instances: []any{map[string]any{"host": "localhost"}}},
		},
	}
	config.DatadogYAML.APIKey = "1234567890"

	assert.NoError(t, WriteConfigs(config, tempDir))

	appMonitoring := filepath.Join(tempDir, "application_monitoring.yaml")
	assert.FileExists(t, appMonitoring)
	assert.True(t, everyoneCanRead(t, appMonitoring),
		"application_monitoring.yaml should grant Everyone read")

	datadogYAML := filepath.Join(tempDir, datadogConfFile)
	assert.FileExists(t, datadogYAML)
	assert.False(t, everyoneCanRead(t, datadogYAML),
		"datadog.yaml (0640) should not grant Everyone read")

	// conf.d integration configs are written 0644 but must not be world-readable on Windows.
	confDCheck := filepath.Join(tempDir, "conf.d", "mycheck.yaml")
	assert.FileExists(t, confDCheck)
	assert.False(t, everyoneCanRead(t, confDCheck),
		"conf.d integration configs should not grant Everyone read")
}
