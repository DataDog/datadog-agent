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
	apiClient     *api.Client
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
	e2e.Run(t, &agentSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithName("cws-e2e-tests"),
			awshost.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfig),
				agentparams.WithSecurityAgentConfig(securityAgentConfig),
				agentparams.WithSystemProbeConfig(systemProbeConfig),
			),
		),
	))
}

func (a *agentSuite) SetupSuite() {
	a.BaseSuite.SetupSuite()
	a.apiClient = api.NewClient()
}

func (a *agentSuite) TearDownSuite() {
	if len(a.signalRuleID) != 0 {
		a.apiClient.DeleteSignalRule(a.signalRuleID)
	}
	if len(a.agentRuleID) != 0 {
		a.apiClient.DeleteAgentRule(a.agentRuleID)
	}
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("rm -r %s", a.dirname))
	a.BaseSuite.TearDownSuite()
}

func (a *agentSuite) TestOpenSignal() {
	// Create temporary directory
	tempDir := a.Env().RemoteHost.MustExecute("mktemp -d")
	a.dirname = strings.TrimSuffix(tempDir, "\n")
	a.filename = fmt.Sprintf("%s/secret", a.dirname)
	a.testID = uuid.NewString()[:4]
	a.desc = fmt.Sprintf("e2e test rule %s", a.testID)
	a.agentRuleName = fmt.Sprintf("new_e2e_agent_rule_%s", a.testID)

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

	a.EventuallyWithT(func(c *assert.CollectT) {
		policies := a.Env().RemoteHost.MustExecute(fmt.Sprintf("DD_APP_KEY=%s DD_API_KEY=%s %s runtime policy download >| temp.txt && cat temp.txt", appKey, apiKey, securityAgentPath))
		assert.NotEmpty(c, policies, "should not be empty")
		a.policies = policies
	}, 5*time.Minute, 10*time.Second)

	// Check that the newly created rule is in the policies
	assert.Contains(a.T(), a.policies, a.desc, "The policies should contain the created rule")

	// Push policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo cp temp.txt %s", policiesPath))
	a.Env().RemoteHost.MustExecute("rm temp.txt")
	policiesFile := a.Env().RemoteHost.MustExecute(fmt.Sprintf("cat %s", policiesPath))
	assert.Contains(a.T(), policiesFile, a.desc, "The policies file should contain the created rule")

	// Reload policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo %s runtime policy reload", securityAgentPath))

	// Check `downloaded` ruleset_loaded
	result, err := api.WaitAppLogs(a.apiClient, "host:cws-new-e2e-test-host rule_id:ruleset_loaded")
	require.NoError(a.T(), err, "could not get new ruleset")

	agentContext := result.Attributes["agent"].(map[string]interface{})
	assert.EqualValues(a.T(), "ruleset_loaded", agentContext["rule_id"], "Ruleset should be loaded")

	// Trigger agent event
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("touch %s", a.filename))

	// Check agent event
	err = a.waitAgentLogs("security-agent", "Successfully posted payload to")
	require.NoError(a.T(), err, "could not send payload")

	// Check app signal
	signal, err := api.WaitAppSignal(a.apiClient, fmt.Sprintf("host:cws-new-e2e-test-host @workflow.rule.id:%s", a.signalRuleID))
	require.NoError(a.T(), err)
	assert.Contains(a.T(), signal.Tags, fmt.Sprintf("rule_id:%s", strings.ToLower(a.agentRuleName)), "unable to find agent_rule_name tag")
	agentContext = signal.Attributes["agent"].(map[string]interface{})
	assert.Contains(a.T(), agentContext["rule_id"], a.agentRuleName, "unable to find tag")
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
