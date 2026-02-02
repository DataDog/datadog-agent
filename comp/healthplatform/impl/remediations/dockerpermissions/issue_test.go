// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dockerpermissions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIssue_QuoteEscaping(t *testing.T) {
	tests := []struct {
		name               string
		dockerDir          string
		osName             string
		expectedInLinux    []string
		expectedInWindows  []string
		shouldContainQuote bool
	}{
		{
			name:      "Normal path - Linux",
			dockerDir: "/var/lib/docker",
			osName:    "linux",
			expectedInLinux: []string{
				"sudo setfacl -Rm g:dd-agent:rx '/var/lib/docker/containers'",
			},
			shouldContainQuote: false,
		},
		{
			name:      "Path with single quote - Linux",
			dockerDir: "/var/lib/dock'er",
			osName:    "linux",
			expectedInLinux: []string{
				"sudo setfacl -Rm g:dd-agent:rx '/var/lib/dock'\\''er/containers'",
			},
			shouldContainQuote: false,
		},
		{
			name:      "Normal path - Windows",
			dockerDir: "C:\\ProgramData\\docker",
			osName:    "windows",
			expectedInWindows: []string{
				"icacls \"C:\\ProgramData\\docker\\containers\"",
			},
			shouldContainQuote: false,
		},
		{
			name:      "Path with double quote - Windows",
			dockerDir: "C:\\Program\"Data\\docker",
			osName:    "windows",
			expectedInWindows: []string{
				"icacls \"C:\\Program\"\"Data\\docker\\containers\"",
			},
			shouldContainQuote: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issueTemplate := NewDockerPermissionIssue()
			context := map[string]string{
				"dockerDir": tt.dockerDir,
				"os":        tt.osName,
			}

			issue, err := issueTemplate.BuildIssue(context)
			require.NoError(t, err)
			require.NotNil(t, issue)
			require.NotNil(t, issue.Remediation)

			// Verify remediation steps contain expected escaped content
			var stepsText []string
			for _, step := range issue.Remediation.Steps {
				stepsText = append(stepsText, step.Text)
			}
			allSteps := strings.Join(stepsText, "\n")

			if tt.osName == "linux" {
				for _, expected := range tt.expectedInLinux {
					assert.Contains(t, allSteps, expected,
						"Linux remediation should contain properly escaped path")
				}
			} else if tt.osName == "windows" {
				for _, expected := range tt.expectedInWindows {
					assert.Contains(t, allSteps, expected,
						"Windows remediation should contain properly escaped path")
				}
			}

			// Verify the description doesn't break (even though it's just display text)
			assert.NotEmpty(t, issue.Description)
			assert.Contains(t, issue.Description, tt.dockerDir)
		})
	}
}

func TestBuildIssue_DefaultValues(t *testing.T) {
	issueTemplate := NewDockerPermissionIssue()

	// Test with empty context
	issue, err := issueTemplate.BuildIssue(map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, issue)

	// Should use default values
	assert.Equal(t, "docker-file-tailing-disabled", issue.Id)
	assert.Contains(t, issue.Description, "/var/lib/docker")
	assert.Contains(t, issue.Tags, "linux")
}

func TestBuildLinux_EscapesSingleQuotes(t *testing.T) {
	issueTemplate := NewDockerPermissionIssue()

	// Test path with single quote
	remediation := issueTemplate.buildLinux("/path/with'quote")

	// Find the setfacl command
	var setfaclCmd string
	for _, step := range remediation.Steps {
		if strings.Contains(step.Text, "setfacl") {
			setfaclCmd = step.Text
			break
		}
	}

	// Should escape single quote as '\''
	assert.Contains(t, setfaclCmd, "'/path/with'\\''quote/containers'")
}

func TestBuildWindows_EscapesDoubleQuotes(t *testing.T) {
	issueTemplate := NewDockerPermissionIssue()

	// Test path with double quote
	remediation := issueTemplate.buildWindows("C:\\path\\with\"quote")

	// Find the icacls command
	var icaclsCmd string
	for _, step := range remediation.Steps {
		if strings.Contains(step.Text, "icacls") {
			icaclsCmd = step.Text
			break
		}
	}

	// Should escape double quote by doubling it
	assert.Contains(t, icaclsCmd, "\"C:\\path\\with\"\"quote\\containers\"")
}

func TestRenderTemplate_WithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name      string
		dockerDir string
	}{
		{"Normal path", "/var/lib/docker"},
		{"Path with spaces", "/var/lib/my docker"},
		{"Path with quotes", "/var/lib/dock'er"},
		{"Windows path", "C:\\ProgramData\\docker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Linux template
			linuxResult := renderTemplate(linuxScriptTemplate, tt.dockerDir)
			assert.Contains(t, linuxResult, tt.dockerDir)

			// Test Windows template
			windowsResult := renderTemplate(windowsScriptTemplate, tt.dockerDir)
			assert.Contains(t, windowsResult, tt.dockerDir)
		})
	}
}
