// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentDiagnoseSuite struct {
	e2e.BaseSuite[environments.Host]
}

var allSuites = []string{
	"check-datadog",
	"connectivity-datadog-autodiscovery",
	"connectivity-datadog-core-endpoints",
	"connectivity-datadog-event-platform",
}

func TestAgentDiagnoseEC2Suite(t *testing.T) {
	e2e.Run(t, &agentDiagnoseSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// type summary represents the number of success, fail, warnings and errors of a diagnose command
type summary struct {
	total    int
	success  int
	warnings int
	fail     int
	errors   int
}

func getDiagnoseOutput(v *agentDiagnoseSuite, commandArgs ...agentclient.AgentArgsOption) string {
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().FakeIntake.Client().GetServerHealth())
	}, 5*time.Minute, 20*time.Second, "timedout waiting for fakeintake to be healthy")

	return v.Env().Agent.Client.Diagnose(commandArgs...)
}

func (v *agentDiagnoseSuite) TestDiagnoseDefaultConfig() {
	diagnose := getDiagnoseOutput(v)
	assert.NotContains(v.T(), diagnose, "FAIL")
}

func (v *agentDiagnoseSuite) TestDiagnoseLocal() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--local"}))
	assert.NotContains(v.T(), diagnose, "FAIL")
}

func (v *agentDiagnoseSuite) TestDiagnoseLocalFallback() {
	svcManager := svcmanager.NewSystemctl(v.Env().RemoteHost)
	svcManager.Stop("datadog-agent")

	diagnose := getDiagnoseOutput(v)
	assert.Contains(v.T(), diagnose, "Running diagnose command locally", "Expected diagnose command to fallback to local diagnosis when the Agent is stopped, but it did not.")
	assert.NotContains(v.T(), diagnose, "FAIL")

	svcManager.Start("datadog-agent")
}

func (v *agentDiagnoseSuite) TestDiagnoseOtherCmdPort() {
	params := agentparams.WithAgentConfig("cmd_port: 4567")
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(v)
	assert.NotContains(v.T(), diagnose, "FAIL")
}

func (v *agentDiagnoseSuite) TestDiagnoseList() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--list"}))
	for _, suite := range allSuites {
		assert.Contains(v.T(), diagnose, suite)
	}
}

func (v *agentDiagnoseSuite) TestDiagnoseInclude() {
	diagnose := getDiagnoseOutput(v)
	diagnoseSummary := getDiagnoseSummary(diagnose)

	for _, suite := range allSuites {
		diagnoseInclude := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--include", suite}))
		resultInclude := getDiagnoseSummary(diagnoseInclude)

		assert.Less(v.T(), resultInclude.total, diagnoseSummary.total, "Expected number of checks for suite %v to be lower than the total amount of checks (%v) but was %v", suite, diagnoseSummary.total, resultInclude.total)
		assert.Zero(v.T(), resultInclude.fail)
		assert.Zero(v.T(), resultInclude.errors)
	}

	// Create an args array to include all suites
	includeArgs := strings.Split("--include "+strings.Join(allSuites, " --include "), " ")

	// Diagnose with all suites included should be equal to diagnose without args
	diagnoseIncludeEverySuite := getDiagnoseOutput(v, agentclient.WithArgs(includeArgs))
	diagnoseIncludeEverySuiteSummary := getDiagnoseSummary(diagnoseIncludeEverySuite)
	assert.Equal(v.T(), diagnoseIncludeEverySuiteSummary, diagnoseSummary)
}

func (v *agentDiagnoseSuite) TestDiagnoseExclude() {
	for _, suite := range allSuites {
		diagnoseExclude := getDiagnoseOutput(v, agentclient.WithArgs([]string{"--exclude", suite}))
		resultExclude := getDiagnoseSummary(diagnoseExclude)

		assert.Equal(v.T(), resultExclude.fail, 0)
		assert.Equal(v.T(), resultExclude.errors, 0)
	}

	// Create an args array to exclude all suites
	excludeArgs := strings.Split("--exclude "+strings.Join(allSuites, " --exclude "), " ")

	// Diagnose with all suites excluded should do nothing
	diagnoseExcludeEverySuite := getDiagnoseOutput(v, agentclient.WithArgs(excludeArgs))
	summary := getDiagnoseSummary(diagnoseExcludeEverySuite)
	assert.Equal(v.T(), summary.total, 0)
}

func (v *agentDiagnoseSuite) TestDiagnoseVerbose() {
	diagnose := getDiagnoseOutput(v, agentclient.WithArgs([]string{"-v"}))
	summary := getDiagnoseSummary(diagnose)

	re := regexp.MustCompile("PASS")
	matches := re.FindAllString(diagnose, -1)

	// Verify that verbose mode display extra information such 'PASS' for successful checks
	assert.Equal(v.T(), len(matches), summary.total, "Expected to have the same number of 'PASS' as the number of checks (%v), but was %v", summary.total, len(matches))
	assert.Contains(v.T(), diagnose, "connectivity-datadog-core-endpoints")
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
func getDiagnoseSummary(diagnoseOutput string) summary {
	matches := summaryRE.FindStringSubmatch(diagnoseOutput)

	return summary{
		total:    getRegexGroupValue(summaryRE, matches, "total"),
		success:  getRegexGroupValue(summaryRE, matches, "success"),
		warnings: getRegexGroupValue(summaryRE, matches, "warning"),
		fail:     getRegexGroupValue(summaryRE, matches, "fail"),
		errors:   getRegexGroupValue(summaryRE, matches, "error"),
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
