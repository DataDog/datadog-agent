// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func parseResponse(res *http.Response, method string, URL string) (string, error) {
	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code %d trying to %s %s", res.StatusCode, method, URL)
	}

	defer res.Body.Close()
	all, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error while reading response from %s: %s", URL, err)
	}

	return string(all), nil
}

// Get is a high level helper to query an URL and return its body as a string
func Get(ctx context.Context, URL string, headers map[string]string, timeout time.Duration) (string, error) {
	client := http.Client{
		Transport: CreateHTTPTransport(),
		Timeout:   timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URL, nil)
	if err != nil {
		return "", err
	}

	for header, value := range headers {
		req.Header.Add(header, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	return parseResponse(res, "GET", URL)
}

// Put is a high level helper to query an URL using the PUT method and return its body as a string
func Put(ctx context.Context, URL string, headers map[string]string, body []byte, timeout time.Duration) (string, error) {
	client := http.Client{
		Transport: CreateHTTPTransport(),
		Timeout:   timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, URL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	for header, value := range headers {
		req.Header.Add(header, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	return parseResponse(res, "PUT", URL)
}
