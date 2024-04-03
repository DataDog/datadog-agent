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
	testID string
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

func (a *agentSuite) Test00OpenSignal() {
	apiClient := api.NewClient()

	// Create temporary directory
	tempDir := a.Env().RemoteHost.MustExecute("mktemp -d")
	dirname := strings.TrimSuffix(tempDir, "\n")
	filename := fmt.Sprintf("%s/secret", dirname)
	desc := fmt.Sprintf("e2e test rule %s", a.testID)
	agentRuleName := fmt.Sprintf("new_e2e_agent_rule_%s", a.testID)

	// Create CWS Agent rule
	rule := fmt.Sprintf("open.file.path == \"%s\"", filename)
	res, err := apiClient.CreateCWSAgentRule(agentRuleName, desc, rule)
	require.NoError(a.T(), err, "Agent rule creation failed")
	agentRuleID := res.Data.GetId()

	// Create Signal Rule (backend)
	res2, err := apiClient.CreateCwsSignalRule(desc, "signal rule for e2e testing", agentRuleName, []string{})
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
	a.EventuallyWithT(func(c *assert.CollectT) {
		policies = a.Env().RemoteHost.MustExecute(fmt.Sprintf("DD_APP_KEY=%s DD_API_KEY=%s %s runtime policy download >| temp.txt && cat temp.txt", appKey, apiKey, securityAgentPath))
		assert.NotEmpty(c, policies, "should not be empty")
	}, 5*time.Minute, 10*time.Second)

	// Check that the newly created rule is in the policies
	assert.Contains(a.T(), policies, desc, "The policies should contain the created rule")

	// Push policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo cp temp.txt %s", policiesPath))
	a.Env().RemoteHost.MustExecute("rm temp.txt")
	policiesFile := a.Env().RemoteHost.MustExecute(fmt.Sprintf("cat %s", policiesPath))
	assert.Contains(a.T(), policiesFile, desc, "The policies file should contain the created rule")

	// Reload policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo %s runtime policy reload", securityAgentPath))

	// Check `downloaded` ruleset_loaded
	result, err := api.WaitAppLogs(apiClient, fmt.Sprintf("host:%s rule_id:ruleset_loaded", a.Env().Agent.Client.Hostname()))
	require.NoError(a.T(), err, "could not get new ruleset")

	agentContext := result.Attributes["agent"].(map[string]interface{})
	assert.EqualValues(a.T(), "ruleset_loaded", agentContext["rule_id"], "Ruleset should be loaded")

	// Trigger agent event
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("touch %s", filename))

	// Check agent event
	err = a.waitAgentLogs("security-agent", "Successfully posted payload to")
	require.NoError(a.T(), err, "could not send payload")

	// Check app signal
	signal, err := api.WaitAppSignal(apiClient, fmt.Sprintf("host:%s @workflow.rule.id:%s", a.Env().Agent.Client.Hostname(), signalRuleID))
	require.NoError(a.T(), err)
	assert.Contains(a.T(), signal.Tags, fmt.Sprintf("rule_id:%s", strings.ToLower(agentRuleName)), "unable to find rule_id tag")
	agentContext = signal.Attributes["agent"].(map[string]interface{})
	assert.Contains(a.T(), agentContext["rule_id"], agentRuleName, "unable to find tag")

	// Cleanup
	err = apiClient.DeleteSignalRule(signalRuleID)
	assert.NoErrorf(a.T(), err, "failed to delete signal rule %s", signalRuleID)
	err = apiClient.DeleteAgentRule(agentRuleID)
	assert.NoErrorf(a.T(), err, "failed to delete agent rule %s", agentRuleID)
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("rm -r %s", dirname))
}

// TestFeatureCWSEnabled tests that the CWS activation is properly working
func (a *agentSuite) Test01FeatureCWSEnabled() {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	a.Require().NoError(err, "could not get API key")
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	a.Require().NoError(err, "could not get APP key")
	ddSQLClient := api.NewDDSQLClient(apiKey, appKey)

	query := fmt.Sprintf("SELECT h.hostname, a.feature_cws_enabled FROM host h JOIN datadog_agent a USING (datadog_agent_key) WHERE h.hostname = '%s'", a.Env().Agent.Client.Hostname())
	a.Assert().EventuallyWithTf(func(collect *assert.CollectT) {
		resp, err := ddSQLClient.Do(query)
		if !assert.NoErrorf(collect, err, "ddsql query failed") {
			return
		}
		if !assert.Len(collect, resp.Data, 1, "ddsql query didn't returned a single row") {
			return
		}
		if !assert.Len(collect, resp.Data[0].Attributes.Columns, 2, "ddsql query didn't returned two columns") {
			return
		}

		columnChecks := []struct {
			name          string
			expectedValue interface{}
		}{
			{
				name:          "hostname",
				expectedValue: a.Env().Agent.Client.Hostname(),
			},
			{
				name:          "feature_cws_enabled",
				expectedValue: true,
			},
		}

		for _, columnCheck := range columnChecks {
			result := false
			for _, column := range resp.Data[0].Attributes.Columns {
				if column.Name == columnCheck.name {
					if !assert.Len(collect, column.Values, 1, "column %s should have a single value", columnCheck.name) {
						return
					}
					if !assert.Equal(collect, columnCheck.expectedValue, column.Values[0], "column %s should be equal", columnCheck.name) {
						return
					}
					result = true
					break
				}
			}
			if !assert.Truef(collect, result, "column %s isn't present or has an unexpected value", columnCheck.name) {
				return
			}
		}
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
