// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type BuildJobHandler struct {
	httpClientProvider httpclient.Provider
}

func (h *BuildJobHandler) WithHttpClientProvider(httpClientProvider httpclient.Provider) *BuildJobHandler {
	h.httpClientProvider = httpClientProvider
	return h
}

func NewBuildJobHandler(runnerConfig *config.Config) *BuildJobHandler {
	return &BuildJobHandler{
		httpClientProvider: httpclient.NewDefaultProvider(runnerConfig),
	}
}

type BuildJobInputs struct {
	JobName             string               `json:"jobName,omitempty"`
	BuildWithParameters *BuildWithParameters `json:"buildWithParameters,omitempty"`
}

type BuildWithParametersOption = string

const (
	NoParameters   BuildWithParametersOption = "no_parameters"
	WithParameters BuildWithParametersOption = "with_parameters"
)

type BuildWithParameters struct {
	Option     BuildWithParametersOption `json:"option,omitempty"`
	Parameters map[string]string         `json:"parameters,omitempty"`
}

type BuildJobOutputs struct{}

func (h *BuildJobHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[BuildJobInputs](task)
	if err != nil {
		return nil, err
	}
	domainAndHeaders, err := getHeadersAndDomain(credential)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("error getting headers and API URL: %w", err))
	}
	jobURL, err := buildJobURL(domainAndHeaders.Domain, inputs)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("error building job URL: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, jobURL.String(), nil)
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
	return &BuildJobOutputs{}, nil
}

func buildJobURL(domain string, inputs BuildJobInputs) (*url.URL, error) {
	params := url.Values{"delay": []string{"0sec"}}
	var path string
	if inputs.BuildWithParameters != nil && inputs.BuildWithParameters.Option == WithParameters {
		path = fmt.Sprintf("/job/%s/buildWithParameters", encodeJobNameForUrl(inputs.JobName))
		for k, v := range inputs.BuildWithParameters.Parameters {
			params.Add(k, v)
		}
	} else {
		path = fmt.Sprintf("/job/%s/build", encodeJobNameForUrl(inputs.JobName))
	}
	jobURL, err := url.Parse(domain + path)
	if err != nil {
		return nil, err
	}
	jobURL.RawQuery = params.Encode()
	return jobURL, nil
}
