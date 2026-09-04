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

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func joinStepText(steps []*healthplatform.RemediationStep) string {
	texts := make([]string, len(steps))
	for i, step := range steps {
		texts[i] = step.GetText()
	}
	return strings.Join(texts, "\n")
}

func TestBuildIssue_Defaults(t *testing.T) {
	template := NewDockerPermissionIssue()
	issue, err := template.BuildIssue(map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, issue)

	assert.Equal(t, IssueName, issue.GetIssueName())
	assert.Equal(t, IssueType, issue.GetIssueType())
	assert.Contains(t, issue.GetTitle(), "/var/run/docker.sock")
	assert.Contains(t, issue.GetDescription(), "permission")
	assert.Equal(t, "permissions", issue.GetCategory())
	assert.Equal(t, "agent", issue.GetLocation())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.GetSeverity())
	assert.Equal(t, "docker", issue.GetSource())
	assert.Contains(t, issue.GetTags(), "docker")
	assert.Contains(t, issue.GetTags(), "linux")

	require.NotNil(t, issue.GetRemediation())
	assert.NotEmpty(t, issue.GetRemediation().GetSummary())
	require.NotEmpty(t, issue.GetRemediation().GetSteps())
}

func TestBuildIssue_Linux(t *testing.T) {
	template := NewDockerPermissionIssue()
	issue, err := template.BuildIssue(map[string]string{
		"socketPaths": "/var/run/docker.sock,/host/var/run/docker.sock",
		"os":          "linux",
	})
	require.NoError(t, err)

	assert.Contains(t, issue.GetTitle(), "/var/run/docker.sock,/host/var/run/docker.sock")

	remediation := issue.GetRemediation()
	require.NotNil(t, remediation)
	assert.Contains(t, joinStepText(remediation.GetSteps()), "usermod -aG docker dd-agent")

	script := remediation.GetScript()
	require.NotNil(t, script)
	assert.Equal(t, "bash", script.GetLanguage())
	assert.Equal(t, "fix-docker-socket-permissions.sh", script.GetFilename())
	assert.Contains(t, script.GetContent(), "usermod -aG docker dd-agent")
}

func TestBuildIssue_Windows(t *testing.T) {
	template := NewDockerPermissionIssue()
	issue, err := template.BuildIssue(map[string]string{
		"socketPaths": "//./pipe/docker_engine",
		"os":          "windows",
	})
	require.NoError(t, err)
	assert.Contains(t, issue.GetTags(), "windows")

	remediation := issue.GetRemediation()
	require.NotNil(t, remediation)
	assert.Contains(t, joinStepText(remediation.GetSteps()), "docker-users")

	script := remediation.GetScript()
	require.NotNil(t, script)
	assert.Equal(t, "powershell", script.GetLanguage())
	assert.Equal(t, "Fix-DockerSocketPermissions.ps1", script.GetFilename())
	assert.Contains(t, script.GetContent(), "docker-users")
}

func TestBuildIssue_Extra(t *testing.T) {
	template := NewDockerPermissionIssue()
	issue, err := template.BuildIssue(map[string]string{
		"socketPaths": "/var/run/docker.sock",
		"os":          "linux",
	})
	require.NoError(t, err)

	require.NotNil(t, issue.GetExtra())
	fields := issue.GetExtra().GetFields()
	assert.Equal(t, "docker", fields["integration"].GetStringValue())
	assert.Equal(t, "/var/run/docker.sock", fields["socket_paths"].GetStringValue())
	assert.Equal(t, "linux", fields["os"].GetStringValue())
	assert.NotEmpty(t, fields["impact"].GetStringValue())
}

func TestNewModule(t *testing.T) {
	m := NewModule(issues.ModuleDeps{})
	assert.Equal(t, IssueName, m.IssueName())
	assert.Equal(t, IssueType, m.IssueType())

	issue, err := m.BuildIssue(map[string]string{})
	require.NoError(t, err)
	assert.NotNil(t, issue)

	require.NotNil(t, m.BuiltInPeriodicHealthCheck())
	assert.Equal(t, "docker", m.BuiltInPeriodicHealthCheck().Source)
	assert.Nil(t, m.BuiltInStartupHealthCheck())
}
