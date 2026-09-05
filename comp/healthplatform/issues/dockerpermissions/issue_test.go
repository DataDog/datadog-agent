// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dockerpermissions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The only producer of this issue is the socket check, which reports the
// unreachable socket paths under "dockerDirs".
func TestBuildIssueFromSocketCheck(t *testing.T) {
	issue, err := NewDockerPermissionIssue().BuildIssue(map[string]string{
		"dockerDirs": "/var/run/docker.sock,/host/var/run/docker.sock",
		"os":         "linux",
	})

	require.NoError(t, err)
	require.NotNil(t, issue)

	assert.Empty(t, issue.Id, "Id is set by the caller (ReportIssue), not by the template")
	assert.Equal(t, IssueName, issue.IssueName)
	assert.Equal(t, IssueType, issue.IssueType)

	require.NotNil(t, issue.Extra)
	fields := issue.Extra.GetFields()
	assert.Equal(t, "docker", fields["integration"].GetStringValue())
	assert.Equal(t, "linux", fields["os"].GetStringValue())
	// The socket check passes socket paths, not directories.
	assert.Equal(t, "/var/run/docker.sock,/host/var/run/docker.sock", fields["dir_path"].GetStringValue())

	impact := fields["impact"].GetStringValue()
	assert.Contains(t, impact, "cannot query the Docker daemon")
	assert.Contains(t, impact, "container metrics")
	assert.Contains(t, impact, "container image vulnerability scanning")

	require.NotNil(t, issue.Remediation)
	assert.NotEmpty(t, issue.Remediation.Summary)
	assert.NotEmpty(t, issue.Remediation.Steps)
	require.NotNil(t, issue.Remediation.Script)
	assert.Equal(t, "bash", issue.Remediation.Script.Language)
	assert.Contains(t, issue.Remediation.Script.Content, `DOCKER_DIR="/var/run/docker.sock`,
		"the script template should be rendered, not returned as-is")
}

func TestBuildIssueWindowsRemediation(t *testing.T) {
	issue, err := NewDockerPermissionIssue().BuildIssue(map[string]string{
		"dockerDirs": `//./pipe/docker_engine`,
		"os":         "windows",
	})

	require.NoError(t, err)
	require.NotNil(t, issue)
	assert.Contains(t, issue.Tags, "windows")

	require.NotNil(t, issue.Remediation)
	require.NotNil(t, issue.Remediation.Script)
	assert.Equal(t, "powershell", issue.Remediation.Script.Language)
	assert.Contains(t, issue.Remediation.Script.Content, `$dockerDir = "//./pipe/docker_engine"`)
}
