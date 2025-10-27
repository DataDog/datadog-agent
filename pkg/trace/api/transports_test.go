// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// mockTransport is a simple mock that returns a nil response and an error
type mockTransport struct {
	returnNilResponse bool
}

func (m *mockTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	if m.returnNilResponse {
		return nil, errors.New("mock error with nil response")
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

// TestForwardingTransport_NilResponse verifies that RoundTrip doesn't panic
// when the underlying transport returns a nil response with an error.
func TestForwardingTransport_NilResponse(t *testing.T) {
	mockRT := &mockTransport{returnNilResponse: true}
	mainEndpoint, _ := url.Parse("http://localhost:8080")

	ft := newForwardingTransport(mockRT, mainEndpoint, "test-key", nil)

	req, err := http.NewRequest("POST", "http://localhost:8080/v1/traces", strings.NewReader("test body"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// This should not panic even though the underlying transport returns nil response
	resp, err := ft.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		t.Error("expected an error from RoundTrip")
	}

	if resp != nil {
		t.Error("expected nil response")
	}
}

// TestForwardingTransport_MultipleTargetsNilResponse verifies that RoundTrip doesn't panic
// when forwarding to multiple targets and the underlying transport returns a nil response.
func TestForwardingTransport_MultipleTargetsNilResponse(t *testing.T) {
	mockRT := &mockTransport{returnNilResponse: true}
	mainEndpoint, _ := url.Parse("http://localhost:8080")
	additionalEndpoints := map[string][]string{
		"http://localhost:8081": {"key1"},
	}

	ft := newForwardingTransport(mockRT, mainEndpoint, "test-key", additionalEndpoints)

	req, err := http.NewRequest("POST", "http://localhost:8080/v1/traces", strings.NewReader("test body"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// This should not panic even though the underlying transport returns nil response
	resp, err := ft.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		t.Error("expected an error from RoundTrip")
	}

	if resp != nil {
		t.Error("expected nil response")
	}
}
