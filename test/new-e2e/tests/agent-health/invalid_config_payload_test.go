// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

// invalidConfigFixtures holds one scenario per name:
//   - 'invalid_config_<name>.yaml' is appended to the base agent configuration
//   - 'invalid_config_<name>.env' (optional) is the agent's environment file
//   - 'invalid_config_<name>.system-probe.yaml' (optional) is the system-probe configuration
//   - 'invalid_config_<name>.expected.json' (optional) is the expected health report. Without it, the scenario
//     expects no configuration issue.
//
//go:embed fixtures/invalid_config_enforce.env
//go:embed fixtures/invalid_config_enforce.expected.json
//go:embed fixtures/invalid_config_enforce.yaml
//go:embed fixtures/invalid_config_env_types.env
//go:embed fixtures/invalid_config_env_types.expected.json
//go:embed fixtures/invalid_config_env_types.yaml
//go:embed fixtures/invalid_config_file_types.expected.json
//go:embed fixtures/invalid_config_file_types.yaml
//go:embed fixtures/invalid_config_harmless.env
//go:embed fixtures/invalid_config_harmless.yaml
//go:embed fixtures/invalid_config_raw_json.expected.json
//go:embed fixtures/invalid_config_raw_json.yaml
//go:embed fixtures/invalid_config_secret.py
//go:embed fixtures/invalid_config_secrets.expected.json
//go:embed fixtures/invalid_config_secrets.yaml
//go:embed fixtures/invalid_config_system_probe.expected.json
//go:embed fixtures/invalid_config_system_probe.system-probe.yaml
//go:embed fixtures/invalid_config_system_probe.yaml
//go:embed fixtures/invalid_config_valid.env
//go:embed fixtures/invalid_config_valid.yaml
//go:embed fixtures/invalid_config_value_scrubbing.env
//go:embed fixtures/invalid_config_value_scrubbing.expected.json
//go:embed fixtures/invalid_config_value_scrubbing.yaml
var invalidConfigFixtures embed.FS

const (
	invalidConfigEnvFilePath   = "/etc/datadog-agent/environment"
	invalidConfigSecretBackend = "/tmp/bin/secret.py"
	// healthPlatformPersistence is where the health platform persists its issues across restarts
	healthPlatformPersistence = "/opt/datadog-agent/run/health-platform"
	// invalidConfigAbsenceWindow is how long the absence of an issue is checked. The forwarder sends every 5s.
	invalidConfigAbsenceWindow = time.Minute
	placeholderTimestamp       = "<timestamp>"
	placeholderHostname        = "<hostname>"
	placeholderAgentVersion    = "<agent_version>"
)

// configIssueKind describes an issue type reported for the schema violations of a configuration file
type configIssueKind struct {
	issueType  string
	idPrefix   string
	configFile string
}

var configIssueKinds = []configIssueKind{
	{issueType: "invalid_config", idPrefix: "invalid-config:", configFile: "/etc/datadog-agent/datadog.yaml"},
	{issueType: "invalid_system-probe_config", idPrefix: "invalid-system-probe-config:", configFile: "/etc/datadog-agent/system-probe.yaml"},
}

// invalidConfigLeakMarkers must never appear in a payload: the configured values, URL credentials and secret handles
var invalidConfigLeakMarkers = []string{"RAW_", "ENC["}

// invalidConfigSuite asserts the entire health report sent for configuration schema violations
type invalidConfigSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestInvalidConfigPayloadSuite runs the invalid-config payload tests
func TestInvalidConfigPayloadSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &invalidConfigSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(resilienceBaseConfig),
				),
			),
		)),
	)
}

// TestValidConfig checks that valid scalar, list and map values, from the file and environment variables, raise no issue
func (s *invalidConfigSuite) TestValidConfig() {
	s.runScenario("valid")
}

// TestHarmlessInput checks that unknown settings, settings without a value, empty collections and deprecated
// settings raise no issue
func (s *invalidConfigSuite) TestHarmlessInput() {
	s.runScenario("harmless")
}

// TestFileTypeViolations checks the issues raised for values of the wrong type in the configuration file: maps and
// lists given to scalar settings, maps given to list settings, lists and scalars given to map settings, wrong values
// inside map settings and a wrong value under a deprecated name. A violation under a key holding URL credentials must
// be reported scrubbed.
func (s *invalidConfigSuite) TestFileTypeViolations() {
	s.runScenario("file_types")
}

