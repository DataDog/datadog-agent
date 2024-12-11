// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api defines the HTTP interface for the remote config backend
package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollEndpoint      = "/api/v0.1/configurations"
	orgDataEndpoint   = "/api/v0.1/org"
	orgStatusEndpoint = "/api/v0.1/status"
)

var (
	// ErrUnauthorized is the error that will be logged for the customer to see in case of a 401. We make it as
	// descriptive as possible (while not leaking data) to make RC onboarding easier
	ErrUnauthorized = fmt.Errorf("unauthorized. Please make sure your API key is valid and has the Remote Config scope")
	// ErrProxy is the error that will be logged if we suspect that there is a wrong proxy setup for remote-config.
	// It is displayed for any 4XX status code except 401
	ErrProxy = fmt.Errorf(
		"4XX status code. This might be related to the proxy settings. " +
			"Please make sure the agent can reach Remote Configuration with the proxy setup",
	)
)

// API is the interface to implement for a configuration fetcher
type API interface {
	Fetch(context.Context, *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error)
	FetchOrgData(context.Context) (*pbgo.OrgDataResponse, error)
	FetchOrgStatus(context.Context) (*pbgo.OrgStatusResponse, error)
}

// Auth defines the possible Authentication data to access the RC backend
type Auth struct {
	APIKey    string
	AppKey    string
	UseAppKey bool
}

// HTTPClient fetches configurations using HTTP requests
type HTTPClient struct {
	baseURL string
	client  *http.Client
	header  http.Header
}

// NewHTTPClient returns a new HTTP configuration client
func NewHTTPClient(auth Auth, cfg model.Reader, baseURL *url.URL) (*HTTPClient, error) {
	header := http.Header{
		"Content-Type": []string{"application/x-protobuf"},
		"DD-Api-Key":   []string{auth.APIKey},
	}
	if auth.UseAppKey {
		header["DD-Application-Key"] = []string{auth.AppKey}
	}
	transport := httputils.CreateHTTPTransport(cfg)
	// Set the keep-alive timeout to 30s instead of the default 90s, so the http RC client is not closed by the backend
	transport.IdleConnTimeout = 30 * time.Second

	httpClient := &http.Client{
		Transport: transport,
	}
	if baseURL.Scheme != "https" && !cfg.GetBool("remote_configuration.no_tls") {
		return nil, fmt.Errorf("remote Configuration URL %s is invalid as TLS is required by default. While it is not advised, the `remote_configuration.no_tls` config option can be set to `true` to disable this protection", baseURL)
	}
	if transport.TLSClientConfig.InsecureSkipVerify && !cfg.GetBool("remote_configuration.no_tls_validation") {
		return nil, fmt.Errorf("remote Configuration does not allow skipping TLS validation by default (currently skipped because `skip_ssl_validation` is set to true). While it is not advised, the `remote_configuration.no_tls_validation` config option can be set to `true` to disable this protection")
	}
	return &HTTPClient{
		client:  httpClient,
		header:  header,
		baseURL: baseURL.String(),
	}, nil
}

// Fetch remote configuration
func (c *HTTPClient) Fetch(ctx context.Context, request *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	body, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}

	url := c.baseURL + pollEndpoint
	log.Debugf("fetching configurations at %s", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create org data request: %w", err)
	}
	req.Header = c.header

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to issue request: %w", err)
	}
	defer resp.Body.Close()

	// Any other error will have a generic message
	if resp.StatusCode != 200 {
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		log.Debugf("Got a %d response code. Response body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("non-200 response code: %d", resp.StatusCode)
	}

	err = checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	response := &pbgo.LatestConfigsResponse{}
	err = proto.Unmarshal(body, response)
	if err != nil {
		log.Debugf("Error decoding response, %v, response body: %s", err, string(body))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, err
}

// FetchOrgData org data
func (c *HTTPClient) FetchOrgData(ctx context.Context) (*pbgo.OrgDataResponse, error) {
	url := c.baseURL + orgDataEndpoint
	log.Debugf("fetching org data at %s", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, &bytes.Buffer{})
	if err != nil {
		return nil, fmt.Errorf("failed to create org data request: %w", err)
	}
	req.Header = c.header

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to issue org data request: %w", err)
	}
	defer resp.Body.Close()

	err = checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	response := &pbgo.OrgDataResponse{}
	err = proto.Unmarshal(body, response)
	if err != nil {
		log.Debugf("Error decoding response, %v, response body: %s", err, string(body))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, err
}

// FetchOrgStatus returns the org and key status
func (c *HTTPClient) FetchOrgStatus(ctx context.Context) (*pbgo.OrgStatusResponse, error) {
	url := c.baseURL + orgStatusEndpoint
	log.Debugf("fetching org status at %s", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, &bytes.Buffer{})
	if err != nil {
		return nil, fmt.Errorf("failed to create org data request: %w", err)
	}
	req.Header = c.header

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to issue org data request: %w", err)
	}
	defer resp.Body.Close()

	err = checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	response := &pbgo.OrgStatusResponse{}
	err = proto.Unmarshal(body, response)
	if err != nil {
		log.Debugf("Error decoding response, %v, response body: %s", err, string(body))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, err
}

func checkStatusCode(resp *http.Response) error {
	// Specific case: authentication method is wrong
	// we want to be descriptive about what can be done
	// to fix this as the error is pretty common
	if resp.StatusCode == 401 {
		return ErrUnauthorized
	}

	if resp.StatusCode >= 400 && resp.StatusCode <= 499 {
		return fmt.Errorf("%w: %d", ErrProxy, resp.StatusCode)
	}

	// Any other error will have a generic message
	if resp.StatusCode != 200 {
		return fmt.Errorf("non-200 response code: %d", resp.StatusCode)
	}

	return nil
}
