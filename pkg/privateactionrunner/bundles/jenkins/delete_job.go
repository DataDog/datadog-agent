// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
)

type DeleteJobHandler struct {
	httpClientProvider httpclient.Provider
}

func (h *DeleteJobHandler) WithHttpClientProvider(httpClientProvider httpclient.Provider) *DeleteJobHandler {
	h.httpClientProvider = httpClientProvider
	return h
}

func NewDeleteJobHandler() *DeleteJobHandler {
	return &DeleteJobHandler{
		httpClientProvider: httpclient.NewDefaultProvider(),
	}
}

type DeleteJobInputs struct {
	JobName string `json:"jobName,omitempty"`
}

type DeleteJobOutputs struct{}

func (h *DeleteJobHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (interface{}, error) {
	inputs, err := types.ExtractInputs[DeleteJobInputs](task)
	if err != nil {
		return nil, err
	}
	domainAndHeaders, err := getHeadersAndDomain(credential)
	if err != nil {
		return nil, utils.DefaultActionError(fmt.Errorf("error getting headers and API URL: %w", err))
	}
	jobURL := fmt.Sprintf("%s/job/%s/", domainAndHeaders.Domain, encodeJobNameForUrl(inputs.JobName))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, jobURL, nil)
	if err != nil {
		return nil, utils.DefaultActionError(fmt.Errorf("error creating request: %w", err))
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
		return nil, utils.DefaultActionError(fmt.Errorf("error making request: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, utils.DefaultActionError(fmt.Errorf("unexpected status code: %d", resp.StatusCode))
	}
	return &DeleteJobOutputs{}, nil
}
