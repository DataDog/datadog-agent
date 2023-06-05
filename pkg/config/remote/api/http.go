// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollEndpoint    = "/api/v0.1/configurations"
	orgDataEndpoint = "/api/v0.1/org"
)

var (
	// errUnauthorized is the error that will be logged for the customer to see in case of a 401. We make it as
	// descriptive as possible (while not leaking data) to make RC onboarding easier
	errUnauthorized = fmt.Errorf("unauthorized. Please make sure your API key is valid and has the Remote Config scope")
)

// API is the interface to implement for a configuration fetcher
type API interface {
	Fetch(context.Context, *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error)
	FetchOrgData(context.Context) (*pbgo.OrgDataResponse, error)
}

type Auth struct {
	ApiKey    string
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
func NewHTTPClient(auth Auth) (*HTTPClient, error) {
	header := http.Header{
		"Content-Type": []string{"application/x-protobuf"},
		"DD-Api-Key":   []string{auth.ApiKey},
	}
	if auth.UseAppKey {
		header["DD-Application-Key"] = []string{auth.AppKey}
	}
	transport := httputils.CreateHTTPTransport()
	// Set the keep-alive timeout to 30s instead of the default 90s, so the http RC client is not closed by the backend
	transport.IdleConnTimeout = 30 * time.Second

	httpClient := &http.Client{
		Transport: transport,
	}
	baseRawURL := utils.GetMainEndpoint(config.Datadog, "https://config.", "remote_configuration.rc_dd_url")
	baseURL, err := url.Parse(baseRawURL)
	if err != nil {
		return nil, err
	}
	if baseURL.Scheme != "https" && !config.Datadog.GetBool("remote_configuration.no_tls") {
		return nil, fmt.Errorf("Remote Configuration URL %s is invalid as TLS is required by default. While it is not advised, the `remote_configuration.no_tls` config option can be set to `true` to disable this protection.", baseRawURL)
	}
	if transport.TLSClientConfig.InsecureSkipVerify && !config.Datadog.GetBool("remote_configuration.no_tls_validation") {
		return nil, fmt.Errorf("Remote Configuration does not allow skipping TLS validation by default (currently skipped because `skip_ssl_validation` is set to true). While it is not advised, the `remote_configuration.no_tls_validation` config option can be set to `true` to disable this protection.")
	}
	return &HTTPClient{
		client:  httpClient,
		header:  header,
		baseURL: baseRawURL,
	}, nil
}

// Fetch remote configuration
func (c *HTTPClient) Fetch(ctx context.Context, request *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	body, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}

	url := c.baseURL + pollEndpoint
	log.Debugf("Querying url %s with %+v", url, request)
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

	// Specific case: authentication method is wrong
	// we want to be descriptive about what can be done
	// to fix this as the error is pretty common
	if resp.StatusCode == 401 {
		return nil, errUnauthorized
	}

	// Any other error will have a generic message
	if resp.StatusCode != 200 {
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		log.Debugf("Got a %d response code. Response body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("non-200 response code: %d", resp.StatusCode)
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
	log.Debugf("Querying url %s", url)
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

	var body []byte
	// Specific case: authentication method is wrong
	// we want to be descriptive about what can be done
	// to fix this as the error is pretty common
	if resp.StatusCode == 401 {
		return nil, errUnauthorized
	}

	// Any other error will have a generic message
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("non-200 response code: %d", resp.StatusCode)
	}

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
