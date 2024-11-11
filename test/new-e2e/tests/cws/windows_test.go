// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cws holds cws e2e tests
package cws

import (
	_ "embed"

	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	testos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/config"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

const (
	// windowsHostnamePrefix is the prefix of the hostname of the agent
	windowsHostnamePrefix = "cws-e2e-windows"

	// securityAgentPathWindows is the path of the security-agent binary
	securityAgentPathWindows = "C:/Program Files/Datadog/Datadog Agent/bin/agent/security-agent.exe"

	// policiesPathWindows is the path of the default runtime security policies
	policiesPathWindows = "C:/ProgramData/Datadog/runtime-security.d/test.policy"
)

type agentSuiteWindows struct {
	e2e.BaseSuite[environments.Host]
	apiClient *api.Client
	testID    string
}

func TestAgentWindowsSuite(t *testing.T) {
	testID := uuid.NewString()[:4]
	ddHostname := fmt.Sprintf("%s-%s", windowsHostnamePrefix, testID)
	agentConfig := config.GenDatadogAgentConfig(ddHostname, "tag1", "tag2")
	e2e.Run[environments.Host](t, &agentSuiteWindows{testID: testID},
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
					agentparams.WithSecurityAgentConfig(securityAgentConfig),
					agentparams.WithSystemProbeConfig(systemProbeConfig),
				),
				awshost.WithEC2InstanceOptions(ec2.WithOS(testos.WindowsDefault), ec2.WithInstanceType("t3.xlarge")),
			),
		),
	)
	t.Logf("Running testsuite with DD_HOSTNAME=%s", ddHostname)

}

func (a *agentSuiteWindows) SetupSuite() {
	a.BaseSuite.SetupSuite()
	a.apiClient = api.NewClient()
}

func (a *agentSuiteWindows) Hostname() string {
	return a.Env().Agent.Client.Hostname()
}

func (a *agentSuiteWindows) Client() *api.Client {
	return a.apiClient
}

func (a *agentSuiteWindows) Test00RulesetLoadedDefaultFile() {
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, a, "file", "default.policy")
	}, 4*time.Minute, 10*time.Second)
}

func (a *agentSuiteWindows) Test01RulesetLoadedDefaultRC() {
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, a, "remote-config", "default.policy")
	}, 4*time.Minute, 10*time.Second)
}

func (a *agentSuiteWindows) Test02Selftests() {
	time.Sleep(time.Minute)
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testSelftestsEvent(c, a, func(event *api.SelftestsEvent) {
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_windows_create_file", "missing selftest result")
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_windows_open_registry_key_name", "missing selftest result")
		})
	}, 4*time.Minute, 10*time.Second)
}

