// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// StatusCodeError exposes the status code of a failed request
type StatusCodeError struct {
	// StatusCode is the HTTP status code, e.g. 404
	StatusCode int
	// Method is the HTTP method, e.g. GET
	Method string
	// URL is the URL that was queried
	URL string
}

func (e *StatusCodeError) Error() string {
	return fmt.Sprintf("status code %d trying to %s %s", e.StatusCode, e.Method, e.URL)
}

func parseResponse(res *http.Response, method string, URL string) (string, error) {
	if res.StatusCode != 200 {
		return "", &StatusCodeError{StatusCode: res.StatusCode, Method: method, URL: URL}
	}

	defer res.Body.Close()
	all, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error while reading response from %s: %s", URL, err)
	}

	return string(all), nil
}

// Get is a high level helper to query an URL and return its body as a string
func Get(ctx context.Context, URL string, headers map[string]string, timeout time.Duration, cfg pkgconfigmodel.Reader) (string, error) {
	client := http.Client{
		Transport: CreateHTTPTransport(cfg),
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
func Put(ctx context.Context, URL string, headers map[string]string, body []byte, timeout time.Duration, cfg pkgconfigmodel.Reader) (string, error) {
	client := http.Client{
		Transport: CreateHTTPTransport(cfg),
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

// SetJSONError writes a server error as JSON with the correct http error code
func SetJSONError(w http.ResponseWriter, err error, errorCode int) {
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(errorCode)
	fmt.Fprintln(w, string(body))
}
