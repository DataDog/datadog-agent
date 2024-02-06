// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api provides test helpers to interact with the Datadog API
package api

import (
	"context"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

// Client represents the datadog API context
type Client struct {
	api *datadog.APIClient
	ctx context.Context
}

// NewClient initialise a client with the API and APP keys
func NewClient() *Client {
	apiKey, _ := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	appKey, _ := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
			"appKeyAuth": {
				Key: appKey,
			},
		},
	)

	cfg := datadog.NewConfiguration()

	return &Client{
		api: datadog.NewAPIClient(cfg),
		ctx: ctx,
	}
}

// GetAppLog returns the logs corresponding to the query
func (c *Client) GetAppLog(query string) (*datadog.LogsListResponse, error) {
	sort := datadog.LOGSSORT_TIMESTAMP_ASCENDING

	body := datadog.LogsListRequest{
		Filter: &datadog.LogsQueryFilter{
			From:  datadog.PtrString("now-15m"),
			Query: &query,
			To:    datadog.PtrString("now"),
		},
		Page: &datadog.LogsListRequestPage{
			Limit: datadog.PtrInt32(25),
		},
		Sort: &sort,
	}
	request := datadog.ListLogsOptionalParameters{
		Body: &body,
	}

	result, r, err := c.api.LogsApi.ListLogs(c.ctx, request)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetAppSignal returns the signal corresponding to the query
func (c *Client) GetAppSignal(query string) (*datadog.SecurityMonitoringSignalsListResponse, error) {
	now := time.Now().UTC()
	queryFrom := now.Add(-15 * time.Minute)
	sort := datadog.SECURITYMONITORINGSIGNALSSORT_TIMESTAMP_ASCENDING

	body := datadog.SecurityMonitoringSignalListRequest{
		Filter: &datadog.SecurityMonitoringSignalListRequestFilter{
			From:  &queryFrom,
			Query: &query,
			To:    &now,
		},
		Page: &datadog.SecurityMonitoringSignalListRequestPage{
			Limit: datadog.PtrInt32(25),
		},
		Sort: &sort,
	}

	request := datadog.SearchSecurityMonitoringSignalsOptionalParameters{
		Body: &body,
	}

	result, r, err := c.api.SecurityMonitoringApi.SearchSecurityMonitoringSignals(c.ctx, request)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateCwsSignalRule creates a cws signal rule
func (c *Client) CreateCwsSignalRule(name string, msg string, agentRuleID string, tags []string) (*datadog.SecurityMonitoringRuleResponse, error) {
	if tags == nil {
		tags = []string{}
	}
	var (
		detectionMethod    = datadog.SECURITYMONITORINGRULEDETECTIONMETHOD_THRESHOLD
		evaluationWindow   = datadog.SECURITYMONITORINGRULEEVALUATIONWINDOW_ZERO_MINUTES
		keepAlive          = datadog.SECURITYMONITORINGRULEKEEPALIVE_ZERO_MINUTES
		maxSignalDuration  = datadog.SECURITYMONITORINGRULEMAXSIGNALDURATION_ZERO_MINUTES
		aggregation        = datadog.SECURITYMONITORINGRULEQUERYAGGREGATION_COUNT
		monitoringRuleType = datadog.SECURITYMONITORINGRULETYPECREATE_WORKLOAD_SECURITY
	)

	body := datadog.SecurityMonitoringRuleCreatePayload{
		Cases: []datadog.SecurityMonitoringRuleCaseCreate{
			{
				Condition: datadog.PtrString("a > 0"),
				Status:    datadog.SECURITYMONITORINGRULESEVERITY_INFO,
			},
		},

		HasExtendedTitle: datadog.PtrBool(true),
		IsEnabled:        true,
		Name:             name,
		Message:          msg,
		Options: datadog.SecurityMonitoringRuleOptions{
			DetectionMethod:   &detectionMethod,
			EvaluationWindow:  &evaluationWindow,
			KeepAlive:         &keepAlive,
			MaxSignalDuration: &maxSignalDuration,
		},

		Queries: []datadog.SecurityMonitoringRuleQueryCreate{
			{
				Aggregation: &aggregation,
				Query:       "@agent.rule_id:" + agentRuleID,
				Name:        datadog.PtrString("a"),
			},
		},
		Tags: tags,
		Type: &monitoringRuleType,
	}

	response, r, err := c.api.SecurityMonitoringApi.CreateSecurityMonitoringRule(c.ctx, body)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// CreateCWSAgentRule creates a cws agent rule
func (c *Client) CreateCWSAgentRule(name string, msg string, secl string) (*datadog.CloudWorkloadSecurityAgentRuleResponse, error) {
	body := datadog.CloudWorkloadSecurityAgentRuleCreateRequest{
		Data: datadog.CloudWorkloadSecurityAgentRuleCreateData{
			Attributes: datadog.CloudWorkloadSecurityAgentRuleCreateAttributes{
				Description: &msg,
				Enabled:     datadog.PtrBool(true),
				Expression:  secl,
				Name:        name,
			},
			Type: "agent_rule",
		},
	}

	response, r, err := c.api.CloudWorkloadSecurityApi.CreateCloudWorkloadSecurityAgentRule(c.ctx, body)
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
	r, err := c.api.SecurityMonitoringApi.DeleteSecurityMonitoringRule(c.ctx, ruleID)
	if r != nil {
		_ = r.Body.Close()
	}
	return err
}

// DeleteAgentRule deletes an agent rule
func (c *Client) DeleteAgentRule(ruleID string) error {
	r, err := c.api.CloudWorkloadSecurityApi.DeleteCloudWorkloadSecurityAgentRule(c.ctx, ruleID)
	if r != nil {
		_ = r.Body.Close()
	}
	return err
}
