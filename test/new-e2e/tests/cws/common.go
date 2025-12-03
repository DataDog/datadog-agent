// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	_ "embed"

	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/api"
)

const (
	// systemProbePath is the path of the system-probe binary
	systemProbePath = "/opt/datadog-agent/embedded/bin/system-probe"

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
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, a, "file", "default.policy")
	}, 4*time.Minute, 10*time.Second)
}

func (a *agentSuite) Test01RulesetLoadedDefaultRC() {
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, a, "remote-config", "threat-detection.policy")
	}, 4*time.Minute, 10*time.Second)
}

func (a *agentSuite) Test02Selftests() {
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testSelftestsEvent(c, a, func(event *api.SelftestsEvent) {
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_open", "missing selftest result")
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_chmod", "missing selftest result")
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_chown", "missing selftest result")
			validateEventSchema(c, &event.Event, "self_test_schema.json")
		})
	}, 4*time.Minute, 10*time.Second)
}

func (a *agentSuite) Test03OpenSignal() {
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
			a.Env().RemoteHost.MustExecute("rm -r " + dirname)
		}
	}()

	// Create temporary directory
	tempDir := a.Env().RemoteHost.MustExecute("mktemp -d")
	dirname = strings.TrimSuffix(tempDir, "\n")
	filepath := dirname + "/secret"
	desc := "e2e test rule " + a.testID
	agentRuleName := "new_e2e_agent_rule_" + a.testID

	// Create CWS Agent rule
	rule := fmt.Sprintf("open.file.path == \"%s\"", filepath)
	res, err := a.apiClient.CreateCWSAgentRule(agentRuleName, desc, rule, []string{`os == "linux"`})
	require.NoError(a.T(), err, "Agent rule creation failed")
	agentRuleID = res.Data.GetId()

	// Create Signal Rule (backend)
	res2, err := a.apiClient.CreateCwsSignalRule(desc, "signal rule for e2e testing", agentRuleName, []string{})
	require.NoError(a.T(), err, "Signal rule creation failed")
	signalRuleID = *res2.SecurityMonitoringStandardRuleResponse.Id

	// Check if the agent is ready
	isReady := a.Env().Agent.Client.IsReady()
	assert.Equal(a.T(), isReady, true, "Agent should be ready")

	// Download policies
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(a.T(), err, "Could not get API KEY")

	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(a.T(), err, "Could not get APP KEY")

	var policies string
	require.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		policies = a.Env().RemoteHost.MustExecute(fmt.Sprintf("DD_APP_KEY=%s DD_API_KEY=%s %s runtime policy download >| temp.txt && cat temp.txt", appKey, apiKey, systemProbePath))
		assert.NotEmpty(c, policies, "should not be empty")
	}, 1*time.Minute, 1*time.Second)

	// Check that the newly created rule is in the policies
	require.Contains(a.T(), policies, desc, "The policies should contain the created rule")

	// Push policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo cp temp.txt %s && rm temp.txt", policiesPath))
	policiesFile := a.Env().RemoteHost.MustExecute("cat " + policiesPath)
	require.Contains(a.T(), policiesFile, desc, "The policies file should contain the created rule")

	// Reload policies
	a.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo %s runtime policy reload", systemProbePath))

	// Check if the policy is loaded
	policyName := path.Base(policiesPath)
	require.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, a, "file", policyName)
	}, 4*time.Minute, 5*time.Second)

	// Check 'datadog.security_agent.runtime.running' metric
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		testMetricExists(c, a, "datadog.security_agent.runtime.running", map[string]string{"host": a.Hostname()})
	}, 4*time.Minute, 5*time.Second)

	// Check app event
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		// Trigger agent event
		a.Env().RemoteHost.MustExecute("touch " + filepath)
		testRuleEvent(c, a, agentRuleName, func(e *api.RuleEvent) {
			assert.Equal(c, "open", e.Evt.Name, "event name should be open")
			assert.Equal(c, filepath, e.File.Path, "file path does not match")
			assert.Contains(c, e.Tags, "tag1", "missing event tag")
			assert.Contains(c, e.Tags, "tag2", "missing event tag")
		})
	}, 10*time.Minute, 30*time.Second)

	// Check app signal
	assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		signal, err := a.apiClient.GetSignal(fmt.Sprintf("host:%s @workflow.rule.id:%s", a.Env().Agent.Client.Hostname(), signalRuleID))
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotNil(c, signal) {
			return
		}
		assert.Contains(c, signal.Tags, "rule_id:"+strings.ToLower(agentRuleName), "unable to find rule_id tag")
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

