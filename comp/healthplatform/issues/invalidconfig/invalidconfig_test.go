// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

func testHostname(t *testing.T) hostnameinterface.Component {
	t.Helper()
	hn, _ := hostnamemock.NewMock("test-host")
	return hn
}

func testSelfIdent(t *testing.T) *selfident.SelfIdent {
	t.Helper()
	return selfident.New(nil)
}

// requireSchema skips the test when the compressed schema files haven't been
// generated yet (run `dda inv schema.compress`). CI always has them; local
// dev builds do not unless explicitly generated.
func requireSchema(t *testing.T) {
	t.Helper()
	if _, err := schema.GetCoreSchema(); err != nil {
		t.Skipf("embedded schema not available (%v); run `dda inv schema.compress`", err)
	}
}

func TestBuildIssue_SchemaViolationProducesMediumSeverity(t *testing.T) {
	ctx := map[string]string{
		contextKeyConfigPath: "/etc/datadog-agent/datadog.yaml",
		contextKeyErrorCount: "2",
	}
	ctx[contextErrorKey(0)] = "at '/agent_ipc/port': got string, want integer"
	ctx[contextErrorKey(1)] = "at '/tags': got object, want array"
	issue, err := InvalidConfigIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Empty(t, issue.GetId(), "Id is set by the runner (ReportIssue), not by the template")
	assert.Equal(t, IssueName, issue.GetIssueName())
	assert.Equal(t, IssueType, issue.GetIssueType())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.GetSeverity())
	assert.Equal(t, "Datadog Agent Configuration Has 2 Schema Violations in datadog.yaml", issue.GetTitle())
	assert.Equal(t, float64(2),
		issue.GetExtra().GetFields()[contextKeyErrorCount].GetNumberValue())
	assert.Contains(t, issue.GetDescription(), "agent_ipc/port")
	assert.Contains(t, issue.GetDescription(), "/tags")
	assert.Contains(t, issue.GetDescription(), "; ", "description must use a visible delimiter between violations so the UI renders them legibly")

	errorsStruct := issue.GetExtra().GetFields()[contextKeyErrors].GetStructValue()
	require.NotNil(t, errorsStruct, "extra.errors must be a struct with one entry per violation")
	assert.Len(t, errorsStruct.GetFields(), 2, "each violation must get its own key")
	assert.Equal(t, "got string, want integer", errorsStruct.GetFields()["/agent_ipc/port"].GetListValue().GetValues()[0].GetStringValue())
	assert.Equal(t, "got object, want array", errorsStruct.GetFields()["/tags"].GetListValue().GetValues()[0].GetStringValue())
}

// A vanilla mock has only defaults, which round-trip through YAML cleanly and
// pass the schema. Confirms Run() is a no-op on a healthy config.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	reports, err := newChecker(config.NewMock(t), testHostname(t), testSelfIdent(t)).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// Duration strings must remain accepted for both dotted and nested config keys.
func TestCheck_DurationStringIsNotAViolation(t *testing.T) {
	for _, yaml := range []string{
		"remote_configuration.refresh_interval: 5s\n",   // flat dotted key
		"remote_configuration:\n  refresh_interval: 5s", // nested key
	} {
		cfg := config.NewMockFromYAML(t, yaml)
		reports, err := newChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
		require.NoError(t, err)
		assert.Empty(t, reports, "duration string %q should not produce a schema violation: %+v", yaml, reports)
	}
}

// Inject a string into an integer-typed field. Confirms the validator surfaces
// the violation and the checker wraps it into an IssueReport.
func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	requireSchema(t)
	cfg := config.NewMock(t)
	cfg.SetInTest("agent_ipc.port", "RAW_VALUE_MUST_NOT_APPEAR_7c81")

	reports, err := newChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
	if err != nil {
		t.Skipf("schema validator unavailable (schema not embedded in test binary): %v", err)
	}
	require.Len(t, reports, 1)
	assert.Equal(t, IssueName, reports[0].IssueName)
	assert.True(t, strings.HasPrefix(reports[0].IssueID, IssueID+":"), "IssueID %q must be scoped with a host+path suffix", reports[0].IssueID)
	assert.Equal(t, "at '/agent_ipc/port': got string, want integer", reports[0].Context[contextErrorKey(0)])
	issue, err := InvalidConfigIssue{}.BuildIssue(reports[0].Context)
	require.NoError(t, err)
	encoded, err := json.Marshal(issue)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "RAW_VALUE_MUST_NOT_APPEAR_7c81")
}

