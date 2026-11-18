// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	schema "github.com/DataDog/datadog-agent/comp/core/schema/def"
	schemafx "github.com/DataDog/datadog-agent/comp/core/schema/fx"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	sysprobeconfigfx "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/fx"
	sysprobeconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/impl"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidsysprobeconfig"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// invalidConfigScenarioDir holds one scenario per name:
//   - '<name>.yaml' is appended to the base datadog.yaml
//   - '<name>.env' (optional) holds the DD_* environment variables of the Agent, one KEY='value' per line
//   - '<name>.system-probe.yaml' (optional) is the system-probe.yaml content
//   - '<name>.expected.json' (optional) is the expected health report. Without it, the scenario expects no
//     configuration issue.
//
// The configuration files are written to a temporary directory and loaded by the real config, sysprobeconfig and
// schema components, so this test covers the entire pipeline: schema → configuration loading (file, environment
// variables, schema enforcement, secrets) → startup checks → store → forwarder → intake payload.
const invalidConfigScenarioDir = "testdata/invalidconfig"

// invalidConfigSecrets are the values the secrets resolver returns for the ENC[] handles of the scenarios
const invalidConfigSecrets = "secrets.json"

// expectedConfigDir is the configuration directory referenced by the expected reports. It replaces the temporary
// directory the scenario configuration is written to.
const expectedConfigDir = "/etc/datadog-agent"

// The scenarios without an expected report also set the canary, so the test can wait for a report proving the startup
// check ran: empty reports are not sent.
const (
	noIssueCanaryEnvVar = "DD_AGENT_IPC_PORT"
	noIssueCanaryPath   = "/agent_ipc/port"
)

// invalidConfigLeakMarkers must never appear in a payload: the configured values, URL credentials and secret handles
var invalidConfigLeakMarkers = []string{"RAW_", "ENC["}

const (
	placeholderTimestamp    = "<timestamp>"
	placeholderHostname     = "<hostname>"
	placeholderAgentVersion = "<agent_version>"
)

// configIssueKind describes an issue type reported for the schema violations of a configuration file
type configIssueKind struct {
	issueType  string
	idPrefix   string
	configFile string
}

var configIssueKinds = []configIssueKind{
	{issueType: invalidconfig.IssueType, idPrefix: invalidconfig.IssueID + ":", configFile: "datadog.yaml"},
	{issueType: invalidsysprobeconfig.IssueType, idPrefix: invalidsysprobeconfig.IssueID + ":", configFile: "system-probe.yaml"},
}

func TestInvalidConfigPayload(t *testing.T) {
	requireSchema(t)

	for _, tc := range []struct {
		name        string
		description string
	}{
		{"valid", "valid scalar, list and map values, from the file and environment variables, raise no issue"},
		{"harmless", "unknown settings, settings without a value, empty collections and deprecated settings raise no issue"},
		{"file_types", "values of the wrong type in the file are reported, URL credentials in keys are scrubbed"},
		{"env_types", "values of the wrong type in environment variables are reported, valid ones hide invalid file values"},
		{"enforce", "with 'schema.enforce', invalid settings from the file and environment variables are reported as reverted"},
		{"raw_json", "JSON objects given to map settings in the file are accepted, other JSON values are reported"},
		{"value_scrubbing", "URL credentials of a value not matching its pattern are scrubbed from the message"},
		{"secrets", "settings set from secrets are validated before resolution, and neither handles nor values leak"},
		{"system_probe", "violations of system-probe.yaml are reported as invalid system-probe configuration issues"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Log(tc.description)
			runInvalidConfigScenario(t, tc.name)
		})
	}
}

