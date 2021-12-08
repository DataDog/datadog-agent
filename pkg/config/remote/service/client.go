// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/tinylib/msgp/msgp"
)

// Client is the interface to implement for a configuration fetcher
type Client interface {
	Fetch(context.Context, *pbgo.ClientLatestConfigsRequest) (*pbgo.LatestConfigsResponse, error)
}

// HTTPClient fetches configurations using HTTP requests
type HTTPClient struct {
	baseURL  string
	client   *http.Client
	header   http.Header
	hostname string
}

// NewHTTPClient returns a new HTTP configuration client
func NewHTTPClient(baseURL, apiKey, appKey, hostname string) *HTTPClient {
	header := http.Header{
		"DD-Api-Key":         []string{apiKey},
		"DD-Application-Key": []string{appKey},
		"Content-Type":       []string{"application/msgpack"},
	}

	httpClient := &http.Client{
		Transport: httputils.CreateHTTPTransport(),
	}

	return &HTTPClient{
		client:   httpClient,
		header:   header,
		baseURL:  baseURL,
		hostname: hostname,
	}
}

// Fetch remote configuration
func (c *HTTPClient) Fetch(ctx context.Context, request *pbgo.ClientLatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	body, err := request.MarshalMsg([]byte{})
	if err != nil {
		return nil, err
	}

	url := c.baseURL + "/configurations"
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

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	response := &pbgo.LatestConfigsResponse{}
	err = msgp.Decode(bytes.NewBuffer(body), response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, err
}
