// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	cws "github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/lib"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

type agentSuite struct {
	e2e.Suite[e2e.AgentEnv]
	apiClient     cws.MyAPIClient
	signalRuleID  string
	agentRuleID   string
	dirname       string
	filename      string
	testID        string
	desc          string
	agentRuleName string
	policies      string
}

//go:embed config/e2e-datadog.yaml
var agentConfig string

//go:embed config/e2e-system-probe.yaml
var systemProbeConfig string

//go:embed config/e2e-security-agent.yaml
var securityAgentConfig string

func TestAgentSuite(t *testing.T) {

	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef(
		e2e.WithVMParams(ec2params.WithName("cws-e2e-tests")),
		e2e.WithAgentParams(
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithSecurityAgentConfig(securityAgentConfig),
			agentparams.WithSystemProbeConfig(systemProbeConfig),
		),
	))
}

func (a *agentSuite) SetupSuite() {

	// Create temporary directory
	tempDir := a.Env().VM.Execute("mktemp -d")
	a.dirname = strings.TrimSuffix(tempDir, "\n")
	a.filename = fmt.Sprintf("%s/secret", a.dirname)
	a.testID = uuid.NewString()[:4]
	a.desc = fmt.Sprintf("e2e test rule %s", a.testID)
	a.agentRuleName = fmt.Sprintf("e2e_agent_rule_%s", a.testID)
	a.Suite.SetupSuite()
}

func (a *agentSuite) TearDownSuite() {

	if len(a.signalRuleID) != 0 {
		a.apiClient.DeleteSignalRule(a.signalRuleID)
	}
	if len(a.agentRuleID) != 0 {
		a.apiClient.DeleteAgentRule(a.agentRuleID)
	}
	a.Env().VM.Execute(fmt.Sprintf("rm -r %s", a.dirname))
	a.Suite.TearDownSuite()
}

func (a *agentSuite) TestOpenSignal() {
	a.apiClient = cws.NewAPIClient()

	// Create CWS Agent rule
	rule := fmt.Sprintf("open.file.path == \"%s\"", a.filename)
	res, err := a.apiClient.CreateCWSAgentRule(a.agentRuleName, a.desc, rule)
	require.NoError(a.T(), err, "Agent rule creation failed")
	a.agentRuleID = res.Data.GetId()

	// Create Signal Rule (backend)
	res2, err := a.apiClient.CreateCwsSignalRule(a.desc, "signal rule for e2e testing", a.agentRuleName, []string{})
	require.NoError(a.T(), err, "Signal rule creation failed")
	a.signalRuleID = res2.GetId()

	// Check if the agent is ready
	isReady := a.Env().Agent.IsReady()
	assert.Equal(a.T(), isReady, true, "Agent should be ready")

	// Check if system-probe has started
	err = a.Env().Agent.WaitAgentLogs("system-probe", cws.SystemProbeStartLog)
	require.NoError(a.T(), err, "system-probe could not start")

	// Check if security-agent has started
	err = a.Env().Agent.WaitAgentLogs("security-agent", cws.SecurityStartLog)
	require.NoError(a.T(), err, "security-agent could not start")

	// Download policies
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(a.T(), err, "Could not get API KEY")

	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(a.T(), err, "Could not get APP KEY")

	a.EventuallyWithT(func(c *assert.CollectT) {
		policies := a.Env().VM.Execute(fmt.Sprintf("DD_APP_KEY=%s DD_API_KEY=%s DD_SITE=datadoghq.com %s runtime policy download", appKey, apiKey, cws.SecurityAgentPath))
		assert.NotEmpty(c, policies, "should not be empty")
		a.policies = policies
	}, 5*time.Minute, 10*time.Second)

	// Check that the newly created rule is in the policies
	assert.Contains(a.T(), a.policies, a.desc, "The policies should contain the created rule")

	// Push policies
	a.Env().VM.Execute(fmt.Sprintf("echo -e %s > temp.txt\nsudo cp temp.txt %s", strconv.Quote(a.policies), cws.PoliciesPath))
	a.Env().VM.Execute("rm temp.txt")
	policiesFile := a.Env().VM.Execute(fmt.Sprintf("cat %s", cws.PoliciesPath))
	assert.Contains(a.T(), policiesFile, a.desc, "The policies file should contain the created rule")

	// Reload policies
	a.Env().VM.Execute(fmt.Sprintf("sudo %s runtime policy reload", cws.SecurityAgentPath))

	// Check `downloaded` ruleset_loaded
	result, err := cws.WaitAppLogs(a.apiClient, "rule_id:ruleset_loaded")
	require.NoError(a.T(), err, "could not get new ruleset")

	agentContext := result.Attributes["agent"].(map[string]interface{})
	assert.EqualValues(a.T(), "ruleset_loaded", agentContext["rule_id"], "Ruleset should be loaded")

	// Trigger agent event
	a.Env().VM.Execute(fmt.Sprintf("touch %s", a.filename))

	// Check agent event
	err = a.Env().Agent.WaitAgentLogs("security-agent", "Successfully posted payload to")
	require.NoError(a.T(), err, "could not send payload")

	// Check app signal
	signal, err := cws.WaitAppSignal(a.apiClient, fmt.Sprintf("rule_id:%s", a.agentRuleName))
	require.NoError(a.T(), err)
	assert.Contains(a.T(), signal.Tags, fmt.Sprintf("rule_id:%s", a.agentRuleName), "unable to find agent_rule_name tag")
	agentContext = signal.Attributes["agent"].(map[string]interface{})
	assert.Contains(a.T(), agentContext["rule_id"], a.agentRuleName, "unable to find tag")

}
