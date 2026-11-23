// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diagnoseResult contains the results of the diagnose command
type diagnoseResult struct {
	Summary counters `json:"summary"`
}

type baseDiagnoseSuite struct {
	e2e.BaseSuite[environments.Host]

	suites []string
}

var commonSuites = []string{
	"check-datadog",
	"connectivity-datadog-autodiscovery",
	"connectivity-datadog-core-endpoints",
	"connectivity-datadog-event-platform",
	"port-conflict",
}

func getDiagnoseOutput(v *baseDiagnoseSuite, commandArgs ...agentclient.AgentArgsOption) string {
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().FakeIntake.Client().GetServerHealth())
	}, 5*time.Minute, 20*time.Second, "timedout waiting for fakeintake to be healthy")

	diagnose := v.Env().Agent.Client.Diagnose(commandArgs...)
	v.T().Logf("Diagnose command output: %s", diagnose)
	return diagnose
}

func (v *baseDiagnoseSuite) TestDiagnoseDefaultConfig() {
	diagnose := getDiagnoseOutput(v)
	v.AssertOutputNotError(diagnose)

	diagnose = getDiagnoseOutput(v, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Zero(v.T(), diagnoseJSON.Summary.Fail)
	assert.Zero(v.T(), diagnoseJSON.Summary.UnexpectedErr)
}

func (v *baseDiagnoseSuite) TestDiagnoseLocal() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--local"}))
	v.AssertOutputNotError(diagnose)

	diagnose = getDiagnoseOutput(v, agentclient.WithArgs([]string{"--json", "--local"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Zero(v.T(), diagnoseJSON.Summary.Fail)
	assert.Zero(v.T(), diagnoseJSON.Summary.UnexpectedErr)
}

func (v *baseDiagnoseSuite) TestDiagnoseList() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--list"}))
	for _, suite := range v.suites {
		assert.Contains(v.T(), diagnose, suite)
	}
}

func (v *baseDiagnoseSuite) AssertDiagnoseInclude() {
	for _, format := range []string{"text", "json"} {
		v.Run(format, func() {
			for _, suite := range v.suites {
				v.Run(suite, func() {
					v.assertDiagnoseSelection(format, []string{suite}, true)
				})
			}
			v.Run("all", func() {
				baseline := v.assertDiagnoseSelection(format, v.suites, false)
				included := v.assertDiagnoseSelection(format, v.suites, true)
				assert.Equal(v.T(), baseline.identities(), included.identities(), "including every suite must select the same diagnoses as the default")
			})
		})
	}
}

func (v *baseDiagnoseSuite) assertDiagnoseSelection(format string, suites []string, include bool) includeOutput {
	args := []string{"--verbose"}
	if format == "json" {
		args = append(args, "--json")
	}
	if include {
		for _, suite := range suites {
			args = append(args, "--include", "^"+regexp.QuoteMeta(suite)+"$")
		}
	}
	output := getDiagnoseOutput(v, agentclient.WithArgs(args))
	result, err := parseIncludeOutput(output, format)
	require.NoError(v.T(), err)
	// The default invocation is only a membership reference. Health is checked
	// on each selected response, independently of the reference's network outcomes.
	if include {
		require.Zero(v.T(), result.Summary.Fail, "diagnose reported failed checks for %v", suites)
		require.Zero(v.T(), result.Summary.UnexpectedErr, "diagnose reported unexpected errors for %v", suites)
	}
	seen := make(map[string]bool)
	for _, run := range result.Runs {
		require.Contains(v.T(), suites, run.Name)
		require.NotContains(v.T(), seen, run.Name, "duplicate diagnose suite")
		require.NotEmpty(v.T(), run.Diagnoses, "suite %q emitted no diagnoses", run.Name)
		seen[run.Name] = true
	}
	for _, suite := range suites {
		// No integration checks are configured for this suite on the default host.
		if suite != "check-datadog" {
			require.True(v.T(), seen[suite], "selected suite %q has no results", suite)
		}
	}
	return result
}

func (v *baseDiagnoseSuite) AssertDiagnoseExclude() {
	for _, suite := range v.suites {
		diagnoseExclude := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--exclude", suite}))
		resultExclude := getDiagnoseSummary(diagnoseExclude)
		assert.Equal(v.T(), resultExclude.Fail, 0)
		assert.Equal(v.T(), resultExclude.UnexpectedErr, 0)
	}

	// Create an args array to exclude all suites
	excludeArgs := strings.Split(" --exclude "+strings.Join(v.suites, " --exclude "), " ")

	// Diagnose with all suites excluded should do nothing
	diagnoseExcludeEverySuite := getDiagnoseOutput(v, agentclient.WithArgs(excludeArgs))
	summary := getDiagnoseSummary(diagnoseExcludeEverySuite)
	assert.Equal(v.T(), summary.Total, 0)
}

func (v *baseDiagnoseSuite) AssertDiagnoseJSONExclude() {
	for _, suite := range v.suites {
		diagnoseExclude := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--json", "--exclude", suite}))
		diagnoseExcludeResult := unmarshalDiagnose(diagnoseExclude)
		require.NotNil(v.T(), diagnoseExcludeResult)

		resultExclude := diagnoseExcludeResult.Summary

		assert.Equal(v.T(), resultExclude.Fail, 0)
		assert.Equal(v.T(), resultExclude.UnexpectedErr, 0)
	}

	// Create an args array to exclude all suites
	excludeArgs := strings.Split(" --json "+" --exclude "+strings.Join(v.suites, " --exclude "), " ")

	// Diagnose with all suites excluded should do nothing
	diagnoseExcludeEverySuite := getDiagnoseOutput(v, agentclient.WithArgs(excludeArgs))
	diagnoseExcludeEverySuiteResult := unmarshalDiagnose(diagnoseExcludeEverySuite)
	require.NotNil(v.T(), diagnoseExcludeEverySuiteResult)
	assert.Equal(v.T(), diagnoseExcludeEverySuiteResult.Summary.Total, 0)

}

func (v *baseDiagnoseSuite) TestDiagnoseVerbose() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"-v"}))
	summary := getDiagnoseSummary(diagnose)
	re := regexp.MustCompile("PASS")
	matches := re.FindAllString(diagnose, -1)
	// Verify that verbose mode display extra information such 'PASS' for successful checks
	assert.Equal(v.T(), len(matches), summary.Success, "Expected to have the same number of 'PASS' as successful checks (%v), but was %v", summary.Success, len(matches))
	assert.Contains(v.T(), diagnose, "connectivity-datadog-core-endpoints")
}

