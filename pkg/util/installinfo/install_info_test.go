// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installinfo

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_logVersionHistoryToFile(t *testing.T) {
	tests := []struct {
		name               string
		versionHistoryFile string
		installInfoFile    string
		version            string
		timestamp          time.Time
		want               string
	}{
		{
			name:               "install_info is empty",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile:    "",
			version:            "2",
			timestamp:          time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:               `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z","install_method":{"tool":"","tool_version":"","installer_version":""}},{"version":"2","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"","tool_version":"","installer_version":""}}]}`,
		},
		{
			name:               "existing version-history.json is empty",
			versionHistoryFile: "",
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "1",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:      `{"entries":[{"version":"1","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"install_script","tool_version":"install_script","installer_version":"install_script-x.x.x"}}]}`,
		},
		{
			name:               "version in new entry is same as the last entry",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "1",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:      `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
		},
		{
			name:               "version and timestamp of the new entry is empty",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			want: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
		},

		{
			name:               "existing version-history.json in invalid JSON", // Ignore the invalid last entry.
			versionHistoryFile: `{"entries":[{"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "2",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:      `{"entries":[{"version":"2","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"install_script","tool_version":"install_script","installer_version":"install_script-x.x.x"}}]}`,
		},
		{
			name:               "install_info in invalid YAML",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "2",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:      `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z","install_method":{"tool":"","tool_version":"","installer_version":""}},{"version":"2","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"","tool_version":"","installer_version":""}}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vh, _ := os.CreateTemp("", "version-history.json")
			vh.WriteString(tt.versionHistoryFile)
			defer os.Remove(vh.Name())
			versionHistoryFilePath := vh.Name()

			var installInfoFilePath string
			if tt.installInfoFile != "" {
				f, _ := os.CreateTemp("", "install_info")
				f.WriteString(tt.installInfoFile)
				defer os.Remove(f.Name())
				installInfoFilePath = f.Name()
			}
			logVersionHistoryToFile(versionHistoryFilePath, installInfoFilePath, tt.version, tt.timestamp)
			b, _ := os.ReadFile(versionHistoryFilePath)
			assert.Equal(t, tt.want, string(b))
		})
	}
}

