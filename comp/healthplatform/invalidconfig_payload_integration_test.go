// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"text/template"
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
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigfx "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/fx"
	sysprobeconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/impl"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	forwarderimpl "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/impl"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidsysprobeconfig"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// invalidConfigScenarioDir holds one scenario per name, each one run as a subtest:
//   - '<name>.yaml' is appended to the base datadog.yaml. Its first line is '# description: <scenario description>'.
//   - '<name>.env' (optional) holds the DD_* environment variables of the Agent, one KEY='value' per line
//   - '<name>.system-probe.yaml' (optional) is the system-probe.yaml content
//   - '<name>.expected.json' (optional) is the expected health report. Without it, the scenario expects no
//     configuration issue.
//
// The configuration files are written to a temporary directory and loaded by the real config, sysprobeconfig and
// schema components, so this test covers the entire pipeline: schema → configuration loading (file, environment
// variables, schema enforcement, secrets) → startup checks → store → forwarder → intake payload.
const invalidConfigScenarioDir = "testdata/invalidconfig"

const scenarioDescriptionPrefix = "# description: "

// baseConfigTemplate is the datadog.yaml every scenario configuration is appended to, as a text/template
//
//go:embed testdata/invalidconfig/base.yaml
var baseConfigTemplate string

// scenarioFiles holds the scenario files. They are embedded rather than read from the runfiles, which Windows exposes
// as directory junctions that can't be opened as files.
//
//go:embed testdata/invalidconfig
var scenarioFiles embed.FS

// reportsWait is the bubble time the reports are given to be sent once the startup checks finished: several egress
// intervals of base.yaml
const reportsWait = time.Second

// invalidConfigSecrets are the values the secrets resolver returns for the ENC[] handles of the scenarios
const invalidConfigSecrets = "secrets.json"

// expectedConfigDir is the configuration directory referenced by the expected reports. It replaces the temporary
// directory the scenario configuration is written to.
const expectedConfigDir = "/etc/datadog-agent"

// noIssueCanary is an invalid setting set through an environment variable, so the test can wait for a report proving the
// startup check of its configuration file ran: empty reports are not sent, and the checks run in separate goroutines.
// A scenario gets the canary of every check its expected report holds no issue of: the expected issues prove the others
// ran.
type noIssueCanary struct {
	envVar    string
	path      string
	schema    model.SchemaID
	issueType string
}

// noIssueCanaries holds one canary per startup check: the checks run in separate goroutines, so each one is waited for
var noIssueCanaries = []noIssueCanary{
	{envVar: "DD_AGENT_IPC_PORT", path: "/agent_ipc/port", schema: model.CoreSchema, issueType: invalidconfig.IssueType},
	{envVar: "DD_SYSTEM_PROBE_HEALTH_PORT", path: "/system_probe_config/health_port", schema: model.SystemProbeSchema, issueType: invalidsysprobeconfig.IssueType},
}

// isCanary reports whether the issue is the one reported for the canary
func (c noIssueCanary) isCanary(issue *healthplatformpayload.Issue) bool {
	return issue.GetIssueType() == c.issueType && strings.Contains(issue.GetTitle(), c.path)
}

// scenarioCanaries returns the canaries of the checks the expected report holds no issue of: all of them without an
// expected report
func scenarioCanaries(t *testing.T, expectedReport string) []noIssueCanary {
	t.Helper()
	if expectedReport == "" {
		return noIssueCanaries
	}
	var expected struct {
		Issues map[string]struct {
			IssueType string `json:"issue_type"`
		} `json:"issues"`
	}
	require.NoError(t, json.Unmarshal([]byte(expectedReport), &expected), "invalid expected report")
	var canaries []noIssueCanary
	for _, canary := range noIssueCanaries {
		covered := false
		for _, issue := range expected.Issues {
			covered = covered || issue.IssueType == canary.issueType
		}
		if !covered {
			canaries = append(canaries, canary)
		}
	}
	return canaries
}

