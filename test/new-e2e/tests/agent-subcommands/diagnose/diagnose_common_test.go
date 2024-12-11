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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"

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

// DiagnoseResult contains the results of the diagnose command
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
	assert.NotNil(v.T(), diagnoseJSON)
	assert.Zero(v.T(), diagnoseJSON.Summary.Fail)
	assert.Zero(v.T(), diagnoseJSON.Summary.UnexpectedErr)
}

func (v *baseDiagnoseSuite) TestDiagnoseLocal() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--local"}))
	v.AssertOutputNotError(diagnose)

	diagnose = getDiagnoseOutput(v, agentclient.WithArgs([]string{"--json", "--local"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	assert.NotNil(v.T(), diagnoseJSON)
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
	diagnose := getDiagnoseOutput(v)
	diagnoseSummary := getDiagnoseSummary(diagnose)
	for _, suite := range v.suites {
		diagnoseInclude := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--include", suite}))
		resultInclude := getDiagnoseSummary(diagnoseInclude)
		assert.Less(v.T(), resultInclude.Total, diagnoseSummary.Total, "Expected number of checks for suite %v to be lower than the total amount of checks (%v) but was %v", suite, diagnoseSummary.Total, resultInclude.Total)
		assert.Zero(v.T(), resultInclude.Fail)
		assert.Zero(v.T(), resultInclude.UnexpectedErr)
	}
	// Create an args array to include all suites
	includeArgs := strings.Split("--include "+strings.Join(v.suites, " --include "), " ")
	// Diagnose with all suites included should be equal to diagnose without args
	diagnoseIncludeEverySuite := getDiagnoseOutput(v, agentclient.WithArgs(includeArgs))
	diagnoseIncludeEverySuiteSummary := getDiagnoseSummary(diagnoseIncludeEverySuite)
	assert.Equal(v.T(), diagnoseIncludeEverySuiteSummary, diagnoseSummary)
}

func (v *baseDiagnoseSuite) AssertDiagnoseJSONInclude() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--json"}))
	diagnoseResult := unmarshalDiagnose(diagnose)
	assert.NotNil(v.T(), diagnoseResult)
	diagnoseSummary := diagnoseResult.Summary
	for _, suite := range v.suites {
		diagnoseInclude := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--json", "--include", suite}))
		diagnoseIncludeResult := unmarshalDiagnose(diagnoseInclude)
		assert.NotNil(v.T(), diagnoseIncludeResult)

		resultInclude := diagnoseIncludeResult.Summary

		assert.Less(v.T(), resultInclude.Total, diagnoseSummary.Total, "Expected number of checks for suite %v to be lower than the total amount of checks (%v) but was %v", suite, diagnoseSummary.Total, resultInclude.Total)
		assert.Zero(v.T(), resultInclude.Fail)
		assert.Zero(v.T(), resultInclude.UnexpectedErr)
	}

	// Create an args array to include all suites
	includeArgs := strings.Split(" --json "+" --include "+strings.Join(v.suites, " --include "), " ")

	// Diagnose with all suites included should be equal to diagnose without args
	diagnoseIncludeEverySuite := getDiagnoseOutput(v, agentclient.WithArgs(includeArgs))
	diagnoseIncludeEverySuiteResult := unmarshalDiagnose(diagnoseIncludeEverySuite)
	assert.NotNil(v.T(), diagnoseIncludeEverySuiteResult)
	assert.Equal(v.T(), diagnoseIncludeEverySuiteResult.Summary, diagnoseSummary)
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
		assert.NotNil(v.T(), diagnoseExcludeResult)

		resultExclude := diagnoseExcludeResult.Summary

		assert.Equal(v.T(), resultExclude.Fail, 0)
		assert.Equal(v.T(), resultExclude.UnexpectedErr, 0)
	}

	// Create an args array to exclude all suites
	excludeArgs := strings.Split(" --json "+" --exclude "+strings.Join(v.suites, " --exclude "), " ")

	// Diagnose with all suites excluded should do nothing
	diagnoseExcludeEverySuite := getDiagnoseOutput(v, agentclient.WithArgs(excludeArgs))
	diagnoseExcludeEverySuiteResult := unmarshalDiagnose(diagnoseExcludeEverySuite)
	assert.NotNil(v.T(), diagnoseExcludeEverySuiteResult)
	assert.Equal(v.T(), diagnoseExcludeEverySuiteResult.Summary.Total, 0)

}

func (v *baseDiagnoseSuite) TestDiagnoseVerbose() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"-v"}))
	summary := getDiagnoseSummary(diagnose)
	re := regexp.MustCompile("PASS")
	matches := re.FindAllString(diagnose, -1)
	// Verify that verbose mode display extra information such 'PASS' for successful checks
	assert.Equal(v.T(), len(matches), summary.Total, "Expected to have the same number of 'PASS' as the number of checks (%v), but was %v", summary.Total, len(matches))
	assert.Contains(v.T(), diagnose, "connectivity-datadog-core-endpoints")
}

func (v *baseDiagnoseSuite) TestDiagnoseJSON() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"-v", "--json"}))
	diagnoseResult := unmarshalDiagnose(diagnose)
	assert.NotNil(v.T(), diagnoseResult)

	summary := diagnoseResult.Summary

	// Verify that verbose mode displays extra information such as 'PASS' for successful checks
	assert.Equal(v.T(), summary.Success, summary.Total, "Expected to have the same number of 'PASS' as the number of checks (%v), but was %v", summary.Total, summary.Success)
	assert.Contains(v.T(), diagnose, "connectivity-datadog-core-endpoints")
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

// unmarshalDiagnose converts a diagnose string to a DiagnoseResult struct
func unmarshalDiagnose(s string) *diagnoseResult {
	result := &diagnoseResult{}
	err := json.Unmarshal([]byte(s), result)
	if err != nil {
		return nil
	}
	return result
}
