// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			InstallMethod: installMethod{
				Tool:             "tool",
				ToolVersion:      "tool_version",
				InstallerVersion: "installer_version",
			},
		}
		expected[i] = versionHistoryEntry{
			Version:   fmt.Sprintf("%d", i+10),
			Timestamp: now.Add(time.Duration(i+10) * time.Second),
			InstallMethod: installMethod{
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
