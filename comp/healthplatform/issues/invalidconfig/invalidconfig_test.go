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
	schemaimpl "github.com/DataDog/datadog-agent/comp/core/schema/impl"
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
	if _, err := schema.GetRawSchema(model.CoreSchema); err != nil {
		t.Skipf("embedded schema not available (%v); run `dda inv schema.compress`", err)
	}
}

func TestBuildIssue_SchemaViolationProducesMediumSeverity(t *testing.T) {
	ctx := BuildContext(config.NewMock(t), "/etc/datadog-agent/datadog.yaml", &model.Violation{
		Path: "/agent_ipc/port", ActualType: "string", ExpectedTypes: []string{"integer"},
	})
	issue, err := InvalidConfigIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Empty(t, issue.GetId(), "Id is set by the runner (ReportIssue), not by the template")
	assert.Equal(t, IssueName, issue.GetIssueName())
	assert.Equal(t, IssueType, issue.GetIssueType())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.GetSeverity())
	assert.Equal(t, "Incorrect type for `/agent_ipc/port`", issue.GetTitle())
	assert.Equal(t, "Check this setting in `/etc/datadog-agent/datadog.yaml` or environment variables.", issue.Remediation.Steps[0].Text)
	assert.Equal(t, float64(1),
		issue.GetExtra().GetFields()[contextKeyErrorCount].GetNumberValue())
	assert.Contains(t, issue.GetDescription(), "agent_ipc/port")

	errorsStruct := issue.GetExtra().GetFields()[contextKeyErrors].GetStructValue()
	require.NotNil(t, errorsStruct, "extra.errors must be a struct with one entry per violation")
	assert.Len(t, errorsStruct.GetFields(), 1)
	assert.Equal(t, issue.Description+" "+issue.Remediation.Steps[1].Text, errorsStruct.GetFields()["/agent_ipc/port"].GetListValue().GetValues()[0].GetStringValue())
}

func TestBuildIssue_MissingConfigPath(t *testing.T) {
	issue, err := InvalidConfigIssue{}.BuildIssue(nil)
	require.NoError(t, err)
	assert.Equal(t, "Invalid Agent configuration", issue.Title)
	assert.NotContains(t, issue.Description, "unknown path")
	assert.Equal(t, "Check this setting in your Agent configuration file or environment variables.", issue.Remediation.Steps[0].Text)
	assert.Equal(t, "(unknown path)", issue.Extra.GetFields()[contextKeyConfigPath].GetStringValue())
}

func reverted(violation *model.Violation) *model.Violation {
	violation.MarkAsReverted()
	return violation
}

func TestBuildIssue_ValidatorMessage(t *testing.T) {
	cfg := config.NewMock(t)
	for _, tc := range []struct {
		name      string
		violation *model.Violation
		want      string
	}{
		{"reverted", reverted(&model.Violation{Path: "/hostname", SettingKey: "hostname", Message: "at '/hostname': 'banana' does not match pattern '^[^a]+$'"}),
			"invalid value for 'hostname', reverted to default. Error: `at '/hostname': 'banana' does not match pattern '^[^a]+$'`"},
		{"kept", &model.Violation{Path: "/hostname", SettingKey: "hostname", Message: "at '/hostname': 'banana' does not match pattern '^[^a]+$'"},
			"invalid value for 'hostname'. Error: `at '/hostname': 'banana' does not match pattern '^[^a]+$'`"},
		{"no setting key", &model.Violation{Path: "", Message: "got string, want object"},
			"invalid configuration: `got string, want object`"},
		{"markdown", reverted(&model.Violation{Path: "/hostname", SettingKey: "hostname", Message: "at '/hostname': '`[link](https://example.test)' does not match pattern 'x'"}),
			"invalid value for 'hostname', reverted to default. Error: ``at '/hostname': '`[link](https://example.test)' does not match pattern 'x'``"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue, err := InvalidConfigIssue{}.BuildIssue(BuildContext(cfg, "/etc/datadog-agent/datadog.yaml", tc.violation))
			require.NoError(t, err)
			assert.Equal(t, tc.want, issue.Description)
		})
	}
}

func TestBuildIssue_InvalidConfigurationTitle(t *testing.T) {
	cfg := config.NewMock(t)
	for _, tc := range []struct {
		name      string
		violation *model.Violation
		want      string
	}{
		{"reverted", reverted(&model.Violation{Path: "/hostname", SettingKey: "hostname", Message: "at '/hostname': 'banana' does not match pattern '^[^a]+$'"}),
			"Invalid configuration: `/hostname`, reverted to default"},
		{"kept", &model.Violation{Path: "/hostname", SettingKey: "hostname", Message: "at '/hostname': 'banana' does not match pattern '^[^a]+$'"},
			"Invalid configuration: `/hostname`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue, err := InvalidConfigIssue{}.BuildIssue(BuildContext(cfg, "/etc/datadog-agent/datadog.yaml", tc.violation))
			require.NoError(t, err)
			assert.Equal(t, tc.want, issue.Title)
		})
	}
}