func Test_logVersionHistoryToFile_maxVersionHistoryEntries(t *testing.T) {
	now := time.Now().UTC()

	entries := make([]versionHistoryEntry, maxVersionHistoryEntries)
	expected := make([]versionHistoryEntry, maxVersionHistoryEntries)
	for i := 0; i < maxVersionHistoryEntries; i++ {
		entries[i] = versionHistoryEntry{
			Version:   fmt.Sprintf("%d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
			InstallMethod: InstallInfo{
				Tool:             "tool",
				ToolVersion:      "tool_version",
				InstallerVersion: "installer_version",
			},
		}
		expected[i] = versionHistoryEntry{
			Version:   fmt.Sprintf("%d", i+10),
			Timestamp: now.Add(time.Duration(i+10) * time.Second),
			InstallMethod: InstallInfo{
				Tool:             "tool",
				ToolVersion:      "tool_version",
				InstallerVersion: "installer_version",
			},
		}
	}

	actual, _ := os.CreateTemp("", "version-history.json")
	defer os.Remove(actual.Name())
	b, _ := json.Marshal(versionHistoryEntries{Entries: entries})
	actual.Write(b)

	installInfoFile, _ := os.CreateTemp("", "install_info")
	defer os.Remove(installInfoFile.Name())
	installInfoFile.WriteString(`
---
install_method:
  tool: tool
  tool_version: tool_version
  installer_version: installer_version
`)

	for i := maxVersionHistoryEntries; i < maxVersionHistoryEntries+10; i++ {
		logVersionHistoryToFile(
			actual.Name(),
			installInfoFile.Name(),
			fmt.Sprintf("%d", i),
			now.Add(time.Duration(i)*time.Second),
		)
	}

	actualBytes, _ := os.ReadFile(actual.Name())
	expectedBytes, _ := json.Marshal(versionHistoryEntries{Entries: expected})
	assert.Equal(t, string(expectedBytes), string(actualBytes))
}

func Test_useEnvVarsToSetInstallInfo(t *testing.T) {
	// Regardless of what the install info file contains, the env vars will be used.
	t.Setenv("DD_INSTALL_INFO_TOOL", "install_script")
	t.Setenv("DD_INSTALL_INFO_TOOL_VERSION", "install_script")
	t.Setenv("DD_INSTALL_INFO_INSTALLER_VERSION", "install_script-x.x.x")
	tests := []struct {
		name               string
		versionHistoryFile string
		installInfoFile    string
		version            string
		timestamp          time.Time
		want               string
	}{
		{
			name:               "install_info is empty",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile:    "",
			version:            "2",
			timestamp:          time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			// now we have the install info through env vars, so the second version is populated
			want: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z","install_method":{"tool":"","tool_version":"","installer_version":""}},{"version":"2","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"install_script","tool_version":"install_script","installer_version":"install_script-x.x.x"}}]}`,
		},
		{
			name:               "existing version-history.json is empty",
			versionHistoryFile: "",
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "1",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:      `{"entries":[{"version":"1","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"install_script","tool_version":"install_script","installer_version":"install_script-x.x.x"}}]}`,
		},
		{
			name:               "version in new entry is same as the last entry",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "1",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:      `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
		},
		{
			name:               "version and timestamp of the new entry is empty",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			// we still need a version to be able to write a new entry
			want: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
		},

		{
			name:               "existing version-history.json in invalid JSON",
			versionHistoryFile: `{"entries":[{"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "2",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			want:      `{"entries":[{"version":"2","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"install_script","tool_version":"install_script","installer_version":"install_script-x.x.x"}}]}`,
		},
		{
			name:               "install_info in invalid YAML",
			versionHistoryFile: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z"}]}`,
			installInfoFile: `
---
install_method:
  tool: install_script tool_version: install_script
  installer_version: install_script-x.x.x
`,
			version:   "2",
			timestamp: time.Date(2022, 4, 12, 7, 10, 58, 1234, time.UTC),
			// now we have the install info through env vars, so the second version is populated
			want: `{"entries":[{"version":"1","timestamp":"2022-04-07T14:24:58.152534935Z","install_method":{"tool":"","tool_version":"","installer_version":""}},{"version":"2","timestamp":"2022-04-12T07:10:58.000001234Z","install_method":{"tool":"install_script","tool_version":"install_script","installer_version":"install_script-x.x.x"}}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vh, err := os.CreateTemp("", "version-history.json")
			require.NoError(t, err)
			vh.WriteString(tt.versionHistoryFile)
			defer os.Remove(vh.Name())
			versionHistoryFilePath := vh.Name()

			f, err := os.CreateTemp("", "install_info")
			require.NoError(t, err)
			f.WriteString(tt.installInfoFile)
			defer os.Remove(f.Name())
			installInfoFilePath := f.Name()
			logVersionHistoryToFile(versionHistoryFilePath, installInfoFilePath, tt.version, tt.timestamp)
			b, _ := os.ReadFile(versionHistoryFilePath)
			assert.Equal(t, tt.want, string(b))
		})
	}
}

func TestScrubFromEnvVars(t *testing.T) {
	t.Setenv("DD_INSTALL_INFO_TOOL", "./my_installer.sh --password=hunter2")
	t.Setenv("DD_INSTALL_INFO_TOOL_VERSION", "2.5.0 password=hunter2")
	t.Setenv("DD_INSTALL_INFO_INSTALLER_VERSION", "3.7.1 password=hunter2")

	info, ok := getFromEnvVars()
	assert.True(t, ok)

	assert.Equal(t, "./my_installer.sh --password=********", info.Tool)
	assert.Equal(t, "2.5.0 password=********", info.ToolVersion)
	assert.Equal(t, "3.7.1 password=********", info.InstallerVersion)
}

func TestScrubFromPath(t *testing.T) {
	installInfoYaml := `install_method:
  tool: "./my_installer.sh --password=hunter2"
  tool_version: "2.5.0 password=hunter2"
  installer_version: "3.7.1 password=hunter2"
`
	f, err := os.CreateTemp("", "install-info.yaml")
	require.NoError(t, err)
	f.WriteString(installInfoYaml)
	defer os.Remove(f.Name())

	info, err := getFromPath(f.Name())
	require.NoError(t, err)

	assert.Equal(t, "./my_installer.sh --password=********", info.Tool)
	assert.Equal(t, "2.5.0 password=********", info.ToolVersion)
	assert.Equal(t, "3.7.1 password=********", info.InstallerVersion)
}

func TestSetRuntimeInstallInfo(t *testing.T) {
	tests := []struct {
		name        string
		info        *InstallInfo
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid install info",
			info: &InstallInfo{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectError: false,
		},
		{
			name: "valid install info with scrubbing needed",
			info: &InstallInfo{
				Tool:             "ECS --password=hunter2",
				ToolVersion:      "1.0 password=hunter2",
				InstallerVersion: "ecs-task-def-v1 password=hunter2",
			},
			expectError: false,
		},
		{
			name:        "nil install info",
			info:        nil,
			expectError: true,
			errorMsg:    "install info cannot be nil",
		},
		{
			name: "empty tool field",
			info: &InstallInfo{
				Tool:             "",
				ToolVersion:      "1.0",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectError: true,
			errorMsg:    "install info must have tool, tool_version, and installer_version set",
		},
		{
			name: "empty tool version field",
			info: &InstallInfo{
				Tool:             "ECS",
				ToolVersion:      "",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectError: true,
			errorMsg:    "install info must have tool, tool_version, and installer_version set",
		},
		{
			name: "empty installer version field",
			info: &InstallInfo{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "",
			},
			expectError: true,
			errorMsg:    "install info must have tool, tool_version, and installer_version set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setRuntimeInstallInfo(tt.info)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				retrieved := getRuntimeInstallInfo()
				assert.NotNil(t, retrieved)

				if strings.Contains(tt.info.Tool, "password=hunter2") {
					assert.Contains(t, retrieved.Tool, "password=********")
				} else {
					assert.Equal(t, tt.info.Tool, retrieved.Tool)
				}

				if strings.Contains(tt.info.ToolVersion, "password=hunter2") {
					assert.Contains(t, retrieved.ToolVersion, "password=********")
				} else {
					assert.Equal(t, tt.info.ToolVersion, retrieved.ToolVersion)
				}

				if strings.Contains(tt.info.InstallerVersion, "password=hunter2") {
					assert.Contains(t, retrieved.InstallerVersion, "password=********")
				} else {
					assert.Equal(t, tt.info.InstallerVersion, retrieved.InstallerVersion)
				}
			}
		})
	}
}

func TestGetRuntimeInstallInfo(t *testing.T) {
	info := &InstallInfo{
		Tool:             "ECS",
		ToolVersion:      "1.0",
		InstallerVersion: "ecs-task-def-v1",
	}
	err := setRuntimeInstallInfo(info)
	require.NoError(t, err)

	retrieved := getRuntimeInstallInfo()
	assert.NotNil(t, retrieved)
	assert.Equal(t, info.Tool, retrieved.Tool)
	assert.Equal(t, info.ToolVersion, retrieved.ToolVersion)
	assert.Equal(t, info.InstallerVersion, retrieved.InstallerVersion)

	retrieved.Tool = "MODIFIED"
	retrieved2 := getRuntimeInstallInfo()
	assert.Equal(t, "ECS", retrieved2.Tool)
}
