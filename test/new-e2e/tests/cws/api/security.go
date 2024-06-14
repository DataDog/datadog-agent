// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

func (c *Client) getSignals(query string) (*datadogV2.SecurityMonitoringSignalsListResponse, error) {
	now := time.Now().UTC()
	queryFrom := now.Add(-15 * time.Minute)
	body := datadogV2.SecurityMonitoringSignalListRequest{
		Filter: &datadogV2.SecurityMonitoringSignalListRequestFilter{
			From:  &queryFrom,
			Query: &query,
			To:    &now,
		},
		Page: &datadogV2.SecurityMonitoringSignalListRequestPage{
			Limit: datadog.PtrInt32(25),
		},
		Sort: datadogV2.SECURITYMONITORINGSIGNALSSORT_TIMESTAMP_ASCENDING.Ptr(),
	}

	request := datadogV2.NewSearchSecurityMonitoringSignalsOptionalParameters().WithBody(body)
	api := datadogV2.NewSecurityMonitoringApi(c.api)

	result, r, err := api.SearchSecurityMonitoringSignals(c.ctx, *request)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateCwsSignalRule creates a cws signal rule
func (c *Client) CreateCwsSignalRule(name string, msg string, agentRuleID string, tags []string) (*datadogV2.SecurityMonitoringRuleResponse, error) {
	if tags == nil {
		tags = []string{}
	}
	var (
		detectionMethod    = datadogV2.SECURITYMONITORINGRULEDETECTIONMETHOD_THRESHOLD
		evaluationWindow   = datadogV2.SECURITYMONITORINGRULEEVALUATIONWINDOW_ZERO_MINUTES
		keepAlive          = datadogV2.SECURITYMONITORINGRULEKEEPALIVE_ZERO_MINUTES
		maxSignalDuration  = datadogV2.SECURITYMONITORINGRULEMAXSIGNALDURATION_ZERO_MINUTES
		aggregation        = datadogV2.SECURITYMONITORINGRULEQUERYAGGREGATION_COUNT
		monitoringRuleType = datadogV2.SECURITYMONITORINGRULETYPECREATE_WORKLOAD_SECURITY
	)

	body := datadogV2.SecurityMonitoringRuleCreatePayload{
		SecurityMonitoringStandardRuleCreatePayload: &datadogV2.SecurityMonitoringStandardRuleCreatePayload{
			Cases: []datadogV2.SecurityMonitoringRuleCaseCreate{
				{
					Condition: datadog.PtrString("a > 0"),
					Status:    datadogV2.SECURITYMONITORINGRULESEVERITY_INFO,
				},
			},

			HasExtendedTitle: datadog.PtrBool(true),
			IsEnabled:        true,
			Name:             name,
			Message:          msg,
			Options: datadogV2.SecurityMonitoringRuleOptions{
				DetectionMethod:   &detectionMethod,
				EvaluationWindow:  &evaluationWindow,
				KeepAlive:         &keepAlive,
				MaxSignalDuration: &maxSignalDuration,
			},

			Queries: []datadogV2.SecurityMonitoringStandardRuleQuery{
				{
					Aggregation: &aggregation,
					Query:       datadog.PtrString("@agent.rule_id:" + agentRuleID),
					Name:        datadog.PtrString("a"),
				},
			},
			Tags: tags,
			Type: &monitoringRuleType,
		},
	}

	api := datadogV2.NewSecurityMonitoringApi(c.api)

	response, r, err := api.CreateSecurityMonitoringRule(c.ctx, body)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// CreateCWSAgentRule creates a cws agent rule
func (c *Client) CreateCWSAgentRule(name string, msg string, secl string) (*datadogV2.CloudWorkloadSecurityAgentRuleResponse, error) {
	body := datadogV2.CloudWorkloadSecurityAgentRuleCreateRequest{
		Data: datadogV2.CloudWorkloadSecurityAgentRuleCreateData{
			Attributes: datadogV2.CloudWorkloadSecurityAgentRuleCreateAttributes{
				Description: &msg,
				Enabled:     datadog.PtrBool(true),
				Expression:  secl,
				Name:        name,
			},
			Type: "agent_rule",
		},
	}

	api := datadogV2.NewCSMThreatsApi(c.api)

	response, r, err := api.CreateCSMThreatsAgentRule(c.ctx, body)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// DeleteSignalRule deletes a signal rule
func (c *Client) DeleteSignalRule(ruleID string) error {
	api := datadogV2.NewSecurityMonitoringApi(c.api)
	r, err := api.DeleteSecurityMonitoringRule(c.ctx, ruleID)
	if r != nil {
		_ = r.Body.Close()
	}
	return err
}

// DeleteAgentRule deletes an agent rule
func (c *Client) DeleteAgentRule(ruleID string) error {
	api := datadogV2.NewCSMThreatsApi(c.api)
	r, err := api.DeleteCSMThreatsAgentRule(c.ctx, ruleID)
	if r != nil {
		_ = r.Body.Close()
	}
	return err
}
