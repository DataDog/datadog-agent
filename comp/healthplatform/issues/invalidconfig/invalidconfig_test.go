// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	assert.Equal(t, "Open /etc/datadog-agent/datadog.yaml in an editor.", issue.Remediation.Steps[0].Text)
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

func TestBuildIssue_MissingConfigPath(t *testing.T) {
	issue, err := InvalidConfigIssue{}.BuildIssue(map[string]string{contextKeyErrorCount: "1"})
	require.NoError(t, err)
	assert.Equal(t, "Found 1 error in the Agent configuration.", issue.Description)
	assert.Equal(t, "Check the settings listed below in your Agent configuration file or environment variables.", issue.Remediation.Steps[0].Text)
	assert.Equal(t, "(unknown path)", issue.Extra.GetFields()[contextKeyConfigPath].GetStringValue())
}

func TestBuildIssue_Remediation(t *testing.T) {
	const fallback = "Fix each violation listed in the description."
	for _, tc := range []struct{ name, violations, description, want string }{
		{"integer_default", `[{"path":"/dogstatsd_port","actual_type":"object","expected_types":["integer"],"default_status":"known","default_value":8125}]`,
			"`/dogstatsd_port` expects a whole number, but received a YAML mapping.",
			"Set `/dogstatsd_port` to a whole number. The default value for this setting is `8125`."},
		{"empty_default", `[{"path":"/api_key","actual_type":"array","expected_types":["string"],"default_status":"known","default_value":""}]`,
			"`/api_key` expects a string, but received a YAML list.",
			"Set `/api_key` to a string. The default value for this setting is `\"\"` (an empty string)."},
		{"no_default", `[{"path":"/agent_ipc","actual_type":"string","expected_types":["object"],"default_status":"none"}]`,
			"`/agent_ipc` expects a YAML mapping, but received a string.",
			"Set `/agent_ipc` to a YAML mapping. This setting has no default."},
		{"unknown_default", `[{"path":"/additional_endpoints/example","actual_type":"null","expected_types":["array"],"default_status":"unknown"}]`,
			"`/additional_endpoints/example` expects a YAML list, but received null.",
			"Set `/additional_endpoints/example` to a YAML list."},
		{"union", `[{"path":"/setting","actual_type":"boolean","expected_types":["integer","number","string"],"default_status":"unknown"}]`,
			"`/setting` expects a whole number, a number, or a string, but received true or false.",
			"Set `/setting` to a whole number, a number, or a string."},
		{"markdown", "[{\"path\":\"/key`[link](https://example.test)\\n\",\"actual_type\":\"number\",\"expected_types\":[\"string\"],\"default_status\":\"known\",\"default_value\":\"`example`\"}]",
			"``/key`[link](https://example.test) `` expects a string, but received a number.",
			"Set ``/key`[link](https://example.test) `` to a string. The default value for this setting is ``\"`example`\"``."},
		{"empty_facts", "[]", "", fallback},
		{"malformed_facts", "not JSON", "", fallback},
		{"incomplete_facts", "[{}]", "", fallback},
		{"unsupported_type", `[{"path":"/setting","actual_type":"string","expected_types":["unsupported"],"default_status":"unknown"}]`, "", fallback},
		{"missing_default", `[{"path":"/setting","actual_type":"string","expected_types":["integer"],"default_status":"known"}]`, "", fallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue, err := InvalidConfigIssue{}.BuildIssue(map[string]string{
				contextKeyConfigPath: "/etc/datadog-agent/datadog.yaml",
				contextKeyErrorCount: "1",
				contextErrorKey(0):   "at '/setting': configuration does not match schema",
				contextKeyViolations: tc.violations,
			})
			require.NoError(t, err)
			description := tc.description
			if description == "" {
				description = "at '/setting': configuration does not match schema"
			}
			assert.Equal(t, "Found 1 configuration error in /etc/datadog-agent/datadog.yaml: "+description, issue.Description)
			assert.Equal(t, tc.want, issue.Remediation.Steps[1].Text)
		})
	}
}