// runInvalidConfigScenario loads the scenario configuration like the Agent does, then asserts that the health report
// received by the intake matches the scenario expected report and that no payload leaks a configured value
func runInvalidConfigScenario(t *testing.T, name string) {
	expected := readScenarioFile(t, name+".expected.json", false)

	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(
		fakeintakeserver.WithAddress("127.0.0.1:0"),
		fakeintakeserver.WithReadyChannel(ready),
	)
	fi.Start()
	require.True(t, <-ready, "fakeintake server did not become ready")
	t.Cleanup(func() { _ = fi.Stop() })
	fiClient := fakeintakeclient.NewClient(fi.URL())

	confDir := t.TempDir()
	datadogYAML := filepath.Join(confDir, "datadog.yaml")
	sysprobeYAML := filepath.Join(confDir, "system-probe.yaml")
	baseConfig := fmt.Sprintf(`api_key: "0000001"
run_path: %q
health_platform:
  enabled: true
  persist_on_kubernetes: true
  forwarder:
    dd_url: %q
    interval: 100ms
`, filepath.Join(confDir, "run"), fi.URL())
	scenarioConfig := readScenarioFile(t, name+".yaml", true)
	require.NoError(t, os.WriteFile(datadogYAML, []byte(baseConfig+scenarioConfig), 0o600))
	require.NoError(t, os.WriteFile(sysprobeYAML, []byte(readScenarioFile(t, name+".system-probe.yaml", false)), 0o600))

	setScenarioEnv(t, readScenarioFile(t, name+".env", false))
	if expected == "" {
		t.Setenv(noIssueCanaryEnvVar, "not-a-number")
	}

	// Reset the global configurations to their defaults: the config components load into them. The environment must
	// be set before, since the configurations read it when they are built.
	configmock.New(t)
	configmock.NewSystemProbe(t)

	secretResolver := secretsmock.New(t)
	if strings.Contains(scenarioConfig, "ENC[") {
		secretValues := map[string]string{}
		require.NoError(t, json.Unmarshal([]byte(readScenarioFile(t, invalidConfigSecrets, true)), &secretValues))
		secretResolver.SetSecrets(secretValues)
	}

	deps := fxutil.Test[struct {
		fx.In
		Store  storedef.Component
		Schema schema.Component
	}](t,
		Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		// the real config components load the files and validate them against the real schemas
		config.Module(),
		fx.Supply(config.NewAgentParams(datadogYAML)),
		sysprobeconfigfx.Module(),
		fx.Supply(sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(sysprobeYAML))),
		schemafx.Module(),
		fx.Provide(func() secrets.Component { return secretResolver }),
		fx.Provide(func(t testing.TB) delegatedauth.Component { return delegatedauthmock.New(t) }),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	require.NotNil(t, deps.Store)

	if expected == "" {
		// The startup checks report exactly the violations recorded by the schema component: only the canary
		coreViolations := deps.Schema.GetViolations(model.CoreSchema)
		require.Len(t, coreViolations, 1, "datadog.yaml must only hold the canary violation")
		assert.Equal(t, noIssueCanaryPath, coreViolations[0].Path)
		assert.Empty(t, deps.Schema.GetViolations(model.SystemProbeSchema), "system-probe.yaml must be valid")

		// Empty reports are not sent: wait for the canary to prove the startup check ran and its report was sent,
		// then check it is the only configuration issue
		var configIssues []*healthplatformpayload.Issue
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			received, err := fiClient.GetAgentHealth()
			require.NoError(c, err)
			configIssues = configIssues[:0]
			canaryFound := false
			for _, payload := range received {
				for _, issue := range payload.Issues {
					if issueKind(issue.GetIssueType()) == nil {
						continue
					}
					configIssues = append(configIssues, issue)
					if strings.Contains(issue.GetTitle(), noIssueCanaryPath) {
						canaryFound = true
					}
				}
			}
			assert.True(c, canaryFound, "the canary issue was not received yet")
		}, 10*time.Second, 100*time.Millisecond, "the canary configuration issue must be reported")
		for _, issue := range configIssues {
			assert.Contains(t, issue.GetTitle(), noIssueCanaryPath, "unexpected configuration issue %q", issue.GetId())
		}
		return
	}

	var payloads []*healthplatformpayload.HealthReport
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		received, err := fiClient.GetAgentHealth()
		require.NoError(c, err)
		require.NotEmpty(c, received, "no health report received yet")
		payloads = payloads[:0]
		latest := received[0]
		for _, payload := range received {
			payloads = append(payloads, payload.HealthReport)
			// payloads are listed in arrival order: on a tie, the later one is the most recent
			if !payload.GetCollectedTime().Before(latest.GetCollectedTime()) {
				latest = payload
			}
		}
		actual, err := normalizeHealthReport(latest.HealthReport, expected, confDir, deps.Store.IssueDiscriminator)
		require.NoError(c, err)
		assert.JSONEq(c, expected, actual)
	}, 10*time.Second, 100*time.Millisecond, "health report doesn't match %s/%s.expected.json", invalidConfigScenarioDir, name)

	for _, payload := range payloads {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		for _, marker := range invalidConfigLeakMarkers {
			assert.NotContains(t, string(encoded), marker, "a health report leaks a configured value")
		}
	}
}

