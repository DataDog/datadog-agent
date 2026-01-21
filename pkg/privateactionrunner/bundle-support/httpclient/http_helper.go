// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// HTTPClient is an interface for http.Client that enables mocking
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Provider can be used by action handlers to create HTTP clients
type Provider interface {
	NewDefaultClient() (HTTPClient, error)
	NewClient(runnerConfig *RunnerHttpClientConfig) (HTTPClient, error)
}

type defaultHTTPClientProvider struct {
	runnerConfig *config.Config
}

func NewDefaultProvider(runnerConfig *config.Config) Provider {
	return &defaultHTTPClientProvider{
		runnerConfig: runnerConfig,
	}
}

func (d defaultHTTPClientProvider) NewDefaultClient() (HTTPClient, error) {
	client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{
		HTTPTimeout: d.runnerConfig.HTTPTimeout,
	})
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return client, nil
}

func (d defaultHTTPClientProvider) NewClient(clientConfig *RunnerHttpClientConfig) (HTTPClient, error) {
	client, err := NewRunnerHttpClient(clientConfig)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return client, nil
}
