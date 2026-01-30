// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type GetJobStatusHandler struct {
	httpClientProvider httpclient.Provider
}

func (h *GetJobStatusHandler) WithHttpClientProvider(httpClientProvider httpclient.Provider) *GetJobStatusHandler {
	h.httpClientProvider = httpClientProvider
	return h
}

func NewGetJobStatusHandler(runnerConfig *config.Config) *GetJobStatusHandler {
	return &GetJobStatusHandler{
		httpClientProvider: httpclient.NewDefaultProvider(runnerConfig),
	}
}

type GetJobStatusInputs struct {
	JobName string `json:"jobName,omitempty"`
}

type GetJobStatusOutputs struct {
	DisplayName       string `json:"displayName,omitempty"`
	Duration          int    `json:"duration,omitempty"`
	EstimatedDuration int    `json:"estimatedDuration,omitempty"`
	ID                string `json:"id,omitempty"`
	InProgress        bool   `json:"inProgress,omitempty"`
	Result            string `json:"result,omitempty"`
	URL               string `json:"url,omitempty"`
}

func (h *GetJobStatusHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[GetJobStatusInputs](task)
	if err != nil {
		return nil, util.DefaultActionError(err)
	}
	domainAndHeaders, err := getHeadersAndDomain(credential)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("error getting headers and API URL: %w", err))
	}
	jobURL := fmt.Sprintf("%s/job/%s/lastBuild/api/json", domainAndHeaders.Domain, encodeJobNameForUrl(inputs.JobName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jobURL, nil)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("error creating request: %w", err))
	}

	for k, v := range domainAndHeaders.Headers {
		req.Header.Set(k, v[0])
	}
	client, err := h.httpClientProvider.NewDefaultClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("error making request: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, util.DefaultActionError(fmt.Errorf("unexpected status code: %d", resp.StatusCode))
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to read response body: %+v", err))
	}
	parsedRespBody := map[string]interface{}{}
	if err := json.Unmarshal(respBody, &parsedRespBody); err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("error unmarshalling response body: %w", err))
	}
	return &GetJobStatusOutputs{
		DisplayName:       parsedRespBody["displayName"].(string),
		Duration:          int(parsedRespBody["duration"].(float64)),
		EstimatedDuration: int(parsedRespBody["estimatedDuration"].(float64)),
		ID:                parsedRespBody["id"].(string),
		InProgress:        parsedRespBody["building"].(bool),
		Result:            parsedRespBody["result"].(string),
		URL:               parsedRespBody["url"].(string),
	}, nil
}
