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
	NewClient(clientConfig *RunnerHttpClientConfig) (HTTPClient, error)
}

type defaultHTTPClientProvider struct{}

func NewDefaultProvider() Provider {
	return &defaultHTTPClientProvider{}
}

func (d defaultHTTPClientProvider) NewDefaultClient() (HTTPClient, error) {
	client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{})
	if err != nil {
		return nil, utils.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return client, nil
}

func (d defaultHTTPClientProvider) NewClient(clientConfig *RunnerHttpClientConfig) (HTTPClient, error) {
	client, err := NewRunnerHttpClient(clientConfig)
	if err != nil {
		return nil, utils.DefaultActionErrorWithDisplayError(fmt.Errorf("error creating HTTP client: %w", err), "Failed to create HTTP client")
	}
	return client, nil
}

type mockHTTPClientProvider struct {
	mock *MockHTTPClient
}

func NewMockProvider(mock *MockHTTPClient) Provider {
	return &mockHTTPClientProvider{
		mock: mock,
	}
}

func (m mockHTTPClientProvider) NewDefaultClient() (HTTPClient, error) {
	return m.mock, nil
}

func (m mockHTTPClientProvider) NewClient(_ *RunnerHttpClientConfig) (HTTPClient, error) {
	return m.mock, nil
}