// invalidConfigLeakMarkers must never appear in a payload: the configured values, URL credentials and secret handles
var invalidConfigLeakMarkers = []string{"RAW_", "ENC["}

// resolvedSecretValues matches the marker-bearing parts of the secret values of secrets.json, e.g. a URL password
var resolvedSecretValues = regexp.MustCompile(`RAW_SECRET_[A-Za-z0-9_]+`)

// resolvedSecretMarker is held by the secret values of secrets.json the payloads must not leak: the scenarios using
// secrets check it reached the loaded configuration, so the leak check has something to catch
const resolvedSecretMarker = "RAW_SECRET_"

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
	initProcessGlobals()

	scenarios, err := fs.Glob(scenarioFiles, invalidConfigScenarioDir+"/*.yaml")
	require.NoError(t, err)
	found := 0
	for _, scenarioPath := range scenarios {
		name := strings.TrimSuffix(path.Base(scenarioPath), ".yaml")
		if name == "base" || strings.HasSuffix(name, ".system-probe") {
			continue
		}
		found++
		t.Run(name, func(t *testing.T) {
			t.Log(scenarioDescription(t, name))
			synctest.Test(t, func(t *testing.T) {
				runInvalidConfigScenario(t, name)
			})
		})
	}
	require.NotZero(t, found, "no scenario found in %s", invalidConfigScenarioDir)
}

// initProcessGlobals creates, outside any testing/synctest bubble, the process-wide state the Agent components create
// lazily: created in a scenario bubble, it would be bound to it and break the following scenarios.
func initProcessGlobals() {
	mainCtx, _ := common.GetMainCtxCancel()
	_ = mainCtx.Done() // the Done channel is created lazily too
}

// scenarioDescription returns the description of a scenario, given by the '# description: ' first line of its YAML
func scenarioDescription(t *testing.T, name string) string {
	t.Helper()
	firstLine, _, _ := strings.Cut(readScenarioFile(t, name+".yaml", true), "\n")
	description, found := strings.CutPrefix(strings.TrimSpace(firstLine), scenarioDescriptionPrefix)
	require.True(t, found && description != "", "%s/%s.yaml must start with a %q line", invalidConfigScenarioDir, name, scenarioDescriptionPrefix)
	return description
}

