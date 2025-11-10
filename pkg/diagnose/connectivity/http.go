// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivity contains logic for connectivity troubleshooting between the Agent
// and Datadog endpoints. It uses HTTP request to contact different endpoints and displays
// some results depending on endpoints responses, if any.
package connectivity

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func sendHead(ctx context.Context, client *http.Client, url string) (statusCode int, logURL string, err error) {
	status, _, logURL, err := sendRequest(ctx, client, url, http.MethodHead, nil, nil)
	return status, logURL, err
}

func sendGet(ctx context.Context, client *http.Client, url string, headers map[string]string) (statusCode int, body []byte, logURL string, err error) {
	return sendRequest(ctx, client, url, http.MethodGet, nil, headers)
}

func sendPost(ctx context.Context, client *http.Client, url string, payload []byte, headers map[string]string) (statusCode int, body []byte, logURL string, err error) {
	return sendRequest(ctx, client, url, http.MethodPost, payload, headers)
}

// sendRequest sends a generic HTTP Request with the method and payload inside the endpoint information
func sendRequest(ctx context.Context, client *http.Client, url string, method string, payload []byte, headers map[string]string) (statusCode int, body []byte, logURL string, err error) {
	logURL = scrubber.ScrubLine(url)

	var reader io.Reader
	// Create a request for the backend
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)

	if err != nil {
		return 0, nil, logURL, fmt.Errorf("cannot create request for transaction to invalid URL '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}

	if headers != nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if contentType, ok := headers["Content-Type"]; ok && contentType == "" {
			headers["Content-Type"] = "application/json"
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, logURL, fmt.Errorf("cannot send the HTTP request to '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	// Get the endpoint response
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, logURL, fmt.Errorf("fail to read the response Body: %s", scrubber.ScrubLine(err.Error()))
	}

	return resp.StatusCode, body, logURL, nil
}

func withOneRedirect() func(*http.Client) {
	return func(client *http.Client) {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
}

func withTimeout(timeout time.Duration) func(*http.Client) {
	return func(client *http.Client) {
		client.Timeout = timeout
	}
}

func getClient(config config.Component, numberOfWorkers int, log log.Component, clientOptions ...func(*http.Client)) *http.Client {
	transport := forwarder.NewHTTPTransport(config, numberOfWorkers, log)

	client := &http.Client{
		Transport: transport,
	}

	for _, clientOption := range clientOptions {
		clientOption(client)
	}

	return client
}