func TestBuildIssue_RevertedTitleAndImpact(t *testing.T) {
	violation := reverted(&model.Violation{Path: "/agent_ipc/port", SettingKey: "agent_ipc.port", ActualType: "string", ExpectedTypes: []string{"integer"}})
	issue, err := InvalidConfigIssue{}.BuildIssue(BuildContext(config.NewMock(t), "/etc/datadog-agent/datadog.yaml", violation))
	require.NoError(t, err)
	assert.Equal(t, "Incorrect type for `/agent_ipc/port`, reverted to default", issue.Title)
	assert.Equal(t, "`/agent_ipc/port` expects a whole number, but received a string. It was reverted to its default value `0`.", issue.Description)
	assert.Equal(t, "The Datadog Agent ignored the invalid value and is using the default instead, so it may not behave as configured.",
		issue.Extra.GetFields()[contextKeyImpact].GetStringValue())
	assert.True(t, issue.Extra.GetFields()[contextKeyViolations].GetListValue().GetValues()[0].GetStructValue().AsMap()["reverted"].(bool))
}

func TestBuildIssue_Remediation(t *testing.T) {
	const fallback = "Check the value and format of `/setting`."
	for _, tc := range []struct{ name, violation, description, want string }{
		{"integer_default", `{"path":"/dogstatsd_port","actual_type":"object","expected_types":["integer"],"default_status":"known","default_value":8125}`,
			"`/dogstatsd_port` expects a whole number, but received a YAML mapping.",
			"Set `/dogstatsd_port` to a whole number. The default value for this setting is `8125`."},
		{"empty_default", `{"path":"/api_key","actual_type":"array","expected_types":["string"],"default_status":"known","default_value":""}`,
			"`/api_key` expects a string, but received a YAML list.",
			"Set `/api_key` to a string. The default value for this setting is `\"\"` (an empty string)."},
		{"no_default", `{"path":"/agent_ipc","actual_type":"string","expected_types":["object"],"default_status":"none"}`,
			"`/agent_ipc` expects a YAML mapping, but received a string.",
			"Set `/agent_ipc` to a YAML mapping. This setting has no default."},
		{"unknown_default", `{"path":"/additional_endpoints/example","actual_type":"null","expected_types":["array"],"default_status":"unknown"}`,
			"`/additional_endpoints/example` expects a YAML list, but received null.",
			"Set `/additional_endpoints/example` to a YAML list."},
		{"union", `{"path":"/setting","actual_type":"boolean","expected_types":["integer","number","string"],"default_status":"unknown"}`,
			"`/setting` expects a whole number, a number, or a string, but received true or false.",
			"Set `/setting` to a whole number, a number, or a string."},
		{"markdown", "{\"path\":\"/key`[link](https://example.test)\\n\",\"actual_type\":\"number\",\"expected_types\":[\"string\"],\"default_status\":\"known\",\"default_value\":\"`example`\"}",
			"``/key`[link](https://example.test) `` expects a string, but received a number.",
			"Set ``/key`[link](https://example.test) `` to a string. The default value for this setting is ``\"`example`\"``."},
		{"reverted_known_default", `{"path":"/dogstatsd_port","actual_type":"object","expected_types":["integer"],"default_status":"known","default_value":8125,"reverted":true}`,
			"`/dogstatsd_port` expects a whole number, but received a YAML mapping. It was reverted to its default value `8125`.",
			"Set `/dogstatsd_port` to a whole number."},
		{"reverted_empty_default", `{"path":"/api_key","actual_type":"array","expected_types":["string"],"default_status":"known","default_value":"","reverted":true}`,
			"`/api_key` expects a string, but received a YAML list. It was reverted to its default value `\"\"` (an empty string).",
			"Set `/api_key` to a string."},
		{"reverted_no_default", `{"path":"/agent_ipc","actual_type":"string","expected_types":["object"],"default_status":"none","reverted":true}`,
			"`/agent_ipc` expects a YAML mapping, but received a string. It was reverted to its default: this setting has no default value.",
			"Set `/agent_ipc` to a YAML mapping."},
		{"reverted_unknown_default", `{"path":"/setting","actual_type":"boolean","expected_types":["string"],"default_status":"unknown","reverted":true}`,
			"`/setting` expects a string, but received true or false. It was reverted to its default value.",
			"Set `/setting` to a string."},
		{"empty_facts", "", "", fallback},
		{"malformed_facts", "not JSON", "", fallback},
		{"incomplete_facts", "{}", "", fallback},
		{"unsupported_type", `{"path":"/setting","actual_type":"string","expected_types":["unsupported"],"default_status":"unknown"}`, "", fallback},
		{"missing_default", `{"path":"/setting","actual_type":"string","expected_types":["integer"],"default_status":"known"}`, "", fallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload violationPayload
			_ = json.Unmarshal([]byte(tc.violation), &payload)
			path := "/setting"
			if payload.Path != "" {
				path = payload.Path
			}
			issue, err := InvalidConfigIssue{}.BuildIssue(map[string]string{
				contextKeyConfigPath:  "/etc/datadog-agent/datadog.yaml",
				contextKeySettingPath: path,
				contextKeyViolation:   tc.violation,
			})
			require.NoError(t, err)
			description := tc.description
			if description == "" {
				description = "The value at `/setting` does not match the configuration schema."
			}
			assert.Equal(t, description, issue.Description)
			assert.Equal(t, tc.want, issue.Remediation.Steps[1].Text)
		})
	}
}