func (a *agentSuiteWindows) Test03CreateFileSignal() {
	var agentRuleID, signalRuleID, dirname string
	// Cleanup function
	defer func() {
		if signalRuleID != "" {
			err := a.apiClient.DeleteSignalRule(signalRuleID)
			assert.NoErrorf(a.T(), err, "failed to delete signal rule %s", signalRuleID)
		}
		if agentRuleID != "" {
			err := a.apiClient.DeleteAgentRule(agentRuleID)
			assert.NoErrorf(a.T(), err, "failed to delete agent rule %s", agentRuleID)
		}
		if dirname != "" {
			a.Env().RemoteHost.MustExecute(fmt.Sprintf("rm -r %s", dirname))
		}
	}()

	// Create temporary directory
	cmd := "New-Item -ItemType Directory -Path $env:TEMP -Name ([Guid]::NewGuid().Guid) | Select-Object -ExpandProperty FullName"
	tempDir := a.Env().RemoteHost.MustExecute(cmd)
	dirname = strings.TrimSpace(tempDir)
	filepath := fmt.Sprintf("%s\\secret", dirname)
	desc := fmt.Sprintf("e2e test rule %s", a.testID)
	agentRuleName := fmt.Sprintf("new_e2e_agent_rule_%s", a.testID)

	// Create CWS Agent rule
	rule := fmt.Sprintf(`create.file.path == "%s"`, filepath)
	res, err := a.apiClient.CreateCWSAgentRule(agentRuleName, desc, rule, []string{`os == "windows"`})
	require.NoError(a.T(), err, "Agent rule creation failed")
	agentRuleID = res.Data.GetId()

	// Create Signal Rule (backend)
	res2, err := a.apiClient.CreateCwsSignalRule(desc, "signal rule for e2e testing", agentRuleName, []string{})
	require.NoError(a.T(), err, "Signal rule creation failed")
	signalRuleID = *res2.SecurityMonitoringStandardRuleResponse.Id

	// Check if the agent is ready
	isReady := a.Env().Agent.Client.IsReady()
	assert.Equal(a.T(), isReady, true, "Agent should be ready")

	// Check if system-probe has started
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		output, err := a.Env().RemoteHost.Execute("cat C:/ProgramData/Datadog/logs/system-probe.log")
		if !assert.NoError(c, err) {
			return
		}
		assert.Contains(c, output, systemProbeStartLog, "system-probe could not start")
	}, 30*time.Second, 1*time.Second)

	// Check if security-agent has started
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		output, err := a.Env().RemoteHost.Execute("cat C:/ProgramData/Datadog/logs/security-agent.log")
		if !assert.NoError(c, err) {
			return
		}
		assert.Contains(c, output, securityStartLog, "security-agent could not start")
	}, 30*time.Second, 1*time.Second)

	// Wait for host tags
	time.Sleep(3 * time.Minute)

	// Download policies
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(a.T(), err, "Could not get API KEY")

	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(a.T(), err, "Could not get APP KEY")

	var policies string
	require.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		policies = a.Env().RemoteHost.MustExecute(fmt.Sprintf("$env:DD_APP_KEY='%s'; $env:DD_API_KEY='%s'; & '%s' runtime policy download | Out-File temp.txt; Get-Content temp.txt", appKey, apiKey, securityAgentPathWindows))
		assert.NotEmpty(c, policies, "should not be empty")
	}, 1*time.Minute, 1*time.Second)

	// Check that the newly created rule is in the policies
	require.Contains(a.T(), policies, desc, "The policies should contain the created rule")

	// Push policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("cp temp.txt '%s'; rm temp.txt", policiesPathWindows))
	policiesFile := a.Env().RemoteHost.MustExecute(fmt.Sprintf("cat %s", policiesPathWindows))
	require.Contains(a.T(), policiesFile, desc, "The policies file should contain the created rule")

	// Reload policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("& '%s' runtime policy reload", securityAgentPathWindows))

	// Check if the policy is loaded
	policyName := path.Base(policiesPathWindows)
	require.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, a, "file", policyName)
	}, 4*time.Minute, 5*time.Second)

	// Check 'datadog.security_agent.runtime.running' metric
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testMetricExists(c, a, "datadog.security_agent.runtime.running", map[string]string{"host": a.Hostname()})
	}, 4*time.Minute, 5*time.Second)

	// Trigger agent event
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("New-Item '%s' -ItemType File", filepath))

	// Check app event
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testRuleEvent(c, a, agentRuleName, func(e *api.RuleEvent) {
			assert.Equal(c, "create", e.Evt.Name, "event name should be create")
			assert.Equal(c, filepath, e.File.Path, "file path does not match")
			assert.Contains(c, e.Tags, "tag1", "missing event tag")
			assert.Contains(c, e.Tags, "tag2", "missing event tag")
		})
	}, 4*time.Minute, 10*time.Second)

	// Check app signal
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		signal, err := a.apiClient.GetSignal(fmt.Sprintf("host:%s @workflow.rule.id:%s", a.Env().Agent.Client.Hostname(), signalRuleID))
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotNil(c, signal) {
			return
		}
		assert.Contains(c, signal.Tags, fmt.Sprintf("rule_id:%s", strings.ToLower(agentRuleName)), "unable to find rule_id tag")
		if !assert.Contains(c, signal.AdditionalProperties, "attributes", "unable to find 'attributes' field in signal") {
			return
		}
		attributes := signal.AdditionalProperties["attributes"].(map[string]interface{})
		if !assert.Contains(c, attributes, "agent", "unable to find 'agent' field in signal's attributes") {
			return
		}
		agentContext := attributes["agent"].(map[string]interface{})
		if !assert.Contains(c, agentContext, "rule_id", "unable to find 'rule_id' in signal's agent context") {
			return
		}
		assert.Contains(c, agentContext["rule_id"], agentRuleName, "signal doesn't contain agent rule id")
	}, 4*time.Minute, 10*time.Second)
}

// test that the detection of CWS is properly working
// this test can be quite long so run it last
func (a *agentSuiteWindows) Test99CWSEnabled() {
	assert.EventuallyWithTf(a.T(), func(c *assert.CollectT) {
		testCwsEnabled(c, a)
	}, 20*time.Minute, 30*time.Second, "cws activation test timed out for host %s", a.Env().Agent.Client.Hostname())
}