// runInvalidConfigScenario loads the scenario configuration like the Agent does, then asserts that the health report
// received by the intake matches the scenario expected report and that no payload leaks a configured value. It returns
// the loaded system-probe configuration, whose effective settings aren't all visible in the reports.
//
// It must run in a testing/synctest bubble: the fakeintake and the Agent forwarder talk over an in-memory connection, so
// the test knows when the startup checks finished and the reports were sent rather than polling for them.
func runInvalidConfigScenario(t *testing.T, name string) sysprobeconfig.Component {
	expected := readScenarioFile(t, name+".expected.json", false)

	listener := newPipeListener()
	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(
		fakeintakeserver.WithListener(listener, "http://"+pipeListenerAddress),
		fakeintakeserver.WithReadyChannel(ready),
	)
	fi.Start()
	require.True(t, <-ready, "fakeintake server did not become ready")
	t.Cleanup(func() { _ = fi.Stop() })
	clientTransport := listener.transport()
	t.Cleanup(clientTransport.CloseIdleConnections)
	fiClient := fakeintakeclient.NewClient(fi.URL(), fakeintakeclient.WithHTTPTransport(clientTransport))
	// the Agent forwarder reaches the fakeintake, and only it, over the in-memory connection
	agentTransport := listener.transport()
	t.Cleanup(agentTransport.CloseIdleConnections)
	forwarderimpl.SetTransport(t, agentTransport)

	confDir := t.TempDir()
	datadogYAML := filepath.Join(confDir, "datadog.yaml")
	sysprobeYAML := filepath.Join(confDir, "system-probe.yaml")
	// test-owned fleet policies directory, holding no policy: see the config components below
	fleetPoliciesDir := filepath.Join(confDir, "fleet")
	baseTemplate, err := template.New("base.yaml").Parse(baseConfigTemplate)
	require.NoError(t, err)
	var baseConfig strings.Builder
	require.NoError(t, baseTemplate.Execute(&baseConfig, struct{ RunPath, IntakeURL string }{
		RunPath:   filepath.Join(confDir, "run"),
		IntakeURL: fi.URL(),
	}))
	scenarioConfig := readScenarioFile(t, name+".yaml", true)
	require.NoError(t, os.WriteFile(datadogYAML, []byte(baseConfig.String()+scenarioConfig), 0o600))
	require.NoError(t, os.WriteFile(sysprobeYAML, []byte(readScenarioFile(t, name+".system-probe.yaml", false)), 0o600))

	setScenarioEnv(t, readScenarioFile(t, name+".env", false))
	canaries := scenarioCanaries(t, expected)
	for _, canary := range canaries {
		t.Setenv(canary.envVar, "not-a-number")
	}

	configmock.InitAgentConfigs(t)
	// The parameters of NewAgentParams, which would initialize the configurations again (setup.InitConfigObjects),
	// replacing the ones InitAgentConfigs created and registering their override functions again
	agentParams := config.NewParams(config.DefaultConfPath, config.WithFleetPoliciesDirPath(fleetPoliciesDir))
	agentParams.ConfFilePath = datadogYAML

	secretResolver := secretsmock.New(t)
	secretValues := map[string]string{}
	if strings.Contains(scenarioConfig, "ENC[") {
		require.NoError(t, json.Unmarshal([]byte(readScenarioFile(t, invalidConfigSecrets, true)), &secretValues))
		secretResolver.SetSecrets(secretValues)
	}

	deps := fxutil.Test[struct {
		fx.In
		Config         config.Component
		SysprobeConfig sysprobeconfig.Component
		Store          storedef.Component
		Schema         schema.Component
	}](t,
		Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		// the real config components load the files and validate them against the real schemas. They get a test-owned
		// fleet policies directory, which holds no policy: without it, a Fleet Automation policy configured on the host
		// (the registry on Windows) would be merged into the configuration
		config.Module(),
		fx.Supply(agentParams),
		sysprobeconfigfx.Module(),
		fx.Supply(sysprobeconfigimpl.NewParams(
			sysprobeconfigimpl.WithSysProbeConfFilePath(sysprobeYAML),
			sysprobeconfigimpl.WithFleetPoliciesDirPath(fleetPoliciesDir),
		)),
		schemafx.Module(),
		fx.Provide(func() secrets.Component { return secretResolver }),
		fx.Provide(func(t testing.TB) delegatedauth.Component { return delegatedauthmock.New(t) }),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	require.NotNil(t, deps.Store)

	// Validation runs before secrets are resolved, so the expected report is the same whether or not they are: check
	// the secrets were resolved, otherwise the leak check would pass without the secret values in the configuration
	if strings.Contains(scenarioConfig, "ENC[") {
		settings, err := json.Marshal(deps.Config.AllSettings())
		require.NoError(t, err)
		assert.NotContains(t, string(settings), "ENC[", "a secret handle was not resolved")
		assert.Contains(t, string(settings), resolvedSecretMarker, "no secret value holding %q reached the configuration", resolvedSecretMarker)
		// every marker-bearing secret value reached the configuration, so the leak check covers each of them
		for handle, value := range secretValues {
			for _, resolved := range resolvedSecretValues.FindAllString(value, -1) {
				assert.Contains(t, string(settings), resolved, "the value of the %q secret didn't reach the configuration", handle)
			}
		}
	}

	if expected == "" {
		// The startup checks report exactly the violations recorded by the schema component: only the canaries
		for _, canary := range noIssueCanaries {
			violations := deps.Schema.GetViolations(canary.schema)
			require.Len(t, violations, 1, "the %s schema must only hold the canary violation", canary.schema)
			assert.Equal(t, canary.path, violations[0].Path)
		}

		// Empty reports are not sent: every canary proves its startup check ran and its report was sent, then check they
		// are the only configuration issues
		waitForReports()
		received, err := fiClient.GetAgentHealth()
		require.NoError(t, err)
		var configIssues []*healthplatformpayload.Issue
		for _, payload := range received {
			for _, issue := range payload.Issues {
				if isConfigIssue(issue.GetId(), issue.GetIssueType()) {
					configIssues = append(configIssues, issue)
				}
			}
		}
		for _, canary := range noIssueCanaries {
			assert.True(t, slices.ContainsFunc(configIssues, canary.isCanary), "the %s canary issue was not received", canary.path)
		}
		for _, issue := range configIssues {
			isCanary := slices.ContainsFunc(noIssueCanaries, func(canary noIssueCanary) bool { return canary.isCanary(issue) })
			assert.True(t, isCanary, "unexpected configuration issue %q: %s", issue.GetId(), issue.GetTitle())
		}
		assertNoLeakedValue(t, fiClient)
		return deps.SysprobeConfig
	}

	// the startup checks finished before the reports were sent: the latest one holds all their issues. It's compared as
	// received, rather than decoded and encoded again, so the comparison sees exactly the JSON the Agent sent.
	waitForReports()
	received, err := fiClient.GetRawPayloads("/api/v2/agenthealth")
	require.NoError(t, err)
	require.NotEmpty(t, received, "no health report received")
	latest := received[0]
	for _, payload := range received {
		// payloads are listed in arrival order: on a tie, the later one is the most recent
		if !payload.Timestamp.Before(latest.Timestamp) {
			latest = payload
		}
	}
	// without a Content-Encoding header, fakeintake records the Content-Type as the encoding
	require.Equal(t, "application/json", latest.Encoding, "health reports are expected uncompressed, so their raw body can be compared")
	var latestReport healthplatformpayload.HealthReport
	require.NoError(t, json.Unmarshal(latest.Data, &latestReport))
	for _, canary := range canaries {
		require.True(t, slices.ContainsFunc(slices.Collect(maps.Values(latestReport.Issues)), canary.isCanary), "the %s canary issue was not received", canary.path)
	}
	actual, err := normalizeHealthReport(latest.Data, expected, confDir, deps.Store.IssueDiscriminator, canaries)
	require.NoError(t, err)
	assert.JSONEq(t, expected, actual, "health report doesn't match %s/%s.expected.json", invalidConfigScenarioDir, name)

	assertNoLeakedValue(t, fiClient)
	return deps.SysprobeConfig
}

// waitForReports returns once the startup checks finished and the intake stored their reports. The checks run in
// goroutines started with the Agent: once every goroutine of the bubble is durably blocked, they're done. Advancing
// the bubble clock then lets the egress send their issues, and once the bubble is idle again the intake stored every
// report sent.
func waitForReports() {
	synctest.Wait()
	time.Sleep(reportsWait)
	synctest.Wait()
}

// assertNoLeakedValue checks that none of the health reports received by the intake holds a leak marker, both in the
// raw request bodies and in the decoded reports
func assertNoLeakedValue(t *testing.T, fiClient *fakeintakeclient.Client) {
	t.Helper()
	rawPayloads, err := fiClient.GetRawPayloads("/api/v2/agenthealth")
	require.NoError(t, err)
	require.NotEmpty(t, rawPayloads, "no health report received")
	for _, payload := range rawPayloads {
		// without a Content-Encoding header, fakeintake records the Content-Type as the encoding
		require.Equal(t, "application/json", payload.Encoding, "health reports are expected uncompressed, so their raw body can be scanned")
		for _, marker := range invalidConfigLeakMarkers {
			assert.NotContains(t, string(payload.Data), marker, "a raw health report leaks a configured value")
		}
	}

	received, err := fiClient.GetAgentHealth()
	require.NoError(t, err)
	require.NotEmpty(t, received, "no health report received")
	for _, payload := range received {
		encoded, err := json.Marshal(payload.HealthReport)
		require.NoError(t, err)
		for _, marker := range invalidConfigLeakMarkers {
			assert.NotContains(t, string(encoded), marker, "a health report leaks a configured value")
		}
	}
}

// readScenarioFile returns the content of a scenario file, or an empty string for a missing optional file
func readScenarioFile(t *testing.T, name string, required bool) string {
	t.Helper()
	content, err := fs.ReadFile(scenarioFiles, invalidConfigScenarioDir+"/"+name)
	if !required && errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	return string(content)
}

// setScenarioEnv clears the DD_* and proxy environment variables of the test process and sets the scenario ones, given as
// KEY=value lines where the value may be wrapped in single or double quotes. Escapes and inline comments are not
// supported.
func setScenarioEnv(t *testing.T, envFile string) {
	t.Helper()
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		// the real config loader also reads the standard proxy variables, which would route the fake intake through a host proxy
		isProxyVar := strings.EqualFold(key, "HTTP_PROXY") || strings.EqualFold(key, "HTTPS_PROXY") || strings.EqualFold(key, "NO_PROXY")
		// the prefix is matched case-insensitively: on Windows, an inherited dd_ variable is read as its DD_ counterpart
		isDatadogVar := len(key) >= len("DD_") && strings.EqualFold(key[:len("DD_")], "DD_")
		if isDatadogVar || isProxyVar {
			t.Setenv(key, "") // restores the variable at the end of the test
			require.NoError(t, os.Unsetenv(key))
		}
	}
	// The Docker socket check of the bundle skips the host sockets when DOCKER_HOST selects another endpoint. Probing them
	// would make the result depend on the host, and on Windows a successful named pipe dial starts a process-wide I/O
	// completion goroutine blocked in a system call, which never lets a testing/synctest bubble become idle.
	t.Setenv("DOCKER_HOST", "tcp://docker.invalid:2375")
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

// isConfigIssue reports whether an issue is a configuration one, by its type or by its ID: an issue with a configuration
// ID but another type is kept, so the comparison with the expected report shows the wrong type
func isConfigIssue(id, issueType string) bool {
	if issueKind(issueType) != nil {
		return true
	}
	return slices.ContainsFunc(configIssueKinds, func(kind configIssueKind) bool { return strings.HasPrefix(id, kind.idPrefix) })
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

// normalizeHealthReport returns the raw report, as received by the intake, restricted to its configuration issues, with the values that change
// on every run replaced by the placeholders of the expected reports, and the configuration directory replaced by
// expectedConfigDir. Issue IDs are
// a hash of the issue discriminator, the configuration file and the setting path: they are checked against the readable
// IDs of the expected report ('<prefix>:<JSON pointer>') and replaced by them.
func normalizeHealthReport(rawReport []byte, expectedReport, confDir string, discriminator func(hostID string) string, canaries []noIssueCanary) (string, error) {
	var report map[string]any
	if err := json.Unmarshal(rawReport, &report); err != nil {
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
		// only configuration issues are compared: other checks of the bundle (e.g. the Docker socket one) depend on the host
		if issueType, _ := issue["issue_type"].(string); !isConfigIssue(id, issueType) {
			continue
		}
		// the canaries set by the test are not part of the expected report
		if issueType, _ := issue["issue_type"].(string); slices.ContainsFunc(canaries, func(canary noIssueCanary) bool {
			title, _ := issue["title"].(string)
			return canary.issueType == issueType && strings.Contains(title, canary.path)
		}) {
			continue
		}
		// the readable IDs only exist in the expected reports: on the wire, an issue ID is a hash
		for _, wireID := range []any{id, issue["id"]} {
			if wireID, _ := wireID.(string); wireID != "" {
				if _, readable := expected.Issues[wireID]; readable {
					return "", fmt.Errorf("issue %q is reported with its readable ID rather than a hashed one", wireID)
				}
			}
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