// TestEnvVarTypeViolations checks the issues raised for values of the wrong type set through environment variables,
// and that a valid environment variable hides an invalid value from the file
func (s *invalidConfigSuite) TestEnvVarTypeViolations() {
	s.runScenario("env_types")
}

// TestSchemaEnforcementRevertsInvalidSettings checks that, with 'schema.enforce', the issues report that the invalid
// settings from the file and environment variables were reverted to their default
func (s *invalidConfigSuite) TestSchemaEnforcementRevertsInvalidSettings() {
	s.runScenario("enforce")
}

// TestRawJSONInFile checks JSON strings given to map and list settings in the file: valid JSON objects are accepted
// for map settings, other JSON values are reported
func (s *invalidConfigSuite) TestRawJSONInFile() {
	s.runScenario("raw_json")
}

// TestValueScrubbing checks that the URL credentials of a value that doesn't match its pattern are scrubbed from the
// validator message reported in the issue
func (s *invalidConfigSuite) TestValueScrubbing() {
	s.runScenario("value_scrubbing")
}

// TestSecrets checks that settings set from secrets are not reported even when the resolved values are JSON strings
// or invalid scalars, since the configuration is validated before secrets are resolved, and that neither secret
// handles nor resolved values leak into the payload
func (s *invalidConfigSuite) TestSecrets() {
	script, err := invalidConfigFixtures.ReadFile("fixtures/invalid_config_secret.py")
	require.NoError(s.T(), err)
	s.runScenario("secrets", secretsutils.WithUnixSetupCustomScript(invalidConfigSecretBackend, string(script), false))
}

// TestSystemProbeConfig checks that the violations of the system-probe configuration are reported by the Agent as
// invalid system-probe configuration issues
func (s *invalidConfigSuite) TestSystemProbeConfig() {
	s.runScenario("system_probe")
}

// runScenario starts the agent with the scenario configuration and a fresh health platform state, then asserts that
// the health report matches the scenario expected report and that no payload leaks a configured value
func (s *invalidConfigSuite) runScenario(name string, extraOptions ...agentparams.Option) {
	t := s.T()
	scenarioConfig := s.readFixture(name+".yaml", true)
	expected := s.readFixture(name+".expected.json", false)

	// the environment and system-probe files are always written, so the ones of a previous scenario don't apply
	options := append([]agentparams.Option{
		agentparams.WithAgentConfig(resilienceBaseConfig + scenarioConfig),
		agentparams.WithSystemProbeConfig(s.readFixture(name+".system-probe.yaml", false)),
		agentparams.WithFile(invalidConfigEnvFilePath, s.readFixture(name+".env", false), true),
	}, extraOptions...)
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(ec2.WithAgentOptions(options...))))

	// The startup checks run once per agent start, and issues from a previous scenario are persisted as resolved:
	// restart with an empty health platform state so the report only holds this scenario's issues.
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"sudo systemctl stop datadog-agent && sudo rm -rf %s && sudo systemctl start datadog-agent", healthPlatformPersistence))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent not ready after restart")
	fakeIntake := s.Env().FakeIntake.Client()
	require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

	if expected == "" {
		require.Never(t, func() bool {
			payloads, err := fakeIntake.GetAgentHealth()
			if err != nil {
				return false
			}
			for _, payload := range payloads {
				for _, issue := range payload.Issues {
					if issueKind(issue.GetIssueType()) != nil {
						t.Logf("unexpected issue %q: %s", issue.GetId(), issue.GetTitle())
						return true
					}
				}
			}
			return false
		}, invalidConfigAbsenceWindow, defaultIssuePollInterval, "the configuration must not raise a configuration issue")
		return
	}

	var payloads []*aggregator.AgentHealthPayload
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = fakeIntake.GetAgentHealth()
		require.NoError(c, err)
		require.NotEmpty(c, payloads, "no health report received yet")
		actual, err := normalizeHealthReport(latestPayload(payloads), expected)
		require.NoError(c, err)
		assert.JSONEq(c, expected, actual)
	}, defaultIssueTimeout, defaultIssuePollInterval, "health report doesn't match fixtures/invalid_config_%s.expected.json", name)

	for _, payload := range payloads {
		encoded, err := json.Marshal(payload.HealthReport)
		require.NoError(t, err)
		for _, marker := range invalidConfigLeakMarkers {
			assert.NotContains(t, string(encoded), marker, "a health report leaks a configured value")
		}
	}
}