// readScenarioFile returns the content of a scenario file, or an empty string for a missing optional file
func readScenarioFile(t *testing.T, name string, required bool) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(invalidConfigScenarioDir, name))
	if !required && errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	return string(content)
}

// setScenarioEnv clears the DD_* environment variables of the test process and sets the scenario ones, given as
// KEY=value lines where the value may be wrapped in single or double quotes. Escapes and inline comments are not
// supported.
func setScenarioEnv(t *testing.T, envFile string) {
	t.Helper()
	for _, kv := range os.Environ() {
		if key, _, _ := strings.Cut(kv, "="); strings.HasPrefix(key, "DD_") {
			t.Setenv(key, "") // restores the variable at the end of the test
			require.NoError(t, os.Unsetenv(key))
		}
	}
	scanner := bufio.NewScanner(strings.NewReader(envFile))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		require.True(t, found, "invalid environment line %q", line)
		if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		t.Setenv(key, value)
	}
	require.NoError(t, scanner.Err())
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

// normalizeHealthReport returns the payload as JSON with the values that change on every run replaced by the
// placeholders of the expected reports, and the configuration directory replaced by expectedConfigDir. Issue IDs are
// a hash of the issue discriminator, the configuration file and the setting path: they are checked against the readable
// IDs of the expected report ('<prefix>:<JSON pointer>') and replaced by them.
func normalizeHealthReport(payload *healthplatformpayload.HealthReport, expectedReport, confDir string, discriminator func(hostID string) string) (string, error) {
	encoded, err := json.Marshal(payload)
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
		configFile := filepath.Join(confDir, kind.configFile)
		readableIDs[configIssueID(*kind, discriminator(hostname), configFile, strings.TrimPrefix(readableID, kind.idPrefix))] = readableID
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
	if err != nil {
		return "", err
	}
	return replaceConfDir(string(out), confDir), nil
}

// replaceConfDir replaces the configuration directory in the JSON report by expectedConfigDir, converting the path
// separators of the files inside it, so the expected reports hold the same paths on every platform
func replaceConfDir(report, confDir string) string {
	for _, prefix := range []string{confDir + string(filepath.Separator), confDir} {
		escaped, _ := json.Marshal(prefix)
		replacement := expectedConfigDir
		if prefix != confDir {
			replacement += "/"
		}
		report = strings.ReplaceAll(report, strings.Trim(string(escaped), `"`), replacement)
	}
	return report
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

// configIssueID returns the ID of the configuration issue reported for a setting path, mirroring instanceIssueID in
// issues/invalidconfig and issues/invalidsysprobeconfig
func configIssueID(kind configIssueKind, discriminator, configFile, settingPath string) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s\x00%s", discriminator, configFile, settingPath)
	return fmt.Sprintf("%s%016x", kind.idPrefix, h.Sum64())
}
