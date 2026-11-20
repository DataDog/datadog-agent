// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
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

type recordingTransport struct {
	rt   http.RoundTripper
	mu   sync.Mutex
	errs map[string]error
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.rt.RoundTrip(req)
	r.mu.Lock()
	r.errs[req.URL.Host] = err
	r.mu.Unlock()
	return resp, err
}

func TestForwardingTransport_AdditionalSurvivesClientCancel(t *testing.T) {
	received := make(chan struct{})
	release := make(chan struct{})
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	}))
	t.Cleanup(mainServer.Close)
	var additionalBody string
	additionalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		additionalBody = string(b)
		close(received)
		<-release
		w.Write([]byte("ok"))
	}))
	t.Cleanup(additionalServer.Close)

	mainEndpoint, _ := url.Parse(mainServer.URL)
	additionalEndpoint, _ := url.Parse(additionalServer.URL)
	rt := &recordingTransport{rt: http.DefaultTransport, errs: map[string]error{}}
	ft := newForwardingTransport(rt, mainEndpoint, "test-key", map[string][]string{additionalEndpoint.String(): {"key1"}}, 25*1024*1024)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:8080/debugger/v1/input", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, _ := ft.RoundTrip(req)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	<-received
	cancel()
	close(release)
	<-done

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if err := rt.errs[additionalEndpoint.Host]; err != nil {
		t.Errorf("expected the additional endpoint request to complete after the client went away, got %v", err)
	}
	if additionalBody != "payload" {
		t.Errorf("expected the additional endpoint to receive the payload, got %q", additionalBody)
	}
}

func TestForwardingTransport_AdditionalKeepsDeadline(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	}))
	t.Cleanup(mainServer.Close)
	additionalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
		w.Write([]byte("ok"))
	}))
	t.Cleanup(additionalServer.Close)

	mainEndpoint, _ := url.Parse(mainServer.URL)
	additionalEndpoint, _ := url.Parse(additionalServer.URL)
	rt := &recordingTransport{rt: http.DefaultTransport, errs: map[string]error{}}
	ft := newForwardingTransport(rt, mainEndpoint, "test-key", map[string][]string{additionalEndpoint.String(): {"key1"}}, 25*1024*1024)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:8080/debugger/v1/input", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	start := time.Now()
	resp, _ := ft.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("expected the additional endpoint request to stop at the request deadline, took %s", elapsed)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if err := rt.errs[additionalEndpoint.Host]; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the additional endpoint request to hit the request deadline, got %v", err)
	}
}
