// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"errors"
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
	runnerConfig        *config.Config
	enforceURLAllowlist bool
}

// ProviderOption configures optional behavior on the default HTTP client provider.
type ProviderOption func(*defaultHTTPClientProvider)

// WithURLAllowlistDisabled disables URL allowlist enforcement on all clients created by this provider.
func WithURLAllowlistDisabled() ProviderOption {
	return func(p *defaultHTTPClientProvider) {
		p.enforceURLAllowlist = false
	}
}

func NewDefaultProvider(runnerConfig *config.Config, opts ...ProviderOption) Provider {
	p := &defaultHTTPClientProvider{
		runnerConfig:        runnerConfig,
		enforceURLAllowlist: true,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (d defaultHTTPClientProvider) wrapClient(client HTTPClient) HTTPClient {
	if d.enforceURLAllowlist {
		return &urlAllowlistClient{inner: client, config: d.runnerConfig}
	}
	return client
}

func (d defaultHTTPClientProvider) NewDefaultClient() (HTTPClient, error) {
	client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{
		HTTPTimeout: d.runnerConfig.HTTPTimeout,
	})
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return d.wrapClient(client), nil
}

func (d defaultHTTPClientProvider) NewClient(clientConfig *RunnerHttpClientConfig) (HTTPClient, error) {
	client, err := NewRunnerHttpClient(clientConfig)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return d.wrapClient(client), nil
}

// urlAllowlistClient wraps an HTTPClient to enforce the URL allowlist before making requests.
type urlAllowlistClient struct {
	inner  HTTPClient
	config *config.Config
}

func (c *urlAllowlistClient) Do(req *http.Request) (*http.Response, error) {
	if !c.config.IsURLInAllowlist(req.URL.String()) {
		return nil, util.DefaultActionError(errors.New("request url is not allowed by runner policy: check your configuration file"))
	}
	return c.inner.Do(req)
}
