// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package httpclient provides a HTTP client that can be used to make HTTP requests
package httpclient

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
)

// HTTPClient is an interface for http.Client that enables mocking
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Provider can be used by action handlers to create HTTP clients
type Provider interface {
	NewDefaultClient() (HTTPClient, error)
	NewClient(clientConfig *RunnerHTTPClientConfig) (HTTPClient, error)
}

type defaultHTTPClientProvider struct{}

// NewDefaultProvider creates a new default HTTP client provider
func NewDefaultProvider() Provider {
	return &defaultHTTPClientProvider{}
}

func (d defaultHTTPClientProvider) NewDefaultClient() (HTTPClient, error) {
	client, err := NewRunnerHTTPClient(&RunnerHTTPClientConfig{})
	if err != nil {
		return nil, utils.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return client, nil
}

func (d defaultHTTPClientProvider) NewClient(clientConfig *RunnerHTTPClientConfig) (HTTPClient, error) {
	client, err := NewRunnerHTTPClient(clientConfig)
	if err != nil {
		return nil, utils.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return client, nil
}

type mockHTTPClientProvider struct {
	mock *MockHTTPClient
}

// NewMockProvider creates a new mock HTTP client provider
func NewMockProvider(mock *MockHTTPClient) Provider {
	return &mockHTTPClientProvider{
		mock: mock,
	}
}

func (m mockHTTPClientProvider) NewDefaultClient() (HTTPClient, error) {
	return m.mock, nil
}

func (m mockHTTPClientProvider) NewClient(_ *RunnerHTTPClientConfig) (HTTPClient, error) {
	return m.mock, nil
}
