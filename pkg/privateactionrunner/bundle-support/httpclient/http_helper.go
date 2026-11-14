// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"errors"
	"fmt"
	"net/http"

	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
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
	runnerConfig            *config.Config
	enforceURLAllowlist     bool
	enforceDomainAllowlist  bool
	sameOriginRedirectsOnly bool
}

// ProviderOption configures optional behavior on the default HTTP client provider.
type ProviderOption func(*defaultHTTPClientProvider)

// WithURLAllowlistDisabled disables URL allowlist enforcement on all clients created by this provider.
func WithURLAllowlistDisabled() ProviderOption {
	return func(p *defaultHTTPClientProvider) {
		p.enforceURLAllowlist = false
	}
}

// WithDomainAllowlist makes all clients created by this provider refuse
// requests, redirects included, to a host outside private_action_runner.domain_allowlist.
func WithDomainAllowlist() ProviderOption {
	return func(p *defaultHTTPClientProvider) {
		p.enforceDomainAllowlist = true
	}
}

// WithSameOriginRedirectsOnly makes all clients created by this provider refuse
// redirects to another origin, see RunnerHttpClientConfig.SameOriginRedirectsOnly.
func WithSameOriginRedirectsOnly() ProviderOption {
	return func(p *defaultHTTPClientProvider) {
		p.sameOriginRedirectsOnly = true
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

func (d defaultHTTPClientProvider) wrapClient(client *http.Client) HTTPClient {
	if d.enforceDomainAllowlist {
		client.Transport = &domainAllowlistTransport{inner: client.Transport, config: d.runnerConfig}
	}
	if d.enforceURLAllowlist {
		return &urlAllowlistClient{inner: client, config: d.runnerConfig}
	}
	return client
}

func (d defaultHTTPClientProvider) NewDefaultClient() (HTTPClient, error) {
	client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{
		HTTPTimeout:             d.runnerConfig.HTTPTimeout,
		SameOriginRedirectsOnly: d.sameOriginRedirectsOnly,
	})
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return d.wrapClient(client), nil
}

func (d defaultHTTPClientProvider) NewClient(clientConfig *RunnerHttpClientConfig) (HTTPClient, error) {
	if d.sameOriginRedirectsOnly && !clientConfig.SameOriginRedirectsOnly {
		withRedirectPolicy := *clientConfig
		withRedirectPolicy.SameOriginRedirectsOnly = true
		clientConfig = &withRedirectPolicy
	}
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

// domainAllowlistTransport checks every request, including each redirect, against
// the domain allowlist.
type domainAllowlistTransport struct {
	inner  http.RoundTripper
	config *config.Config
}

func (t *domainAllowlistTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if host := req.URL.Hostname(); !t.config.IsDomainAllowed(host) {
		// http.RoundTripper must close the request body, even on error.
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("domain %q is not allowed by %s", host, par.DomainAllowlist)
	}
	return t.inner.RoundTrip(req)
}
