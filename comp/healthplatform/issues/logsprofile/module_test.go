// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsprofile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func TestModuleRegistration(t *testing.T) {
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("host-a"))
	deps := issues.ModuleDeps{
		Config:   config.NewMockFromYAML(t, "logs_enabled: true\nhealth_platform:\n  logs_profile_recommendation:\n    interval: 90s"),
		Hostname: hn,
	}

	registry := issues.NewRegistry()
	for _, module := range issues.GetAllModules(deps) {
		registry.RegisterModule(module)
	}

	for name, typ := range map[string]string{IssueName: IssueType, SuggestedIssueName: SuggestedIssueType} {
		template, ok := registry.GetTemplate(name)
		require.True(t, ok, name)
		assert.Equal(t, typ, template.IssueType())
	}

	checks := registry.GetBuiltInPeriodicHealthChecks()
	require.Len(t, checks, 1, "one check serves both templates")
	assert.Equal(t, checkSource, checks[0].Source)
	assert.ElementsMatch(t, []string{IssueName, SuggestedIssueName}, checks[0].IssueNames)
	assert.Equal(t, 90*time.Second, checks[0].Interval)
	assert.Empty(t, registry.GetBuiltInStartupHealthChecks())
}
