// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"os"
	"path/filepath"
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
	apiClient     apiClient
	signalRuleId  string
	agentRuleID   string
	filename      string
	testId        string
	desc          string
	agentRuleName string
	policies      string
}

func TestAgentSuite(t *testing.T) {
	agentConfig := "hostname: host-custom-e2e\nlog_level: trace"
	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef(
		[]ec2params.Option{
			ec2params.WithName("cws-e2e-tests"),
		},
		agentparams.WithAgentConfig(agentConfig),
		agentparams.WithSecurityAgentConfig("runtime_security_config:\n  enabled: true"),
		agentparams.WithSystemProbeConfig("system_probe_config:\n  enabled: true\nruntime_security_config:\n  enabled: true"),
		agentparams.WithVersion("7.46.0"),
	), params.WithDevMode())
}

func (a *agentSuite) SetupSuite() {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "e2e-temp-folder")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	dirName := filepath.Base(tempDir)
	a.filename = fmt.Sprintf("%s/secret", dirName)
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
	a.Suite.TearDownSuite()
}

func (a *agentSuite) TestRulesCreation() {

	a.apiClient = NewApiClient()

	// Create CWS Agent rule
	res, err := a.apiClient.CreateCWSAgentRule(a.agentRuleName, a.desc, "open.file.path == \"/tmp\"")
	assert.NoError(a.T(), err, "Agent rule creation failed")
	a.agentRuleID = res.Data.GetId()

	// Create Signal Rule (backend)
	res2, err2 := a.apiClient.CreateCwsSignalRule(a.desc, "signal rule for e2e testing", a.agentRuleName, []string{})
	assert.NoError(a.T(), err2, "Signal rule creation failed")
	a.signalRuleId = res2.GetId()

	a.Env().Agent.WaitForReady()

	// Check if the agent has started
	isReady, err := a.Env().Agent.IsReady()
	if err != nil {
		fmt.Println(err)
	}
	assert.Equal(a.T(), isReady, true, "Agent should be ready")

	// Check if the agent use the right configuration
	assert.Contains(a.T(), a.Env().Agent.Config(), "log_level: trace")

	// Check if system-probe has started
	err = cws.WaitAgentLogs(a.Env().VM, "system-probe", cws.SYS_PROBE_START_LOG)
	require.NoError(a.T(), err, "system-probe could not start")

	// Check if security-agent has started
	err = cws.WaitAgentLogs(a.Env().VM, "security-agent", cws.SECURITY_START_LOG)
	require.NoError(a.T(), err, "security-agent could not start")

	// Download policies
	apiKey, _ := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	appKey, _ := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	policies, err := a.Env().VM.ExecuteWithError(fmt.Sprintf("DD_APP_KEY=%s DD_API_KEY=%s DD_SITE=datadoghq.com %s runtime policy download", appKey, apiKey, cws.SEC_AGENT_PATH))
	if err != nil {
		fmt.Println("Error", err)
	}
	assert.NotEmpty(a.T(), policies, "should not be empty")
	a.policies = policies

	// Check that the newly created rule is in the policies
	assert.Contains(a.T(), a.policies, a.desc, "The policies should contain the created rule")

	// Push the policies
	a.Env().VM.Execute(fmt.Sprintf("echo %s > %s", a.policies, cws.POLICIES_PATH))

	// Reload policies
	a.Env().VM.Execute(fmt.Sprintf("%s runtime policy reload", cws.SEC_AGENT_PATH))

	// Check `downloaded` ruleset_loaded

}