func (a *agentSuite) Test04SecurityAgentSIGTERM() {
	output := a.Env().RemoteHost.MustExecute("cat /opt/datadog-agent/run/security-agent.pid")
	pid, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	require.NoError(a.T(), err, "failed to parse security-agent pid")

	var start, end time.Time
	start = time.Now()
	_, err = a.Env().RemoteHost.Execute(fmt.Sprintf("sudo kill -SIGTERM %d", pid))
	require.NoError(a.T(), err, "failed to send SIGTERM to security-agent")
	exited := assert.EventuallyWithT(a.T(), func(c *assert.CollectT) {
		_, err := a.Env().RemoteHost.Execute("pgrep -x security-agent")
		if err == nil {
			c.Errorf("security-agent should not be running")
			return
		}

		// pgrep exits with 1 if no process is found
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitStatus() == 1 {
			end = time.Now()
			return
		}
		assert.NoError(c, err, "failed to check the security-agent process state")
	}, 30*time.Second, 1*time.Second)

	if exited {
		a.T().Logf("security-agent exited after %s", end.Sub(start).String())
	}

	// make sure the security-agent is running after this test
	a.Env().RemoteHost.MustExecute("sudo systemctl start datadog-agent-security.service")
}

// test that the detection of CWS is properly working
// this test can be quite long so run it last
func (a *agentSuite) Test99CWSEnabled() {
	assert.EventuallyWithTf(a.T(), func(c *assert.CollectT) {
		testCwsEnabled(c, a)
	}, 20*time.Minute, 30*time.Second, "cws activation test timed out for host %s", a.Env().Agent.Client.Hostname())
}

type testSuite interface {
	Hostname() string
	Client() *api.Client
}

type eventValidationCb[T any] func(e T)

func testRulesetLoaded(t assert.TestingT, ts testSuite, policySource string, policyName string, extraValidations ...eventValidationCb[*api.RulesetLoadedEvent]) {
	query := fmt.Sprintf("rule_id:ruleset_loaded host:%s @policies.source:%s @policies.name:%s", ts.Hostname(), policySource, policyName)
	rulesetLoaded, err := api.GetAppEvent[api.RulesetLoadedEvent](ts.Client(), query)
	if !assert.NoErrorf(t, err, "could not get %s/%s ruleset_loaded event for host %s", policySource, policyName, ts.Hostname()) {
		return
	}
	if !assert.NotNil(t, rulesetLoaded, "ruleset_loaded should not be nil") {
		return
	}
	assert.Equalf(t, "ruleset_loaded", rulesetLoaded.Agent.RuleID, "found unexpected rule_id in ruleset_loaded event")
	assert.Truef(t, rulesetLoaded.ContainsPolicy(policySource, policyName), "host %s should have policy %s/%s loaded", ts.Hostname(), policySource, policyName)
	for _, extraValidation := range extraValidations {
		extraValidation(rulesetLoaded)
	}
}