func (v *baseDiagnoseSuite) TestDiagnoseJSON() {
	// Retry to tolerate transient timeouts
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"-v", "--json"}))
		diagnoseResult := unmarshalDiagnose(diagnose)
		if !assert.NotNil(c, diagnoseResult) {
			return
		}

		summary := diagnoseResult.Summary

		// Verify that verbose mode displays extra information such as 'PASS' for successful checks
		assert.Equal(c, summary.Success, summary.Total, "Expected to have the same number of 'PASS' as the number of checks (%v), but was %v", summary.Total, summary.Success)
		assert.Contains(c, diagnose, "connectivity-datadog-core-endpoints")
	}, 2*time.Minute, 15*time.Second, "diagnose reported a transient endpoint failure that did not clear on retry")
}

func (v *baseDiagnoseSuite) AssertOutputNotError(diagnose string) {
	assert.NotContains(v.T(), diagnose, "FAIL")
	assert.NotContains(v.T(), diagnose, "UNEXPECTED ERROR")
}

// unmarshalDiagnose converts a diagnose string to a diagnoseResult struct
func unmarshalDiagnose(s string) *diagnoseResult {
	result := &diagnoseResult{}
	err := json.Unmarshal([]byte(s), result)
	if err != nil {
		return nil
	}
	return result
}