// A vanilla mock has only defaults, which round-trip through YAML cleanly and
// pass the schema. Confirms Run() is a no-op on a healthy config.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	reports, err := newTestChecker(config.NewMock(t), testHostname(t), testSelfIdent(t)).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_SeparateIssuesRemainStable(t *testing.T) {
	cfg := config.NewMockFromYAML(t, "agent_ipc: {port: wrong}\ndogstatsd_port: {wrong: type}")
	c := newTestChecker(cfg, testHostname(t), testSelfIdent(t))
	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 2)
	ids := map[string]string{}
	for _, report := range reports {
		issue, err := InvalidConfigIssue{}.BuildIssue(report.Context)
		require.NoError(t, err)
		assert.NotContains(t, issue.Title, "Found")
		errors := issue.Extra.GetFields()[contextKeyErrors].GetStructValue().GetFields()
		require.Len(t, errors, 1)
		for path, messages := range errors {
			ids[path] = report.IssueID
			message := messages.GetListValue().GetValues()[0].GetStringValue()
			assert.Contains(t, message, "expects a whole number")
			assert.Contains(t, message, "Set ")
			assert.Contains(t, message, "The default value for this setting is")
		}
	}
	assert.NotEqual(t, ids["/agent_ipc/port"], ids["/dogstatsd_port"])
	cfg.Set("agent_ipc.port", 5001, model.SourceFile)
	cfg.Set("dogstatsd_port", []string{"still wrong"}, model.SourceFile)
	// validate the updated configuration, like the Agent does when it loads it
	_, err = c.schema.ValidateConfig(cfg)
	require.NoError(t, err)
	reports, err = c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, ids["/dogstatsd_port"], reports[0].IssueID, "changing the rejected value must not create another issue")
	cfg.Set("dogstatsd_port", 8125, model.SourceFile)
	// validate the updated configuration, like the Agent does when it loads it
	_, err = c.schema.ValidateConfig(cfg)
	require.NoError(t, err)
	reports, err = c.Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
	cfg.Set("agent_ipc.port", "wrong again", model.SourceFile)
	// validate the updated configuration, like the Agent does when it loads it
	_, err = c.schema.ValidateConfig(cfg)
	require.NoError(t, err)
	reports, err = c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, ids["/agent_ipc/port"], reports[0].IssueID)
}

// Duration strings must remain accepted for both dotted and nested config keys.
func TestCheck_DurationStringIsNotAViolation(t *testing.T) {
	for _, yaml := range []string{
		"remote_configuration.refresh_interval: 5s\n",   // flat dotted key
		"remote_configuration:\n  refresh_interval: 5s", // nested key
	} {
		cfg := config.NewMockFromYAML(t, yaml)
		reports, err := newTestChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
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

	reports, err := newTestChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
	if err != nil {
		t.Skipf("schema validator unavailable (schema not embedded in test binary): %v", err)
	}
	require.Len(t, reports, 1)
	assert.Equal(t, IssueName, reports[0].IssueName)
	assert.True(t, strings.HasPrefix(reports[0].IssueID, IssueID+":"), "IssueID %q must be scoped with a host+path suffix", reports[0].IssueID)
	assert.Equal(t, "/agent_ipc/port", reports[0].Context[contextKeySettingPath])

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
		{"agent_ipc:\n  port: ENC[ipc_port]\n", ""},
		{"logs_enabled: ENC[enabled]\n", ""},
		{"agent_ipc: ENC[ipc]\n", ""},
		{"additional_endpoints: {'https://example.test': 'ENC[endpoints]'}\n", ""},
		{"api_key: ['ENC[key]']\n", "got array, want string"},
		{"logs_enabled: {value: 'ENC[enabled]'}\n", "got object, want boolean"},
	} {
		t.Run(testCase.yaml, func(t *testing.T) {
			cfg := config.NewMockFromYAML(t, testCase.yaml)

			reports, err := newTestChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
			require.NoError(t, err)
			if testCase.want == "" {
				assert.Empty(t, reports)
				return
			}
			require.Len(t, reports, 1)
			var violation violationPayload
			require.NoError(t, json.Unmarshal([]byte(reports[0].Context[contextKeyViolation]), &violation))
			assert.Equal(t, testCase.want, "got "+violation.ActualType+", want "+strings.Join(violation.ExpectedTypes, " or "))
			issue, err := InvalidConfigIssue{}.BuildIssue(reports[0].Context)
			require.NoError(t, err)
			encoded, err := json.Marshal(issue)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "RAW_URL_PASSWORD_7c81")
			assert.NotContains(t, string(encoded), "ENC[")
		})
	}
}

