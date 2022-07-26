// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollEndpoint = "/api/v0.1/configurations"
)

// API is the interface to implement for a configuration fetcher
type API interface {
	Fetch(context.Context, *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error)
}

// HTTPClient fetches configurations using HTTP requests
type HTTPClient struct {
	baseURL string
	client  *http.Client
	header  http.Header
}

// NewHTTPClient returns a new HTTP configuration client
func NewHTTPClient(apiKey, appKey string) *HTTPClient {
	header := http.Header{
		"DD-Api-Key":         []string{apiKey},
		"DD-Application-Key": []string{appKey},
		"Content-Type":       []string{"application/x-protobuf"},
	}

	httpClient := &http.Client{
		Transport: httputils.CreateHTTPTransport(),
	}

	baseURL := config.GetMainEndpoint("https://config.", "remote_configuration.rc_dd_url")
	return &HTTPClient{
		client:  httpClient,
		header:  header,
		baseURL: baseURL,
	}
}

// Fetch remote configuration
func (c *HTTPClient) Fetch(ctx context.Context, request *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	body, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}

	url := c.baseURL + pollEndpoint
	log.Debugf("Querying url %s with %+v", url, request)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = c.header

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to issue request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		log.Debugf("Non-200 response. Response body: %s", string(body))
		return nil, fmt.Errorf("non-200 response code: %d", resp.StatusCode)
	}

	body, err = ioutil.ReadAll(resp.Body)
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
