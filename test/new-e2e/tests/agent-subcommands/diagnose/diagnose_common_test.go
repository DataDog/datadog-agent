// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// counters contains the count of the diagnosis results
type counters struct {
	Total         int `json:"total,omitempty"`
	Success       int `json:"success,omitempty"`
	Fail          int `json:"fail,omitempty"`
	Warnings      int `json:"warnings,omitempty"`
	UnexpectedErr int `json:"unexpected_error,omitempty"`
}

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

// waitForFakeIntakeHealthy blocks until the fakeintake is reachable, failing the test if it never
// becomes healthy. Call it once before a retry loop so the loop's deadline can't fire while this
// readiness wait is still in progress.
func waitForFakeIntakeHealthy(v *baseDiagnoseSuite) {
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().FakeIntake.Client().GetServerHealth())
	}, 5*time.Minute, 20*time.Second, "timedout waiting for fakeintake to be healthy")
}

// runDiagnose runs the agent diagnose command and returns its output. It assumes the fakeintake is
// already healthy; use getDiagnoseOutput for a one-shot call that also waits for readiness.
func runDiagnose(v *baseDiagnoseSuite, commandArgs ...agentclient.AgentArgsOption) string {
	diagnose := v.Env().Agent.Client.Diagnose(commandArgs...)
	v.T().Logf("Diagnose command output: %s", diagnose)
	return diagnose
}

func getDiagnoseOutput(v *baseDiagnoseSuite, commandArgs ...agentclient.AgentArgsOption) string {
	waitForFakeIntakeHealthy(v)
	return runDiagnose(v, commandArgs...)
}

// runDiagnoseChecked runs diagnose via the error-returning path and logs the output on success.
// Use it inside EventuallyWithExponentialBackoff conditions: they run synchronously on the test
// goroutine, so a returned command error retries the attempt rather than aborting via FailNow.
func runDiagnoseChecked(v *baseDiagnoseSuite, commandArgs ...agentclient.AgentArgsOption) (string, error) {
	diagnose, err := v.Env().Agent.Client.DiagnoseWithError(commandArgs...)
	if err != nil {
		return "", err
	}
	v.T().Logf("Diagnose command output: %s", diagnose)
	return diagnose, nil
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
	// Wait for readiness outside the retry window so the retry deadline below can't fire while the
	// fakeintake health wait is still in progress.
	waitForFakeIntakeHealthy(v)
	// diagnose runs live connectivity checks, so a transient endpoint timeout can make an
	// otherwise-passing run report a failure. The baseline and the include-everything run are
	// separate point-in-time measurements, so retry to tolerate a flake that clears on retry.
	// Retry synchronously on the test goroutine so no diagnose command or log outlives the deadline,
	// and so testify recovers a parser panic without crashing the binary or skipping teardown.
	v.EventuallyWithExponentialBackoff(func() error {
		diagnose, err := runDiagnoseChecked(v)
		if err != nil {
			return err
		}
		diagnoseSummary := getDiagnoseSummary(diagnose)
		for _, suite := range v.suites {
			diagnoseInclude, err := runDiagnoseChecked(v, agentclient.WithArgs([]string{"--include", suite}))
			if err != nil {
				return err
			}
			resultInclude := getDiagnoseSummary(diagnoseInclude)
			if resultInclude.Total >= diagnoseSummary.Total {
				return fmt.Errorf("expected number of checks for suite %v to be lower than the total amount of checks (%v) but was %v", suite, diagnoseSummary.Total, resultInclude.Total)
			}
			if resultInclude.Fail != 0 || resultInclude.UnexpectedErr != 0 {
				return fmt.Errorf("suite %v reported failures: fail=%v unexpected_error=%v", suite, resultInclude.Fail, resultInclude.UnexpectedErr)
			}
		}
		// Create an args array to include all suites
		includeArgs := strings.Split("--include "+strings.Join(v.suites, " --include "), " ")
		// Diagnose with all suites included should be equal to diagnose without args
		diagnoseIncludeEverySuite, err := runDiagnoseChecked(v, agentclient.WithArgs(includeArgs))
		if err != nil {
			return err
		}
		diagnoseIncludeEverySuiteSummary := getDiagnoseSummary(diagnoseIncludeEverySuite)
		if diagnoseIncludeEverySuiteSummary != diagnoseSummary {
			return fmt.Errorf("diagnose with all suites included (%+v) does not match diagnose without args (%+v)", diagnoseIncludeEverySuiteSummary, diagnoseSummary)
		}
		return nil
	}, 2*time.Minute, 15*time.Second, "diagnose reported a transient endpoint failure that did not clear on retry")
}