func TestCheck_UnresolvedSecretWithConfiguredBackend(t *testing.T) {
	requireSchema(t)
	for _, backend := range []string{
		"secret_backend_command: /test/backend",
		"secret_backend_type: file",
		"multi_secret_backends: {test: {type: file}}",
	} {
		t.Run(backend, func(t *testing.T) {
			cfg := config.NewMockFromYAML(t, backend+"\nagent_ipc: {port: 'ENC[value]'}")
			reports, err := newTestChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
			require.NoError(t, err)
			assert.Empty(t, reports)
		})
	}
}

func TestCheck_ResolvedSecrets(t *testing.T) {
	requireSchema(t)
	for _, tc := range []struct{ name, value, want string }{
		{"valid_integer", "5001", ""},
		{"invalid_integer", "SECRET_INVALID_INTEGER", "got string, want integer"},
		// the configuration is validated before secrets are resolved, so ENC[] values are always placeholders
		{"enc_placeholder", "ENC[SECRET_LITERAL]", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewMockFromYAML(t, "agent_ipc:\n  port: ENC[value]\n")
			cfg.Set("agent_ipc.port", tc.value, model.SourceSecret)

			reports, err := newTestChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
			require.NoError(t, err)
			if tc.want == "" {
				assert.Empty(t, reports)
				return
			}
			require.Len(t, reports, 1)
			issue, err := InvalidConfigIssue{}.BuildIssue(reports[0].Context)
			require.NoError(t, err)
			assert.Equal(t, "`/agent_ipc/port` expects a whole number, but received a string.", issue.Description)
			encoded, err := json.Marshal(issue)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), tc.value)
		})
	}
}

// The configuration is validated before secrets are resolved: a placeholder in a compound setting isn't reported
func TestCheck_SecretPlaceholderInMap(t *testing.T) {
	requireSchema(t)
	cfg := config.NewMockFromYAML(t, "additional_endpoints: {'https://example.test': 'ENC[PRIVATE_HANDLE]'}")
	reports, err := newTestChecker(cfg, testHostname(t), testSelfIdent(t)).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
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
	c1 := newTestChecker(cfg1, hn, si)
	c2 := newTestChecker(cfg2, hn, si)
	c1.cfg = fakeConfigFileUsed{Component: cfg1, path: "/etc/datadog-agent/datadog.yaml"}
	c2.cfg = fakeConfigFileUsed{Component: cfg2, path: "/etc/datadog-agent/datadog-cluster.yaml"}

	assert.NotEqual(t, c1.instanceIssueID(""), c2.instanceIssueID(""))
}

// Two checkers with the same config path but different hostnames must not
// collide — this is the org-wide aggregation scenario where a downstream
// consumer keys recommendations on (org, IssueID) alone.
func TestInstanceIssueID_DiffersByHostname(t *testing.T) {
	cfg := config.NewMock(t)
	hn1, _ := hostnamemock.NewMock("host-a")
	hn2, _ := hostnamemock.NewMock("host-b")

	si := testSelfIdent(t)
	c1 := newTestChecker(cfg, hn1, si)
	c2 := newTestChecker(cfg, hn2, si)

	assert.NotEqual(t, c1.instanceIssueID(""), c2.instanceIssueID(""))
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

// newTestChecker returns a checker validating cfg with a new schema component, so results cached by previous tests are
// ignored.
func newTestChecker(cfg config.Component, hostname hostnameinterface.Component, selfIdent *selfident.SelfIdent) *checker {
	schemaComp := schemaimpl.NewComponent().Comp
	if cfg != nil {
		// validate the configuration like the Agent does when it loads it. Errors (ie: the schema isn't embedded in
		// the test binary) leave no violation to report.
		_, _ = schemaComp.ValidateConfig(cfg)
	}
	return newChecker(cfg, schemaComp, hostname, selfIdent)
}