func TestCheck_SecretHandlingPreservesTypeViolations(t *testing.T) {
	requireSchema(t)
	for _, testCase := range []struct {
		yaml string
		want string
	}{
		{"forwarder_apikey_validation_interval: 61\n", ""},
		{"forwarder_apikey_validation_interval: [61]\n", "got array, want integer"},
		{"forwarder_apikey_validation_interval: {value: 61}\n", "got object, want integer"},
		{"api_key: [secret]\n", "got array, want string"},
		{"additional_endpoints: {'https://example.test': [false]}\n", "got boolean, want string"},
		{"additional_endpoints: {'https://qa:RAW_URL_PASSWORD_7c81@example.test': [false]}\n", "got boolean, want string"},
		{"agent_ipc:\n  port: ENC[ipc_port]\n", "got string, want integer"},
		{"logs_enabled: ENC[enabled]\n", "got string, want boolean"},
		{"forwarder_backoff_factor: ENC[factor]\n", "got string, want number"},
		{"api_key: ENC[key]\n", ""},
		{"agent_ipc: ENC[ipc]\n", "got string, want object"},
	} {
		t.Run(testCase.yaml, func(t *testing.T) {
			cfg := config.NewMockFromYAML(t, testCase.yaml)

			reports, err := newChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
			require.NoError(t, err)
			if testCase.want == "" {
				assert.Empty(t, reports)
				return
			}
			require.Len(t, reports, 1)
			assert.Contains(t, reports[0].Context[contextErrorKey(0)], testCase.want)
			issue, err := InvalidConfigIssue{}.BuildIssue(reports[0].Context)
			require.NoError(t, err)
			encoded, err := json.Marshal(issue)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "RAW_URL_PASSWORD_7c81")
			assert.NotContains(t, string(encoded), "ENC[")
		})
	}
}

func TestCheck_ResolvedSecrets(t *testing.T) {
	requireSchema(t)
	for _, tc := range []struct{ name, key, value, want string }{
		{"valid_integer", "agent_ipc.port", "5001", ""},
		{"invalid_integer", "agent_ipc.port", "SECRET_INVALID_INTEGER", "got string, want integer"},
		{"valid_boolean", "logs_enabled", "true", ""},
		{"invalid_boolean", "logs_enabled", "SECRET_INVALID_BOOLEAN", "got string, want boolean"},
		{"valid_number", "forwarder_backoff_factor", "2.5", ""},
		{"invalid_number", "forwarder_backoff_factor", "SECRET_INVALID_NUMBER", "got string, want number"},
		{"valid_duration", "remote_configuration.refresh_interval", "5s", ""},
		{"api_key_string", "api_key", "SECRET_API_KEY", ""},
		{"resolved_enc_literal", "agent_ipc.port", "ENC[SECRET_LITERAL]", "got string, want integer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewMockFromYAML(t, tc.key+": ENC[value]\n")
			cfg.Set(tc.key, tc.value, model.SourceSecret)

			reports, err := newChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
			require.NoError(t, err)
			if tc.want == "" {
				assert.Empty(t, reports)
				return
			}
			require.Len(t, reports, 1)
			assert.Equal(t, "at '/"+strings.ReplaceAll(tc.key, ".", "/")+"': "+tc.want, reports[0].Context[contextErrorKey(0)])
			issue, err := InvalidConfigIssue{}.BuildIssue(reports[0].Context)
			require.NoError(t, err)
			encoded, err := json.Marshal(issue)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), tc.value)
		})
	}
}

func TestBuildIssueReportContext_MixedViolationsAreValueFree(t *testing.T) {
	violations := []schema.Violation{
		{
			Message:       "at '/agent_ipc/port': got string, want integer",
			Path:          "/agent_ipc/port",
			ActualType:    "string",
			ExpectedTypes: []string{"integer"},
		},
		{Message: "at '/unknown': RAW_SECRET_MUST_NOT_APPEAR does not match pattern", Path: "/unknown"},
	}

	ctx := buildIssueReportContext("/etc/datadog-agent/datadog.yaml", violations)
	assert.Equal(t, violations[0].Message, ctx[contextErrorKey(0)])
	assert.Equal(t, "at '/unknown': configuration does not match schema", ctx[contextErrorKey(1)])
	issue, err := InvalidConfigIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	encoded, err := json.Marshal(issue)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "RAW_SECRET_MUST_NOT_APPEAR")
}

// Two checkers with the same hostname but different config files must not
// collide — this is the scenario where core agent and cluster-agent, both on
// the same host, validate their own distinct config file.
func TestInstanceIssueID_DiffersByConfigPath(t *testing.T) {
	hn := testHostname(t)
	cfg1 := config.NewMockFromYAML(t, "config_path_marker: a")
	cfg2 := config.NewMockFromYAML(t, "config_path_marker: b")

	si := testSelfIdent(t)
	c1 := newChecker(cfg1, hn, si)
	c2 := newChecker(cfg2, hn, si)
	c1.cfg = fakeConfigFileUsed{Component: cfg1, path: "/etc/datadog-agent/datadog.yaml"}
	c2.cfg = fakeConfigFileUsed{Component: cfg2, path: "/etc/datadog-agent/datadog-cluster.yaml"}

	assert.NotEqual(t, c1.instanceIssueID(), c2.instanceIssueID())
}

// Two checkers with the same config path but different hostnames must not
// collide — this is the org-wide aggregation scenario where a downstream
// consumer keys recommendations on (org, IssueID) alone.
func TestInstanceIssueID_DiffersByHostname(t *testing.T) {
	cfg := config.NewMock(t)
	hn1, _ := hostnamemock.NewMock("host-a")
	hn2, _ := hostnamemock.NewMock("host-b")

	si := testSelfIdent(t)
	c1 := newChecker(cfg, hn1, si)
	c2 := newChecker(cfg, hn2, si)

	assert.NotEqual(t, c1.instanceIssueID(), c2.instanceIssueID())
}

// fakeConfigFileUsed overrides ConfigFileUsed so instanceIssueID can be tested
// against a path without depending on how config.NewMockFromYAML resolves it.
type fakeConfigFileUsed struct {
	config.Component
	path string
}

func (f fakeConfigFileUsed) ConfigFileUsed() string {
	return f.path
}
