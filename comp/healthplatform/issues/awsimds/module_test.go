// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package awsimds

import (
	"strings"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

func testDeps(t *testing.T, hostname string) issues.ModuleDeps {
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostname))
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	return issues.ModuleDeps{Hostname: hn, Config: cfg}
}

// testModule builds the module directly, bypassing NewModule's environment gate.
func testModule(t *testing.T, hostname string) *awsIMDSModule {
	return newModule(testDeps(t, hostname))
}

func TestInstanceIssueID(t *testing.T) {
	m := testModule(t, "host-a")
	id := m.instanceIssueID()
	assert.True(t, strings.HasPrefix(id, IssueID+":"))
	assert.Len(t, id, len(IssueID)+1+16)
	assert.Equal(t, id, testModule(t, "host-a").instanceIssueID())
	assert.NotEqual(t, id, testModule(t, "host-b").instanceIssueID())
}

// TestNewModule_RegistrationGate verifies the module registers only inside a container (an immutable
// condition); the config-dependent AWS gate is evaluated in check(), not at registration.
func TestNewModule_RegistrationGate(t *testing.T) {
	deps := testDeps(t, "host")

	t.Run("containerized registers", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "true")
		assert.NotNil(t, NewModule(deps))
	})

	t.Run("not containerized declines", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "")
		assert.Nil(t, NewModule(deps))
	})
}

// TestCheck_NotAWS verifies check() resolves to no issue when DMI/UUID detection reports non-AWS,
// so a stored issue can still resolve after a restart even though the module stays registered.
func TestCheck_NotAWS(t *testing.T) {
	dmi.SetupMock(t, "", "", "", "not AWS")
	m := testModule(t, "host")

	reports, err := m.check()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// TestCheck_CloudProviderDisabled verifies the probe is skipped when AWS metadata collection is disabled.
func TestCheck_CloudProviderDisabled(t *testing.T) {
	dmi.SetupMock(t, "", "", "", ec2.DMIBoardVendor)
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("h"))
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	cfg.SetInTest("cloud_provider_metadata", []string{"gcp"})
	m := newModule(issues.ModuleDeps{Hostname: hn, Config: cfg})

	reports, err := m.check()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// TestBuildIssue_SeverityByHostnameConfig verifies severity drops to medium once a hostname is configured.
func TestBuildIssue_SeverityByHostnameConfig(t *testing.T) {
	tmpl := NewAWSIMDSIssue()

	high, err := tmpl.BuildIssue(map[string]string{contextKeyHostnameConfigured: "false"})
	require.NoError(t, err)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, high.Severity)

	medium, err := tmpl.BuildIssue(map[string]string{contextKeyHostnameConfigured: "true"})
	require.NoError(t, err)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, medium.Severity)
}
