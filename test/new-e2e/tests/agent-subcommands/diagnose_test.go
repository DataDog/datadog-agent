// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	_ "embed"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentDiagnoseSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

var allSuites = []string{
	"check-datadog",
	"connectivity-datadog-autodiscovery",
	"connectivity-datadog-core-endpoints",
	"connectivity-datadog-event-platform",
}

func TestAgentDiagnoseEC2Suite(t *testing.T) {
	e2e.Run(t, &agentDiagnoseSuite{}, e2e.FakeIntakeStackDef(), params.WithDevMode())
}

type summary struct {
	total    int
	success  int
	warnings int
	fail     int
	errors   int
}

func getDiagnoseOutput(v *agentDiagnoseSuite, commandArgs ...client.AgentArgsOption) string {
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().Fakeintake.Client.GetServerHealth())
	}, 5*time.Minute, 20*time.Second)

	return v.Env().Agent.Diagnose(commandArgs...)
}

func (v *agentDiagnoseSuite) TestDiagnoseDefaultConfig() {
	diagnose := getDiagnoseOutput(v)
	assert.NotContains(v.T(), diagnose, "FAIL")
}

func (v *agentDiagnoseSuite) TestDiagnoseLocal() {
	diagnose := getDiagnoseOutput(v, client.WithArgs([]string{"--local"}))
	assert.NotContains(v.T(), diagnose, "FAIL")
}

func (v *agentDiagnoseSuite) TestDiagnoseLocalFallback() {
	svcManager := svcmanager.NewSystemctlSvcManager(v.Env().VM)
	svcManager.Stop("datadog-agent")

	diagnose := getDiagnoseOutput(v)
	assert.Contains(v.T(), diagnose, "Running diagnose command locally")
	assert.NotContains(v.T(), diagnose, "FAIL")

	svcManager.Start("datadog-agent")
}

func (v *agentDiagnoseSuite) TestDiagnoseOtherCmdPort() {
	params := agentparams.WithAgentConfig("cmd_port: 4567")
	v.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithAgentParams(params)))

	diagnose := getDiagnoseOutput(v)
	assert.NotContains(v.T(), diagnose, "FAIL")
}

func (v *agentDiagnoseSuite) TestDiagnoseList() {

	diagnose := getDiagnoseOutput(v, client.WithArgs([]string{"--list"}))
	for _, suite := range allSuites {
		assert.Contains(v.T(), diagnose, suite)
	}
}

func (v *agentDiagnoseSuite) TestDiagnoseInclude() {
	diagnoseAll := getDiagnoseOutput(v)
	resultAll := getDiagnoseSummary(diagnoseAll)

	total := 0
	for _, suite := range allSuites {
		diagnoseInclude := getDiagnoseOutput(v, client.WithArgs([]string{"--include", suite}))
		resultInclude := getDiagnoseSummary(diagnoseInclude)

		assert.Less(v.T(), resultInclude.total, resultAll.total)
		assert.Equal(v.T(), resultInclude.fail, 0)
		assert.Equal(v.T(), resultInclude.errors, 0)

		total += resultInclude.total
	}

	assert.Equal(v.T(), total, resultAll.total)

	allinclude := strings.Split("--include "+strings.Join(allSuites, " --include "), " ")
	diagnoseAllInclude := getDiagnoseOutput(v, client.WithArgs(allinclude))
	diagnoseAllIncludeSummary := getDiagnoseSummary(diagnoseAllInclude)
	assert.Equal(v.T(), diagnoseAllIncludeSummary, diagnoseAll)
}

func (v *agentDiagnoseSuite) TestDiagnoseExclude() {
	diagnoseAll := getDiagnoseOutput(v)
	resultAll := getDiagnoseSummary(diagnoseAll)

	for _, suite := range allSuites {
		diagnoseExclude := getDiagnoseOutput(v, client.WithArgs([]string{"--exclude", suite}))
		resultExclude := getDiagnoseSummary(diagnoseExclude)

		assert.Less(v.T(), resultExclude.total, resultAll.total)
		assert.Equal(v.T(), resultExclude.fail, 0)
		assert.Equal(v.T(), resultExclude.errors, 0)
	}

	allExclude := strings.Split("--exclude "+strings.Join(allSuites, " --exclude "), " ")
	diagnoseAllExclude := getDiagnoseOutput(v, client.WithArgs(allExclude))
	diagnoseAllExcludeSummary := getDiagnoseSummary(diagnoseAllExclude)
	assert.Equal(v.T(), diagnoseAllExcludeSummary.total, 0)
}

func getDiagnoseSummary(diagnoseOutput string) summary {

	successRegex := `(?:, Success:(?P<success>\d+))?`
	failRegex := `(?:, Fail:(?P<fail>\d+))?`
	warningRegex := `(?:, Warning:(?P<warning>\d+))?`
	errorRegex := `(?:, Error:(?P<error>\d+))?`
	regexTemplate := `Total:(?P<total>\d+)` + successRegex + failRegex + warningRegex + errorRegex

	re := regexp.MustCompile(regexTemplate)
	matches := re.FindStringSubmatch(diagnoseOutput)

	return summary{
		total:    getRegexGroupValue(re, matches, "total"),
		success:  getRegexGroupValue(re, matches, "success"),
		warnings: getRegexGroupValue(re, matches, "warning"),
		fail:     getRegexGroupValue(re, matches, "fail"),
		errors:   getRegexGroupValue(re, matches, "error"),
	}
}

func getRegexGroupValue(re *regexp.Regexp, matches []string, groupName string) int {
	index := re.SubexpIndex(groupName)
	if index < 0 || index >= len(matches) {
		return 0
	}

	val, err := strconv.Atoi(matches[index])
	if err != nil {
		return 0
	}

	return val
}