func (v *baseDiagnoseSuite) AssertDiagnoseJSONInclude() {
	// Wait for readiness outside the retry window so the retry deadline below can't fire while the
	// fakeintake health wait is still in progress.
	waitForFakeIntakeHealthy(v)
	// diagnose runs live connectivity checks, so a transient endpoint timeout can make an
	// otherwise-passing run report a failure. The baseline and the include-everything run are
	// separate point-in-time measurements, so retry to tolerate a flake that clears on retry.
	// Retry synchronously on the test goroutine so no diagnose command or log outlives the deadline,
	// and so testify recovers a parser panic without crashing the binary or skipping teardown.
	v.EventuallyWithExponentialBackoff(func() error {
		diagnose, err := runDiagnoseChecked(v, agentclient.WithArgs([]string{"--json"}))
		if err != nil {
			return err
		}
		diagnoseResult := unmarshalDiagnose(diagnose)
		if diagnoseResult == nil {
			return fmt.Errorf("could not unmarshal diagnose JSON output")
		}
		diagnoseSummary := diagnoseResult.Summary
		for _, suite := range v.suites {
			diagnoseInclude, err := runDiagnoseChecked(v, agentclient.WithArgs([]string{"--json", "--include", suite}))
			if err != nil {
				return err
			}
			diagnoseIncludeResult := unmarshalDiagnose(diagnoseInclude)
			if diagnoseIncludeResult == nil {
				return fmt.Errorf("could not unmarshal diagnose JSON output for suite %v", suite)
			}

			resultInclude := diagnoseIncludeResult.Summary

			if resultInclude.Total >= diagnoseSummary.Total {
				return fmt.Errorf("expected number of checks for suite %v to be lower than the total amount of checks (%v) but was %v", suite, diagnoseSummary.Total, resultInclude.Total)
			}
			if resultInclude.Fail != 0 || resultInclude.UnexpectedErr != 0 {
				return fmt.Errorf("suite %v reported failures: fail=%v unexpected_error=%v", suite, resultInclude.Fail, resultInclude.UnexpectedErr)
			}
		}

		// Create an args array to include all suites
		includeArgs := strings.Split(" --json "+" --include "+strings.Join(v.suites, " --include "), " ")

		// Diagnose with all suites included should be equal to diagnose without args
		diagnoseIncludeEverySuite, err := runDiagnoseChecked(v, agentclient.WithArgs(includeArgs))
		if err != nil {
			return err
		}
		diagnoseIncludeEverySuiteResult := unmarshalDiagnose(diagnoseIncludeEverySuite)
		if diagnoseIncludeEverySuiteResult == nil {
			return fmt.Errorf("could not unmarshal diagnose JSON output for all suites included")
		}
		if diagnoseIncludeEverySuiteResult.Summary != diagnoseSummary {
			return fmt.Errorf("diagnose with all suites included (%+v) does not match diagnose without args (%+v)", diagnoseIncludeEverySuiteResult.Summary, diagnoseSummary)
		}
		return nil
	}, 2*time.Minute, 15*time.Second, "diagnose reported a transient endpoint failure that did not clear on retry")
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
	// Wait for readiness outside the retry window so the retry deadline below can't fire while the
	// fakeintake health wait is still in progress.
	waitForFakeIntakeHealthy(v)
	// Retry synchronously on the test goroutine so no diagnose command or log outlives the deadline.
	v.EventuallyWithExponentialBackoff(func() error {
		diagnose, err := runDiagnoseChecked(v, agentclient.WithArgs([]string{"-v", "--json"}))
		if err != nil {
			return err
		}
		diagnoseResult := unmarshalDiagnose(diagnose)
		if diagnoseResult == nil {
			return fmt.Errorf("could not unmarshal diagnose JSON output")
		}

		summary := diagnoseResult.Summary

		// Verify that verbose mode displays extra information such as 'PASS' for successful checks
		if summary.Success != summary.Total {
			return fmt.Errorf("expected to have the same number of 'PASS' as the number of checks (%v), but was %v", summary.Total, summary.Success)
		}
		if !strings.Contains(diagnose, "connectivity-datadog-core-endpoints") {
			return fmt.Errorf("diagnose output does not contain connectivity-datadog-core-endpoints")
		}
		return nil
	}, 2*time.Minute, 15*time.Second, "diagnose reported a transient endpoint failure that did not clear on retry")
}

func (v *baseDiagnoseSuite) AssertOutputNotError(diagnose string) {
	assert.NotContains(v.T(), diagnose, "FAIL")
	assert.NotContains(v.T(), diagnose, "UNEXPECTED ERROR")
}

var summaryRE = createSummaryRegex()

func createSummaryRegex() *regexp.Regexp {
	// success, fail, warning and error are optional in the diagnose output (they're printed when their value != 0)
	successRegex := `(?:, Success:(?P<success>\d+))?`
	failRegex := `(?:, Fail:(?P<fail>\d+))?`
	warningRegex := `(?:, Warning:(?P<warning>\d+))?`
	errorRegex := `(?:, Error:(?P<error>\d+))?`
	regexTemplate := `Total:(?P<total>\d+)` + successRegex + failRegex + warningRegex + errorRegex
	return regexp.MustCompile(regexTemplate)
}

// getDiagnoseSummary parses the diagnose output and returns a struct containing number of success, fail, error and warning
func getDiagnoseSummary(diagnoseOutput string) counters {
	matches := summaryRE.FindStringSubmatch(diagnoseOutput)
	return counters{
		Total:         getRegexGroupValue(summaryRE, matches, "total"),
		Success:       getRegexGroupValue(summaryRE, matches, "success"),
		Warnings:      getRegexGroupValue(summaryRE, matches, "warning"),
		Fail:          getRegexGroupValue(summaryRE, matches, "fail"),
		UnexpectedErr: getRegexGroupValue(summaryRE, matches, "error"),
	}
}

// getRegexGroupValue returns the value of a specific named group, or 0 if there is no value for this group
func getRegexGroupValue(re *regexp.Regexp, matches []string, groupName string) int {
	index := re.SubexpIndex(groupName)
	if index < 0 || index >= len(matches) {
		panic(fmt.Sprintf("An error occurred while looking for group '%v' in diagnose output", groupName))
	}
	val, err := strconv.Atoi(matches[index])
	if err != nil {
		return 0
	}

	return val
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