// readFixture returns the content of a scenario fixture, or an empty string for a missing optional fixture
func (s *invalidConfigSuite) readFixture(name string, required bool) string {
	content, err := invalidConfigFixtures.ReadFile("fixtures/invalid_config_" + name)
	if !required && errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	require.NoError(s.T(), err)
	return string(content)
}

// issueKind returns the configuration issue kind of an issue type, nil for other issues
func issueKind(issueType string) *configIssueKind {
	for i := range configIssueKinds {
		if configIssueKinds[i].issueType == issueType {
			return &configIssueKinds[i]
		}
	}
	return nil
}

// latestPayload returns the most recently collected payload
func latestPayload(payloads []*aggregator.AgentHealthPayload) *aggregator.AgentHealthPayload {
	sorted := append([]*aggregator.AgentHealthPayload(nil), payloads...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].GetCollectedTime().Before(sorted[j].GetCollectedTime())
	})
	return sorted[len(sorted)-1]
}

// normalizeHealthReport returns the payload as JSON with the values that change on every run replaced by the
// placeholders of the expected reports. Issue IDs are a hash of the host, the configuration file and the setting path:
// they are checked against the readable IDs of the expected report ('<prefix>:<JSON pointer>') and replaced by them.
func normalizeHealthReport(payload *aggregator.AgentHealthPayload, expectedReport string) (string, error) {
	encoded, err := json.Marshal(payload.HealthReport)
	if err != nil {
		return "", err
	}
	var report map[string]any
	if err := json.Unmarshal(encoded, &report); err != nil {
		return "", err
	}
	var expected struct {
		Issues map[string]struct {
			IssueType string `json:"issue_type"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(expectedReport), &expected); err != nil {
		return "", fmt.Errorf("invalid expected report: %w", err)
	}

	if err := replaceTimestamp(report, "emitted_at"); err != nil {
		return "", err
	}
	host, _ := report["host"].(map[string]any)
	hostname, _ := host["hostname"].(string)
	if hostname == "" {
		return "", errors.New("the report has no hostname")
	}
	host["hostname"] = placeholderHostname
	if host["resource_id"] == hostname {
		host["resource_id"] = placeholderHostname
	}
	if version, _ := host["agent_version"].(string); version != "" {
		host["agent_version"] = placeholderAgentVersion
	}

	readableIDs := map[string]string{}
	for readableID, issue := range expected.Issues {
		kind := issueKind(issue.IssueType)
		if kind == nil {
			return "", fmt.Errorf("expected issue %q has an unknown issue type %q", readableID, issue.IssueType)
		}
		readableIDs[configIssueID(*kind, hostname, strings.TrimPrefix(readableID, kind.idPrefix))] = readableID
	}
	issues, _ := report["issues"].(map[string]any)
	normalized := make(map[string]any, len(issues))
	for id, rawIssue := range issues {
		issue, ok := rawIssue.(map[string]any)
		if !ok {
			return "", fmt.Errorf("issue %q is not an object", id)
		}
		// unexpected IDs are kept as-is, so they show in the diff
		if readableID, found := readableIDs[id]; found {
			if issue["id"] == id {
				issue["id"] = readableID
			}
			id = readableID
		}
		if err := replaceTimestamp(issue, "detected_at"); err != nil {
			return "", err
		}
		if persisted, ok := issue["persisted_issue"].(map[string]any); ok {
			for _, field := range []string{"first_seen", "last_seen", "resolved_at"} {
				if err := replaceTimestamp(persisted, field); err != nil {
					return "", err
				}
			}
		}
		normalized[id] = issue
	}
	report["issues"] = normalized

	out, err := json.Marshal(report)
	return string(out), err
}

// replaceTimestamp replaces the RFC3339 timestamp at field by its placeholder, if the field is set
func replaceTimestamp(object map[string]any, field string) error {
	value, found := object[field]
	if !found {
		return nil
	}
	text, _ := value.(string)
	if _, err := time.Parse(time.RFC3339, text); err != nil {
		return fmt.Errorf("field %q is not an RFC3339 timestamp: %q", field, text)
	}
	object[field] = placeholderTimestamp
	return nil
}

// configIssueID returns the ID of the configuration issue reported by a host agent for a setting path
func configIssueID(kind configIssueKind, hostname, settingPath string) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s\x00%s", hostname, kind.configFile, settingPath)
	return fmt.Sprintf("%s%016x", kind.idPrefix, h.Sum64())
}
