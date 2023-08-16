// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

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
	dirname       string
	filename      string
	testId        string
	desc          string
	agentRuleName string
}

func TestAgentSuite(t *testing.T) {
	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef(
		[]ec2params.Option{
			ec2params.WithName("cws-e2e-tests"),
		},
		agentparams.WithAgentConfig("log_level: debug"),
		agentparams.WithAgentConfig("rc_enabled: true"),
		agentparams.WithVersion("7.46.0"),
	), params.WithDevMode())
}

func (a *agentSuite) SetupSuite() {
	// Create temporary directory
	a.dirname = "e2e-temp-folder"
	_, err := a.Env().VM.ExecuteWithError(fmt.Sprintf("mkdir -p %s", a.dirname))
	if err != nil {
		fmt.Println("Can't create temporary dir")
	}
	a.filename = fmt.Sprintf("%s/secret", a.dirname)
	a.testId = uuid.NewString()[:4]
	a.desc = fmt.Sprintf("e2e test rule %s", a.testId)
	a.agentRuleName = fmt.Sprintf("e2e_agent_rule_%s", a.testId)
}

func (a *agentSuite) TearDownSuite() {
	// Delete temporary directory
	a.Env().VM.ExecuteWithError(fmt.Sprintf("rm -r %s", a.dirname))

	if len(a.signalRuleId) != 0 {
		a.apiClient.DeleteSignalRule(a.signalRuleId)
	}
	if len(a.agentRuleID) != 0 {
		a.apiClient.DeleteAgentRule(a.agentRuleID)
	}
}

func (a *agentSuite) TestRulesCreation() {
	a.apiClient = NewApiClient()
	res, err := a.apiClient.CreateCWSAgentRule(a.agentRuleName, "Description of my custom rule", "open.file.path == \"/tmp\"")
	if err != nil {
		fmt.Println("Error", err)
	}
	a.agentRuleID = res.Data.GetId()
	res2, err2 := a.apiClient.CreateCwsSignalRule(a.desc, a.desc, a.agentRuleName, []string{})
	if err2 != nil {
		fmt.Println("Error", err2)
	}
	a.signalRuleId = res2.GetId()
}

func (a *agentSuite) TestAgentStart() {
	a.UpdateEnv(e2e.AgentStackDef(
		[]ec2params.Option{},
		agentparams.WithAgentConfig(fmt.Sprintf("hostname: host-%s", a.testId)),
		agentparams.WithAgentConfig(`tags: ["tag1", "tag2"]`),
		agentparams.WithAgentConfig("security_agent.remote_workloadmeta: true"),
		agentparams.WithAgentConfig("remote_configuraiton.enabled: true"),
		agentparams.WithAgentConfig("system_probe_config.log_level: trace"),
		agentparams.WithAgentConfig("runtime_security_config.log_patterns: module.APIServer.*"),
		agentparams.WithAgentConfig("runtime_security_config.enabled: true"),
		agentparams.WithAgentConfig("runtime_security_config.network.enabled: true"),
		agentparams.WithAgentConfig("runtime_security_config.remote_configuration.enabled: true"),
	))
	a.Env().Agent.WaitForReady()
	isReady, err := a.Env().Agent.IsReady()
	if err != nil {
		fmt.Println(err)
	}
	assert.Equal(a.T(), isReady, true, "Agent should be ready")
	// assert.Contains(a.T(), a.Env().Agent.Config(), "log_level: debug")
	// assert.Contains(a.T(), a.Env().Agent.Config(), "runtime_security_config.enabled: true")
	// assert.Contains(a.T(), a.Env().Agent.Config(), "runtime_security_config.network.enabled: true")
}
