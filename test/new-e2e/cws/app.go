// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"
)

type ApiClient interface {
	GetAppSignal(string) (*datadog.SecurityMonitoringSignalsListResponse, error)
	CreateCWSSignalRule(string, string, string, []string) (*datadog.SecurityMonitoringRuleResponse, error)
	CreateCWSAgentRule(string, string, string) (*datadog.CloudWorkloadSecurityAgentRuleResponse, error)
	DeleteSignalRule(string) error
	DeleteAgentRule(string) error
	DownloadPolicies() (string, error)
}

type apiClient struct {
	*datadog.APIClient
	ctx context.Context
}

func NewApiClient() apiClient {
	apiKey, _ := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	appKey, _ := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	fmt.Println("api_key:", apiKey, "app_key:", appKey)
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

	apiClient := apiClient{
		APIClient: datadog.NewAPIClient(cfg),
		ctx:       ctx,
	}
	return apiClient
}

func (c apiClient) GetAppSignal(query string) (*datadog.SecurityMonitoringSignalsListResponse, error) {
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

	result, _, err := c.SecurityMonitoringApi.SearchSecurityMonitoringSignals(c.ctx, request)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (c apiClient) CreateCwsSignalRule(name string, msg string, agentRuleID string, tags []string) (*datadog.SecurityMonitoringRuleResponse, error) {
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

	response, _, err := c.SecurityMonitoringApi.CreateSecurityMonitoringRule(c.ctx, body)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c apiClient) CreateCWSAgentRule(name string, msg string, secl string) (*datadog.CloudWorkloadSecurityAgentRuleResponse, error) {

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

	response, _, err := c.CloudWorkloadSecurityApi.CreateCloudWorkloadSecurityAgentRule(c.ctx, body)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c apiClient) DeleteSignalRule(ruleId string) error {
	_, err := c.SecurityMonitoringApi.DeleteSecurityMonitoringRule(c.ctx, ruleId)
	if err != nil {
		return err
	}
	return nil
}

func (c apiClient) DeleteAgentRule(ruleId string) error {
	_, err := c.CloudWorkloadSecurityApi.DeleteCloudWorkloadSecurityAgentRule(c.ctx, ruleId)
	if err != nil {
		return err
	}
	return nil
}

// func (c apiClient) DownloadPolicies() (string, error) {

// }
