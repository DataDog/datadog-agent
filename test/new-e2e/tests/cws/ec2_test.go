// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cws holds cws e2e tests
package cws

import (
	_ "embed"
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/api"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

const (
	// ec2HostnamePrefix is the prefix of the hostname of the agent
	ec2HostnamePrefix = "cws-e2e-ec2-host"

	// securityStartLog is the log corresponding to a successful start of the security-agent
	securityStartLog = "Successfully connected to the runtime security module"

	// systemProbeStartLog is the log corresponding to a successful start of the system-probe
	systemProbeStartLog = "runtime security started"

	// securityAgentPath is the path of the security-agent binary
	securityAgentPath = "/opt/datadog-agent/embedded/bin/security-agent"

	// policiesPath is the path of the default runtime security policies
	policiesPath = "/etc/datadog-agent/runtime-security.d/test.policy"
)

type agentSuite struct {
	e2e.BaseSuite[environments.Host]
	apiClient *api.Client
	testID    string
}

//go:embed config/e2e-system-probe.yaml
var systemProbeConfig string

//go:embed config/e2e-security-agent.yaml
var securityAgentConfig string

func TestAgentSuite(t *testing.T) {
	testID := uuid.NewString()[:4]
	e2e.Run[environments.Host](t, &agentSuite{testID: testID},
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithAgentOptions(
					agentparams.WithAgentConfig(fmt.Sprintf("hostname: %s-%s", ec2HostnamePrefix, testID)),
					agentparams.WithSecurityAgentConfig(securityAgentConfig),
					agentparams.WithSystemProbeConfig(systemProbeConfig),
				),
			),
		),
	)
}

func (a *agentSuite) SetupSuite() {
	a.BaseSuite.SetupSuite()
	a.apiClient = api.NewClient()
}

func (a *agentSuite) Hostname() string {
	return a.Env().Agent.Client.Hostname()
}

func (a *agentSuite) Client() *api.Client {
	return a.apiClient
}

func (a *agentSuite) Test00RulesetLoadedDefaultFile() {
	assert.EventuallyWithT(a.T(), func(collect *assert.CollectT) {
		testRulesetLoaded(collect, a, "file", "default.policy")
	}, 2*time.Minute, 20*time.Second)
}

func (a *agentSuite) Test01RulesetLoadedDefaultRC() {
	assert.EventuallyWithT(a.T(), func(collect *assert.CollectT) {
		testRulesetLoaded(collect, a, "remote-config", "default.policy")
	}, 2*time.Minute, 20*time.Second)
}

func (a *agentSuite) Test02OpenSignal() {
	// Create temporary directory
	tempDir := a.Env().RemoteHost.MustExecute("mktemp -d")
	dirname := strings.TrimSuffix(tempDir, "\n")
	filename := fmt.Sprintf("%s/secret", dirname)
	desc := fmt.Sprintf("e2e test rule %s", a.testID)
	agentRuleName := fmt.Sprintf("new_e2e_agent_rule_%s", a.testID)

	// Create CWS Agent rule
	rule := fmt.Sprintf("open.file.path == \"%s\"", filename)
	res, err := a.apiClient.CreateCWSAgentRule(agentRuleName, desc, rule)
	require.NoError(a.T(), err, "Agent rule creation failed")
	agentRuleID := res.Data.GetId()

	// Create Signal Rule (backend)
	res2, err := a.apiClient.CreateCwsSignalRule(desc, "signal rule for e2e testing", agentRuleName, []string{})
	require.NoError(a.T(), err, "Signal rule creation failed")
	signalRuleID := res2.GetId()

	// Check if the agent is ready
	isReady := a.Env().Agent.Client.IsReady()
	assert.Equal(a.T(), isReady, true, "Agent should be ready")

	// Check if system-probe has started
	err = a.waitAgentLogs("system-probe", systemProbeStartLog)
	require.NoError(a.T(), err, "system-probe could not start")

	// Check if security-agent has started
	err = a.waitAgentLogs("security-agent", securityStartLog)
	require.NoError(a.T(), err, "security-agent could not start")

	// Download policies
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(a.T(), err, "Could not get API KEY")

	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(a.T(), err, "Could not get APP KEY")

	var policies string
	require.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		policies = a.Env().RemoteHost.MustExecute(fmt.Sprintf("DD_APP_KEY=%s DD_API_KEY=%s %s runtime policy download >| temp.txt && cat temp.txt", appKey, apiKey, securityAgentPath))
		assert.NotEmpty(c, policies, "should not be empty")
	}, 1*time.Minute, 5*time.Second)

	// Check that the newly created rule is in the policies
	require.Contains(a.T(), policies, desc, "The policies should contain the created rule")

	// Push policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo cp temp.txt %s && rm temp.txt", policiesPath))
	policiesFile := a.Env().RemoteHost.MustExecute(fmt.Sprintf("cat %s", policiesPath))
	require.Contains(a.T(), policiesFile, desc, "The policies file should contain the created rule")

	// Reload policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo %s runtime policy reload", securityAgentPath))

	// Check if the policy is loaded
	policyName := path.Base(policiesPath)
	require.EventuallyWithT(a.T(), func(collect *assert.CollectT) {
		testRulesetLoaded(collect, a, "file", policyName)
	}, 2*time.Minute, 20*time.Second)

	// Trigger agent event
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("touch %s", filename))

	// Check app signal
	assert.EventuallyWithT(a.T(), func(collect *assert.CollectT) {
		signal, err := a.apiClient.GetSignal(fmt.Sprintf("host:%s @workflow.rule.id:%s", a.Env().Agent.Client.Hostname(), signalRuleID))
		if !assert.NoError(collect, err) {
			return
		}
		if !assert.NotNil(collect, signal) {
			return
		}
		assert.Contains(collect, signal.Tags, fmt.Sprintf("rule_id:%s", strings.ToLower(agentRuleName)), "unable to find rule_id tag")
		agentContext := signal.Attributes["agent"].(map[string]interface{})
		assert.Contains(collect, agentContext["rule_id"], agentRuleName, "unable to find tag")
	}, 5*time.Minute, 20*time.Second)

	// Cleanup
	err = a.apiClient.DeleteSignalRule(signalRuleID)
	assert.NoErrorf(a.T(), err, "failed to delete signal rule %s", signalRuleID)
	err = a.apiClient.DeleteAgentRule(agentRuleID)
	assert.NoErrorf(a.T(), err, "failed to delete agent rule %s", agentRuleID)
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("rm -r %s", dirname))
}

// TestCWSEnabled tests that the detection of CWS is properly working
func (a *agentSuite) Test99CWSEnabled() {
	a.Assert().EventuallyWithTf(func(collect *assert.CollectT) {
		testCwsEnabled(collect, a)
	}, 20*time.Minute, 30*time.Second, "cws activation test timed out for host %s", a.Env().Agent.Client.Hostname())
}

func (a *agentSuite) waitAgentLogs(agentName string, pattern string) error {
	err := backoff.Retry(func() error {
		output, err := a.Env().RemoteHost.Execute(fmt.Sprintf("cat /var/log/datadog/%s.log", agentName))
		if err != nil {
			return err
		}
		if strings.Contains(output, pattern) {
			return nil
		}
		return errors.New("no log found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	return err
}