func testRuleEvent(t assert.TestingT, ts testSuite, ruleID string, extraValidations ...eventValidationCb[*api.RuleEvent]) {
	query := fmt.Sprintf("rule_id:%s host:%s", ruleID, ts.Hostname())
	ruleEvent, err := api.GetAppEvent[api.RuleEvent](ts.Client(), query)
	if !assert.NoErrorf(t, err, "could not get %s event for host %s", ruleID, ts.Hostname()) {
		return
	}
	if !assert.NotNil(t, ruleEvent, "rule event should not be nil") {
		return
	}
	assert.Equalf(t, ruleID, ruleEvent.Agent.RuleID, "found unexpected rule_id in event")
	for _, extraValidation := range extraValidations {
		extraValidation(ruleEvent)
	}
}

func testCwsEnabled(t assert.TestingT, ts testSuite) {
	query := fmt.Sprintf("SELECT h.hostname, a.feature_cws_enabled FROM host h JOIN datadog_agent a USING (datadog_agent_key) WHERE h.hostname = '%s'", ts.Hostname())
	resp, err := ts.Client().TableQuery(query)
	if !assert.NoErrorf(t, err, "ddsql query failed") {
		return
	}
	if !assert.Len(t, resp.Data, 1, "ddsql query didn't returned a single row") {
		return
	}
	if !assert.Len(t, resp.Data[0].Attributes.Columns, 2, "ddsql query didn't returned two columns") {
		return
	}

	columnChecks := []struct {
		name          string
		expectedValue interface{}
	}{
		{
			name:          "hostname",
			expectedValue: ts.Hostname(),
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
				if !assert.Len(t, column.Values, 1, "column %s should have a single value", columnCheck.name) {
					return
				}
				if !assert.Equal(t, columnCheck.expectedValue, column.Values[0], "column %s should be equal", columnCheck.name) {
					return
				}
				result = true
				break
			}
		}
		if !assert.Truef(t, result, "column %s isn't present or has an unexpected value", columnCheck.name) {
			return
		}
	}
}

func testSelftestsEvent(t assert.TestingT, ts testSuite, extraValidations ...eventValidationCb[*api.SelftestsEvent]) {
	query := "rule_id:self_test host:" + ts.Hostname()
	selftestsEvent, err := api.GetAppEvent[api.SelftestsEvent](ts.Client(), query)
	if !assert.NoErrorf(t, err, "could not get selftests event for host %s", ts.Hostname()) {
		return
	}
	if !assert.NotNil(t, selftestsEvent, "selftests event should not be nil") {
		return
	}
	if !assert.Equalf(t, "self_test", selftestsEvent.Agent.RuleID, "found unexpected rule_id in selftests event") {
		return
	}
	assert.Empty(t, selftestsEvent.FailedTests, "selftests should not have failed tests")
	for _, extraValidation := range extraValidations {
		extraValidation(selftestsEvent)
	}
}

func validateEventSchema(t assert.TestingT, e *api.Event, schemaFileName string) {
	b, err := e.MarshalJSON()
	if !assert.NoError(t, err) {
		return
	}

	fs := os.DirFS("../../../../pkg/security/secl")
	documentLoader := gojsonschema.NewBytesLoader(b)
	schemaLoader := gojsonschema.NewReferenceLoaderFileSystem("file:///schemas/"+schemaFileName, http.FS(fs))
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.True(t, result.Valid(), "schema validation failed") {
		for _, err := range result.Errors() {
			t.Errorf("%s", err)
		}
	}
}

func testMetricExists(t assert.TestingT, ts testSuite, metricName string, metricTags ...map[string]string) {
	var tags []string
	for _, tag := range metricTags {
		for k, v := range tag {
			tags = append(tags, fmt.Sprintf("%s:%s", k, v))
		}
	}
	if len(tags) == 0 {
		tags = append(tags, "*")
	}
	query := fmt.Sprintf("%s{%s}", metricName, strings.Join(tags, ","))
	resp, err := ts.Client().QueryMetric(query)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, resp, "query returned a nil response") {
		return
	}
	assert.Greater(t, len(resp.Series), 0, "metric query returned empty series")
}
