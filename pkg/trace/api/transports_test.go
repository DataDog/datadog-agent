// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// nilResponseTransport is a simple mock that returns a nil response and an
// error.
type nilResponseTransport struct{}

func (m *nilResponseTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("mock error with nil response")
}

// TestForwardingTransport_NilResponse verifies that RoundTrip doesn't panic
// when the underlying transport returns a nil response with an error.
func TestForwardingTransport_NilResponse(t *testing.T) {
	mainEndpoint, _ := url.Parse("http://localhost:8080")
	ft := newForwardingTransport(&nilResponseTransport{}, mainEndpoint, "test-key", nil, 25*1024*1024)
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
	mainEndpoint, _ := url.Parse("http://localhost:8080")
	additionalEndpoints := map[string][]string{
		"http://localhost:8081": {"key1"},
	}

	ft := newForwardingTransport(&nilResponseTransport{}, mainEndpoint, "test-key", additionalEndpoints, 25*1024*1024)
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

// TestForwardingTransport_MultipleTargets verifies that the transport forwards
// the request to multiple targets correctly. It explicitly ensures that the
// response body is not closed when forwarding to multiple targets. Furthermore,
// it tests explicitly that the transport works when the request body is nil.
func TestForwardingTransport_MultipleTargets(t *testing.T) {
	const responseBody = "ok"
	setupTransport := func(t *testing.T) http.RoundTripper {
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(responseBody))
		})
		s := httptest.NewServer(h)
		t.Cleanup(s.Close)
		s2 := httptest.NewServer(h)
		t.Cleanup(s2.Close)
		mainEndpoint, _ := url.Parse(s.URL)
		additionalEndpoint, _ := url.Parse(s2.URL)
		additionalEndpoints := map[string][]string{
			additionalEndpoint.String(): {"key1"},
		}
		return newForwardingTransport(http.DefaultTransport, mainEndpoint, "test-key", additionalEndpoints, 25*1024*1024)
	}
	validateResponse := func(t *testing.T, rt http.RoundTripper, req *http.Request) {
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Errorf("failed to round trip: %v", err)
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		read, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}
		if read := string(read); read != responseBody {
			t.Fatalf("expected response body to be %q, got %q", responseBody, read)
		}
	}
	newRequest := func(t *testing.T, method, url string, body io.Reader) *http.Request {
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		return req
	}
	for _, c := range []struct {
		name string
		req  *http.Request
	}{
		{
			name: "nil request body",
			req:  newRequest(t, "POST", "http://localhost:8080/v1/traces", nil /* body */),
		},
		{
			name: "non-nil request body",
			req:  newRequest(t, "POST", "http://localhost:8080/v1/traces", strings.NewReader("request body")),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			rt := setupTransport(t)
			validateResponse(t, rt, c.req)
		})
	}
}

// capturingRoundTripper records the DD-API-KEY header of every request it sees. The forwarding
// transport dispatches to additional targets on separate goroutines concurrently with the main
// target, so accesses must be synchronized.
type capturingRoundTripper struct {
	mu   sync.Mutex
	keys []string
}

func (c *capturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.keys = append(c.keys, req.Header.Get("DD-API-KEY"))
	c.mu.Unlock()
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// TestForwardingTransport_SkipsPendingDelegatedAuthDirective is a regression test: a still
// unresolved DELA(...) directive value must never be sent as a literal DD-API-KEY - the
// delegatedauth component resolves it asynchronously and writes the real key back into this same
// config slot. Shared by debugger.go and symdb.go, which both build their forwarding transport
// straight from the raw AdditionalEndpoints config map via newForwardingTransport.
func TestForwardingTransport_SkipsPendingDelegatedAuthDirective(t *testing.T) {
	mainEndpoint, _ := url.Parse("http://localhost:8080")
	additionalEndpoints := map[string][]string{
		"http://localhost:8081": {"DELA(some-org-uuid, aws)", "real-key"},
	}
	crt := &capturingRoundTripper{}
	ft := newForwardingTransport(crt, mainEndpoint, "main-key", additionalEndpoints, 25*1024*1024)

	req, err := http.NewRequest("POST", "http://localhost:8080/v1/traces", strings.NewReader("test body"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := ft.RoundTrip(req)
	if err != nil {
		t.Fatalf("failed to round trip: %v", err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	if len(crt.keys) != 2 {
		t.Fatalf("expected 2 requests (main + one real key, pending directive skipped), got %d: %v", len(crt.keys), crt.keys)
	}
	for _, key := range crt.keys {
		if key == "DELA(some-org-uuid, aws)" {
			t.Errorf("pending directive must not be sent as a literal DD-API-KEY, got keys: %v", crt.keys)
		}
	}
}
