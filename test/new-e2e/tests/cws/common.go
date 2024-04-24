// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/xeipuuv/gojsonschema"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/api"
)

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
	query := fmt.Sprintf("rule_id:self_test host:%s", ts.Hostname())
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

	fs := os.DirFS("../../../../pkg/security/tests")
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
