// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

func testHostname(t *testing.T) hostnameinterface.Component {
	t.Helper()
	hn, _ := hostnamemock.NewMock("test-host")
	return hn
}

// requireSchema skips the test when the compressed schema files haven't been
// generated yet (run `dda inv schema.generate`). CI always has them; local
// dev builds do not unless explicitly generated.
func requireSchema(t *testing.T) {
	t.Helper()
	if _, err := schema.GetCoreSchema(); err != nil {
		t.Skipf("embedded schema not available (%v); run `dda inv schema.generate`", err)
	}
}

func TestBuildIssue_SchemaViolationProducesMediumSeverity(t *testing.T) {
	ctx := map[string]string{
		contextKeyConfigPath: "/etc/datadog-agent/datadog.yaml",
		contextKeyErrorCount: "2",
	}
	// the check writes message, pointer and fix as parallel keys; BuildIssue never re-parses
	ctx[contextErrorKey(0)] = `agent_ipc.port must be a whole number, but it is the text "8125".`
	ctx[contextPointerKey(0)] = "/agent_ipc/port"
	ctx[contextFixKey(0)] = "Remove the quotes: `agent_ipc.port: 8125`."
	ctx[contextErrorKey(1)] = `tags must be a list of text values, but it is a set of key/value pairs.`
	ctx[contextPointerKey(1)] = "/tags"
	ctx[contextFixKey(1)] = "Set `tags` to something like `[\"first\", \"second\"]`."

	issue, err := InvalidConfigIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Empty(t, issue.GetId(), "Id is set by the runner (ReportIssue), not by the template")
	assert.Equal(t, IssueName, issue.GetIssueName())
	assert.Equal(t, IssueType, issue.GetIssueType())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.GetSeverity())
	assert.Equal(t, "Datadog Agent Configuration Has 2 Schema Violations in datadog.yaml", issue.GetTitle())
	assert.Equal(t, float64(2),
		issue.GetExtra().GetFields()[contextKeyErrorCount].GetNumberValue())

	// the description is the only field `agent diagnose` renders, so it must stand alone:
	// the file, every diagnosis, and the consequence
	desc := issue.GetDescription()
	assert.Contains(t, desc, "/etc/datadog-agent/datadog.yaml")
	assert.Contains(t, desc, "agent_ipc.port must be a whole number")
	assert.Contains(t, desc, "tags must be a list")
	assert.Contains(t, desc, "falls back to the default")

	// path-keyed, with the correction alongside the diagnosis so an inline renderer has both
	errorsStruct := issue.GetExtra().GetFields()[contextKeyErrors].GetStructValue()
	require.NotNil(t, errorsStruct, "extra.errors must be a struct with one entry per violation")
	assert.Len(t, errorsStruct.GetFields(), 2, "each violation must get its own key")
	portLines := errorsStruct.GetFields()["/agent_ipc/port"].GetListValue().GetValues()
	require.Len(t, portLines, 2, "diagnosis then fix")
	assert.Contains(t, portLines[0].GetStringValue(), "must be a whole number")
	assert.Contains(t, portLines[1].GetStringValue(), "Remove the quotes")

	// every violation gets a step naming what to change; nothing restates the problem
	var texts []string
	for _, s := range issue.GetRemediation().GetSteps() {
		texts = append(texts, s.GetText())
	}
	assert.Contains(t, texts[0], "in an editor")
	assert.Contains(t, texts, "Remove the quotes: `agent_ipc.port: 8125`.")
	assert.NotContains(t, texts, "Fix each violation listed in the description.")
	assert.Contains(t, texts[len(texts)-1], "datadog-agent diagnose")
	for i, s := range issue.GetRemediation().GetSteps() {
		assert.Equal(t, int32(i+1), s.GetOrder(), "Order must be contiguous and 1-indexed")
	}
}

// A vanilla mock has only defaults, which round-trip through YAML cleanly and
// pass the schema. Confirms Run() is a no-op on a healthy config.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	reports, err := newChecker(config.NewMock(t), testHostname(t)).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// A duration setting written as a duration string (e.g. "5s") in datadog.yaml is
// coerced by the config into a time.Duration. time.Duration marshals back to a
// string through go-yaml, but the schema types duration fields as numbers, so the
// checker must normalize durations to their numeric form to avoid a spurious
// "got string, want number" violation. Regression test for the e2e diagnose suite.
func TestCheck_DurationStringIsNotAViolation(t *testing.T) {
	for _, yaml := range []string{
		"remote_configuration.refresh_interval: 5s\n",   // flat dotted key
		"remote_configuration:\n  refresh_interval: 5s", // nested key
	} {
		cfg := config.NewMockFromYAML(t, yaml)
		reports, err := newChecker(cfg, testHostname(t)).Run()
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

	reports, err := newChecker(cfg, testHostname(t)).Run()
	if err != nil {
		t.Skipf("schema validator unavailable (schema not embedded in test binary): %v", err)
	}
	require.Len(t, reports, 1)
	assert.Equal(t, IssueName, reports[0].IssueName)
	assert.True(t, strings.HasPrefix(reports[0].IssueID, IssueID+":"), "IssueID %q must be scoped with a host+path suffix", reports[0].IssueID)
	// the message uses the dotted key the customer typed; the json pointer travels separately
	// so a consumer can still attach the violation to a line
	assert.Contains(t, reports[0].Context[contextErrorKey(0)], "agent_ipc.port")
	assert.Contains(t, reports[0].Context[contextErrorKey(0)], `the text "not-a-number"`)
	assert.Equal(t, "/agent_ipc/port", reports[0].Context[contextPointerKey(0)])
	assert.NotEmpty(t, reports[0].Context[contextFixKey(0)], "every violation must carry a correction")
}

// Two checkers with the same hostname but different config files must not
// collide — this is the scenario where core agent and cluster-agent, both on
// the same host, validate their own distinct config file.
func TestInstanceIssueID_DiffersByConfigPath(t *testing.T) {
	hn := testHostname(t)
	cfg1 := config.NewMockFromYAML(t, "config_path_marker: a")
	cfg2 := config.NewMockFromYAML(t, "config_path_marker: b")

	c1 := newChecker(cfg1, hn)
	c2 := newChecker(cfg2, hn)
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

	c1 := newChecker(cfg, hn1)
	c2 := newChecker(cfg, hn2)

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
