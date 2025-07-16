// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package diagnose

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type windowsDiagnoseSuite struct {
	baseDiagnoseSuite
}

func TestWindowsDiagnoseSuite(t *testing.T) {
	t.Parallel()
	var suite windowsDiagnoseSuite
	suite.suites = append(suite.suites, commonSuites...)
	e2e.Run(t, &suite, e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (v *windowsDiagnoseSuite) TestDiagnoseOtherCmdPort() {
	params := agentparams.WithAgentConfig("cmd_port: 4567")
	v.UpdateEnv(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	v.AssertOutputNotError(diagnose)
}

func (v *windowsDiagnoseSuite) TestDiagnoseInclude() {
	v.AssertDiagnoseInclude()
	v.AssertDiagnoseJSONInclude()
}

func (v *windowsDiagnoseSuite) TestDiagnoseExclude() {
	v.AssertDiagnoseExclude()
	v.AssertDiagnoseJSONExclude()
}

// TestDiagnoseVerbose overrides the base method to handle agent-account-check warnings
func (v *windowsDiagnoseSuite) TestDiagnoseVerbose() {
	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"-v"}))
	summary := getDiagnoseSummary(diagnose)

	// Count PASS matches and WARNING matches from agent-account-check specifically
	passRE := regexp.MustCompile("PASS")
	passMatches := passRE.FindAllString(diagnose, -1)

	// Count warnings specifically from agent-account-check suite
	agentAccountWarningRE := regexp.MustCompile(`WARNING \[agent-account-check\]`)
	agentAccountWarnings := len(agentAccountWarningRE.FindAllString(diagnose, -1))

	// Verify no unexpected warnings from other suites
	allWarningRE := regexp.MustCompile(`WARNING \[([^\]]+)\]`)
	allWarningMatches := allWarningRE.FindAllStringSubmatch(diagnose, -1)
	for _, match := range allWarningMatches {
		if len(match) > 1 && match[1] != "agent-account-check" {
			assert.Fail(v.T(), "Warning found in suite '%s', but warnings should only come from agent-account-check", match[1])
		}
	}

	// Verify total matches: PASS + agent-account-check warnings should equal total checks
	assert.Equal(v.T(), len(passMatches)+agentAccountWarnings, summary.Total,
		"Expected PASS count (%d) + agent-account-check warnings (%d) to equal total checks (%d)",
		len(passMatches), agentAccountWarnings, summary.Total)

	assert.Contains(v.T(), diagnose, "connectivity-datadog-core-endpoints")
}

// TestDiagnoseJSON overrides the base method to specifically handle agent-account-check warnings
func (v *windowsDiagnoseSuite) TestDiagnoseJSON() {
	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"-v", "--json"}))

	// Parse the full JSON structure to check warnings per suite
	var fullResult struct {
		Runs []struct {
			Name      string `json:"suite_name"`
			Diagnoses []struct {
				Status int    `json:"result"`
				Name   string `json:"name"`
			} `json:"diagnoses"`
		} `json:"runs"`
		Summary struct {
			Total         int `json:"total"`
			Success       int `json:"success"`
			Fail          int `json:"fail"`
			Warnings      int `json:"warnings"`
			UnexpectedErr int `json:"unexpected_error"`
		} `json:"summary"`
	}

	err := json.Unmarshal([]byte(diagnose), &fullResult)
	require.NoError(v.T(), err, "Failed to parse diagnose JSON output")

	// Verify no failures or unexpected errors occurred
	assert.Zero(v.T(), fullResult.Summary.Fail, "Expected no failed checks")
	assert.Zero(v.T(), fullResult.Summary.UnexpectedErr, "Expected no unexpected errors")

	// Count warnings specifically from agent-account-check and validate warning sources
	agentAccountWarnings := 0
	for _, run := range fullResult.Runs {
		for _, diagnosis := range run.Diagnoses {
			if diagnosis.Status == 2 { // DiagnosisWarning = 2
				if run.Name == "agent-account-check" {
					agentAccountWarnings++
				} else {
					assert.Fail(v.T(), "Unexpected warning found",
						"Warning found in suite '%s' (check: '%s'), but warnings should only come from agent-account-check",
						run.Name, diagnosis.Name)
				}
			}
		}
	}

	// Verify global math using only agent-account-check warnings
	assert.Equal(v.T(), fullResult.Summary.Success+agentAccountWarnings, fullResult.Summary.Total,
		"Expected Success + AgentAccountWarnings to equal Total (Success: %d, AgentAccountWarnings: %d, Total: %d)",
		fullResult.Summary.Success, agentAccountWarnings, fullResult.Summary.Total)

	assert.Contains(v.T(), diagnose, "connectivity-datadog-core-endpoints")
}
