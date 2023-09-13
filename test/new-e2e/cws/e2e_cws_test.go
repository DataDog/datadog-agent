// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cws "github.com/DataDog/datadog-agent/test/new-e2e/cws/lib"
	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

type agentSuite struct {
	e2e.Suite[e2e.AgentEnv]
	apiClient     cws.MyApiClient
	signalRuleId  string
	agentRuleID   string
	dirname       string
	filename      string
	testId        string
	desc          string
	agentRuleName string
	policies      string
}

func TestAgentSuite(t *testing.T) {
	agentConfig := "hostname: momar-e2e-test-host\nlog_level: DEBUG\nlogs_enabled: true"
	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef(
		[]ec2params.Option{
			ec2params.WithName("cws-e2e-tests"),
		},
		agentparams.WithAgentConfig(agentConfig),
		agentparams.WithSecurityAgentConfig("runtime_security_config:\n  enabled: true"),
		agentparams.WithSystemProbeConfig("system_probe_config:\n  enabled: true\n  log_level: trace\nruntime_security_config:\n  enabled: true\nlog_patterns:\n  - module.APIServer.*"),
		agentparams.WithVersion("7.46.0"),
	), params.WithDevMode())
}

func (a *agentSuite) SetupSuite() {
	// Create temporary directory
	tempDir := a.Env().VM.Execute("mktemp -d")
	a.dirname = strings.TrimSuffix(tempDir, "\n")
	a.filename = fmt.Sprintf("%s/secret", a.dirname)
	a.testId = uuid.NewString()[:4]
	a.desc = fmt.Sprintf("e2e test rule %s", a.testId)
	a.agentRuleName = fmt.Sprintf("e2e_agent_rule_%s", a.testId)
	a.Suite.SetupSuite()
}

func (a *agentSuite) TearDownSuite() {

	if len(a.signalRuleId) != 0 {
		a.apiClient.DeleteSignalRule(a.signalRuleId)
	}
	if len(a.agentRuleID) != 0 {
		a.apiClient.DeleteAgentRule(a.agentRuleID)
	}
	a.Env().VM.Execute(fmt.Sprintf("rm -r %s", a.dirname))
	a.Suite.TearDownSuite()
}

func (a *agentSuite) TestOpenSignal() {
	a.apiClient = cws.NewApiClient()

	// Create CWS Agent rule
	rule := fmt.Sprintf("open.file.path == \"%s\"", a.filename)
	res, err := a.apiClient.CreateCWSAgentRule(a.agentRuleName, a.desc, rule)
	require.NoError(a.T(), err, "Agent rule creation failed")
	a.agentRuleID = res.Data.GetId()

	// Create Signal Rule (backend)
	res2, err := a.apiClient.CreateCwsSignalRule(a.desc, "signal rule for e2e testing", a.agentRuleName, []string{})
	require.NoError(a.T(), err, "Signal rule creation failed")
	a.signalRuleId = res2.GetId()

	a.Env().Agent.WaitForReady()

	// Check if the agent has started
	isReady, err := a.Env().Agent.IsReady()
	require.NoError(a.T(), err)

	assert.Equal(a.T(), isReady, true, "Agent should be ready")

	// Check if the agent use the right configuration
	assert.Contains(a.T(), a.Env().Agent.Config(), "log_level: DEBUG")

	// Check if system-probe has started
	err = cws.WaitAgentLogs(a.Env().VM, "system-probe", cws.SYS_PROBE_START_LOG)
	require.NoError(a.T(), err, "system-probe could not start")

	// Check if security-agent has started
	err = cws.WaitAgentLogs(a.Env().VM, "security-agent", cws.SECURITY_START_LOG)
	require.NoError(a.T(), err, "security-agent could not start")

	// Download policies
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(a.T(), err, "Could not get API KEY")

	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(a.T(), err, "Could not get APP KEY")

	policies := a.Env().VM.Execute(fmt.Sprintf("DD_APP_KEY=%s DD_API_KEY=%s DD_SITE=datadoghq.com %s runtime policy download", appKey, apiKey, cws.SEC_AGENT_PATH))

	assert.NotEmpty(a.T(), policies, "should not be empty")
	a.policies = policies

	// Check that the newly created rule is in the policies
	assert.Contains(a.T(), a.policies, a.desc, "The policies should contain the created rule")

	// Push policies
	a.Env().VM.Execute(fmt.Sprintf("echo -e %s > temp.txt\nsudo cp temp.txt %s", strconv.Quote(a.policies), cws.POLICIES_PATH))
	a.Env().VM.Execute("rm temp.txt")
	policiesFile := a.Env().VM.Execute(fmt.Sprintf("cat %s", cws.POLICIES_PATH))
	assert.Contains(a.T(), policiesFile, a.desc, "The policies file should contain the created rule")

	// Reload policies
	a.Env().VM.Execute(fmt.Sprintf("sudo %s runtime policy reload", cws.SEC_AGENT_PATH))

	// Check `downloaded` ruleset_loaded
	result, err := cws.WaitAppLogs(a.apiClient, "rule_id:ruleset_loaded")
	require.NoError(a.T(), err, "could not get new ruleset")

	agentContext := result.Attributes["agent"].(map[string]interface{})
	assert.EqualValues(a.T(), "ruleset_loaded", agentContext["rule_id"], "Ruleset should be loaded")

	// Trigger agent event
	a.Env().VM.Execute(fmt.Sprintf("touch %s", a.filename))

	// Check agent event
	err = cws.WaitAgentLogs(a.Env().VM, "security-agent", "Successfully posted payload to")
	require.NoError(a.T(), err, "could not send payload")

	// Check app signal
	signal, err := cws.WaitAppSignal(a.apiClient, fmt.Sprintf("rule_id:%s", a.agentRuleName))
	require.NoError(a.T(), err)
	assert.Contains(a.T(), signal.Tags, fmt.Sprintf("rule_id:%s", a.agentRuleName), "unable to find agent_rule_name tag")
	agentContext = signal.Attributes["agent"].(map[string]interface{})
	assert.Contains(a.T(), agentContext["rule_id"], a.agentRuleName, "unable to find tag")

}