func TestBuildIssue_MultipleCorrections(t *testing.T) {
	const count = 12
	violations := make([]violationPayload, count)
	for i := range violations {
		violations[i] = violationPayload{Path: fmt.Sprintf("/setting%d", i), ActualType: "string", ExpectedTypes: []string{"integer"}, DefaultStatus: "unknown"}
	}
	raw, err := json.Marshal(violations)
	require.NoError(t, err)
	issue, err := InvalidConfigIssue{}.BuildIssue(map[string]string{
		contextKeyErrorCount: strconv.Itoa(count),
		contextKeyViolations: string(raw),
	})
	require.NoError(t, err)
	text := issue.Remediation.Steps[1].Text
	assert.True(t, strings.HasPrefix(text, "- Set `/setting0` to a whole number."))
	assert.Equal(t, count, strings.Count(text, "to a whole number."))
	assert.True(t, strings.HasSuffix(text, "Set `/setting11` to a whole number."), text)
	assert.Contains(t, text, "Set `/setting10` to a whole number.")
	assert.Equal(t, count, strings.Count(issue.Description, "expects a whole number, but received a string."))
	assert.True(t, strings.HasPrefix(issue.Description, "Found 12 errors in the Agent configuration:"))
	assert.Contains(t, issue.Description, "`/setting11`")
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
	cfg.SetInTest("agent_ipc.port", "not-a-number")

	reports, err := newChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
	if err != nil {
		t.Skipf("schema validator unavailable (schema not embedded in test binary): %v", err)
	}
	require.Len(t, reports, 1)
	assert.Equal(t, IssueName, reports[0].IssueName)
	assert.True(t, strings.HasPrefix(reports[0].IssueID, IssueID+":"), "IssueID %q must be scoped with a host+path suffix", reports[0].IssueID)
	assert.Contains(t, reports[0].Context[contextErrorKey(0)], "agent_ipc/port")

	issue, err := InvalidConfigIssue{}.BuildIssue(reports[0].Context)
	require.NoError(t, err)
	assert.Equal(t, "Set `/agent_ipc/port` to a whole number. The default value for this setting is `0`.", issue.Remediation.Steps[1].Text)
	fields := issue.GetExtra().GetFields()
	issueViolations := fields[contextKeyViolations].GetListValue().GetValues()
	require.Len(t, issueViolations, 1)
	assert.Equal(t, map[string]any{
		"path":           "/agent_ipc/port",
		"actual_type":    "string",
		"expected_types": []any{"integer"},
		"default_status": "known",
		"default_value":  float64(0),
	}, issueViolations[0].GetStructValue().AsMap())
}

func TestCheck_SecretHandlingPreservesTypeViolations(t *testing.T) {
	requireSchema(t)
	for _, testCase := range []struct {
		yaml string
		want string
	}{
		{"forwarder_apikey_validation_interval: 61\n", ""},
		{"api_key: [secret]\n", "got array, want string"},
		{"api_key: {value: secret}\n", "got object, want string"},
		{"additional_endpoints: {'https://qa:RAW_URL_PASSWORD_7c81@example.test': [false]}\n", "got boolean, want string"},
		{"agent_ipc:\n  port: ENC[ipc_port]\n", "got string, want integer"},
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
	for _, tc := range []struct{ name, value, want string }{
		{"valid_integer", "5001", ""},
		{"invalid_integer", "SECRET_INVALID_INTEGER", "got string, want integer"},
		{"resolved_enc_literal", "ENC[SECRET_LITERAL]", "got string, want integer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewMockFromYAML(t, "agent_ipc:\n  port: ENC[value]\n")
			cfg.Set("agent_ipc.port", tc.value, model.SourceSecret)

			reports, err := newChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
			require.NoError(t, err)
			if tc.want == "" {
				assert.Empty(t, reports)
				return
			}
			require.Len(t, reports, 1)
			assert.Equal(t, "at '/agent_ipc/port': "+tc.want, reports[0].Context[contextErrorKey(0)])
			issue, err := InvalidConfigIssue{}.BuildIssue(reports[0].Context)
			require.NoError(t, err)
			encoded, err := json.Marshal(issue)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), tc.value)
		})
	}
}

func TestResolveDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, "agent_ipc: invalid")
	for _, tc := range []struct{ path, status string }{
		{"/agent_ipc", "none"},
		{"/unknown_setting", "unknown"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			status, value := resolveDefault(cfg, tc.path)
			assert.Equal(t, tc.status, status)
			assert.Nil(t, value)
		})
	}
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
