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

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
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
	assert.Equal(t, "logs-agent", issue.GetLocation())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.GetSeverity())
	assert.Equal(t, "agent", issue.GetSource())
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
	stepText := joinStepText(remediation.GetSteps())
	assert.Contains(t, stepText, "docker-users")
	assert.Contains(t, stepText, "daemon.json")

	script := remediation.GetScript()
	require.NotNil(t, script)
	assert.Equal(t, "powershell", script.GetLanguage())
	assert.Equal(t, "Fix-DockerSocketPermissions.ps1", script.GetFilename())
	assert.Contains(t, script.GetContent(), "docker-users")
	// The script must not assume "docker-users" is wired up to the pipe ACL:
	// it has to read the daemon's actual configured group before adding the
	// user to it, since standalone Docker Engine installs don't set one.
	assert.Contains(t, script.GetContent(), "daemon.json")
	assert.Contains(t, script.GetContent(), "$targetGroup")
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

	check := m.BuiltInPeriodicHealthCheck()
	require.NotNil(t, check)
	assert.Equal(t, "docker", check.Source)
	assert.Equal(t, []string{SocketUnavailableIssueName}, check.IssueNames,
		"the shared check must pre-seed the unavailable module's name so bundle.go's restart seeding covers both issue names")
	assert.Nil(t, m.BuiltInStartupHealthCheck())
}

func TestInstanceIssueID_DiffersByHostname(t *testing.T) {
	hn1, _ := hostnamemock.NewMock("host-a")
	hn2, _ := hostnamemock.NewMock("host-b")

	c1 := newChecker(hn1)
	c2 := newChecker(hn2)

	assert.NotEqual(t, c1.instanceIssueID(IssueID), c2.instanceIssueID(IssueID))
}

func TestInstanceIssueID_PrefixedByBaseID(t *testing.T) {
	hn, _ := hostnamemock.NewMock("host-a")
	c := newChecker(hn)

	assert.True(t, strings.HasPrefix(c.instanceIssueID(IssueID), IssueID+":"))
	assert.True(t, strings.HasPrefix(c.instanceIssueID(SocketUnavailableIssueID), SocketUnavailableIssueID+":"))
	assert.NotEqual(t, c.instanceIssueID(IssueID), c.instanceIssueID(SocketUnavailableIssueID))
}

func TestBuildIssue_SocketUnavailable_Defaults(t *testing.T) {
	template := NewDockerSocketUnavailableIssue()
	issue, err := template.BuildIssue(map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, issue)

	assert.Equal(t, SocketUnavailableIssueName, issue.GetIssueName())
	assert.Equal(t, SocketUnavailableIssueType, issue.GetIssueType())
	assert.Contains(t, issue.GetTitle(), "/var/run/docker.sock")
	assert.Contains(t, issue.GetDescription(), "not a permission error")
	assert.Equal(t, "availability", issue.GetCategory())
	assert.Equal(t, "logs-agent", issue.GetLocation())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.GetSeverity())
	assert.Equal(t, "agent", issue.GetSource())
	assert.Contains(t, issue.GetTags(), "docker")
	assert.Contains(t, issue.GetTags(), "linux")

	require.NotNil(t, issue.GetRemediation())
	assert.NotEmpty(t, issue.GetRemediation().GetSummary())
	require.NotEmpty(t, issue.GetRemediation().GetSteps())
	assert.Nil(t, issue.GetRemediation().GetScript())
}

func TestBuildIssue_SocketUnavailable_Linux(t *testing.T) {
	template := NewDockerSocketUnavailableIssue()
	issue, err := template.BuildIssue(map[string]string{
		"socketPaths": "/var/run/docker.sock,/host/var/run/docker.sock",
		"os":          "linux",
	})
	require.NoError(t, err)

	assert.Contains(t, issue.GetTitle(), "/var/run/docker.sock,/host/var/run/docker.sock")

	remediation := issue.GetRemediation()
	require.NotNil(t, remediation)
	assert.Contains(t, joinStepText(remediation.GetSteps()), "systemctl status docker")
	assert.Nil(t, remediation.GetScript())
}

func TestBuildIssue_SocketUnavailable_Windows(t *testing.T) {
	template := NewDockerSocketUnavailableIssue()
	issue, err := template.BuildIssue(map[string]string{
		"socketPaths": "//./pipe/docker_engine",
		"os":          "windows",
	})
	require.NoError(t, err)
	assert.Contains(t, issue.GetTags(), "windows")

	remediation := issue.GetRemediation()
	require.NotNil(t, remediation)
	stepText := joinStepText(remediation.GetSteps())
	assert.Contains(t, stepText, "Get-Service docker")
	assert.Nil(t, remediation.GetScript())
}

func TestBuildIssue_SocketUnavailable_Extra(t *testing.T) {
	template := NewDockerSocketUnavailableIssue()
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

func TestNewSocketUnavailableModule(t *testing.T) {
	m := NewSocketUnavailableModule(issues.ModuleDeps{})
	assert.Equal(t, SocketUnavailableIssueName, m.IssueName())
	assert.Equal(t, SocketUnavailableIssueType, m.IssueType())

	issue, err := m.BuildIssue(map[string]string{})
	require.NoError(t, err)
	assert.NotNil(t, issue)

	assert.Nil(t, m.BuiltInPeriodicHealthCheck())
	assert.Nil(t, m.BuiltInStartupHealthCheck())
}
