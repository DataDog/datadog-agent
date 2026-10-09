// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package secretresolution

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

type testResolver struct {
	secrets.Component
	failures []secrets.ResolutionFailure
}

func (r *testResolver) GetResolutionFailures() []secrets.ResolutionFailure { return r.failures }

func TestSecretResolutionIssue(t *testing.T) {
	r := &testResolver{failures: []secrets.ResolutionFailure{{
		Handle: "redis-password", Origin: "config-id", OriginName: "redis", Path: []string{"password"},
	}}}
	hostname, _ := hostnamemock.NewMock("qa-host")
	m := NewModule(issues.ModuleDeps{Secrets: r, Hostname: hostname})
	check := m.BuiltInPeriodicHealthCheck()
	for _, tc := range []struct{ reason, description, correction string }{
		{"missing", "did not return a value", "returns an entry"},
		{"empty", "returned an empty value", "non-empty value"},
		{"invalid_response", "returned an unreadable response", "valid JSON"},
		{"backend_error", "could not resolve", "access permissions"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			r.failures[0].Reason = tc.reason
			reports, err := check.Fn()
			require.NoError(t, err)
			require.Len(t, reports, 1)
			issue, err := m.BuildIssue(reports[0].Context)
			require.NoError(t, err)
			assert.Equal(t, IssueType, issue.IssueType)
			assert.Equal(t, "Failed to resolve \"ENC[redis-password]\" in redis", issue.Title)
			assert.Contains(t, issue.Description, tc.description)
			assert.Contains(t, issue.Description, "ENC[redis-password]")
			assert.Contains(t, issue.Description, "/password")
			assert.Len(t, issue.Remediation.Steps, 3)
			assert.Contains(t, issue.Remediation.Steps[1].Text, tc.correction)
			assert.Equal(t, "/password", issue.Extra.AsMap()["setting_path"])
		})
	}
	r.failures[0].HasCachedValue = true
	reports, err := check.Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	issue, err := m.BuildIssue(reports[0].Context)
	require.NoError(t, err)
	assert.Contains(t, issue.Description, "previously resolved value")
	id := reports[0].IssueID
	r.failures = nil
	reports, err = check.Fn()
	require.NoError(t, err)
	assert.Empty(t, reports)
	r.failures = []secrets.ResolutionFailure{{Handle: "redis-password", Origin: "config-id", Path: []string{"password"}, Reason: "empty"}}
	reports, err = check.Fn()
	require.NoError(t, err)
	assert.Equal(t, id, reports[0].IssueID, "recovery and recurrence preserve the issue ID")
	otherHostname, _ := hostnamemock.NewMock("other-host")
	otherHost := NewModule(issues.ModuleDeps{Secrets: r, Hostname: otherHostname})
	otherReports, err := otherHost.BuiltInPeriodicHealthCheck().Fn()
	require.NoError(t, err)
	assert.NotEqual(t, id, otherReports[0].IssueID)
}

func TestSecretResolutionScrubsReferences(t *testing.T) {
	r := &testResolver{failures: []secrets.ResolutionFailure{{
		Handle: "https://user:private-password@example.com/secret",
		Origin: "config-id", OriginName: "redis", Reason: "backend_error",
		Path: []string{"https://user:private-password@example.com", "token"},
	}}}
	hostname, _ := hostnamemock.NewMock("qa-host")
	m := NewModule(issues.ModuleDeps{Secrets: r, Hostname: hostname})
	reports, err := m.BuiltInPeriodicHealthCheck().Fn()
	require.NoError(t, err)
	issue, err := m.BuildIssue(reports[0].Context)
	require.NoError(t, err)
	encoded, err := json.Marshal(issue)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-password")
	assert.Contains(t, string(encoded), "example.com")
}
