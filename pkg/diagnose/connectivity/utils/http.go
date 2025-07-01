// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils contains logic for connectivity troubleshooting between the Agent
// and Datadog endpoints. It uses HTTP request to contact different endpoints and displays
// some results depending on endpoints responses, if any.
package utils

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

// Method represents the HTTP method to use for the request
type Method string

const (
	// Head is the HTTP HEAD method
	Head Method = "HEAD"
	// Get is the HTTP GET method
	Get Method = "GET"
	// Post is the HTTP POST method
	Post Method = "POST"
)

// SendHead sends an HTTP HEAD request to the given URL
func SendHead(ctx context.Context, client *http.Client, url string) (statusCode int, logURL string, err error) {
	status, _, logURL, err := SendRequest(ctx, client, url, Head, nil, nil)
	return status, logURL, err
}

// SendGet sends an HTTP GET request to the given URL with the given headers
func SendGet(ctx context.Context, client *http.Client, url string, headers map[string]string) (statusCode int, body []byte, logURL string, err error) {
	return SendRequest(ctx, client, url, Get, nil, headers)
}

// SendPost sends an HTTP Request with the method and payload inside the endpoint information
func SendPost(ctx context.Context, client *http.Client, url string, payload []byte, headers map[string]string) (statusCode int, body []byte, logURL string, err error) {
	return SendRequest(ctx, client, url, Post, payload, headers)
}

// SendRequest sends an HTTP request to the given URL with the given method, payload and headers
// It returns the status code, the body, the scrubbed log URL and an error if any
func SendRequest(ctx context.Context, client *http.Client, url string, method Method, payload []byte, headers map[string]string) (statusCode int, body []byte, logURL string, err error) {
	logURL = scrubber.ScrubLine(url)

	var reader io.Reader
	// Create a request for the backend
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, string(method), url, reader)

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

// WithOneRedirect sets the HTTP client to return only the last response when a redirect is encountered
func WithOneRedirect() func(*http.Client) {
	return func(client *http.Client) {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
}

// WithTimeout sets the timeout for the HTTP client
func WithTimeout(timeout time.Duration) func(*http.Client) {
	return func(client *http.Client) {
		client.Timeout = timeout
	}
}

// GetClient creates a new HTTP client with the given configuration and options
func GetClient(config config.Component, numberOfWorkers int, log log.Component, clientOptions ...func(*http.Client)) *http.Client {
	transport := forwarder.NewHTTPTransport(config, numberOfWorkers, log)

	client := &http.Client{
		Transport: transport,
	}

	for _, clientOption := range clientOptions {
		clientOption(client)
	}

	return client
}
