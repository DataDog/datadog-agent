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
	"testing/synctest"
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

// blockingAdditionalTransport answers the main endpoint at once. For any other
// endpoint it reads the body, then blocks until release is closed or the
// request context ends.
type blockingAdditionalTransport struct {
	mainHost string
	release  chan struct{}

	mu       sync.Mutex
	reached  bool
	finished bool
	body     string
	err      error
}

func (f *blockingAdditionalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == f.mainHost {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	}
	b, _ := io.ReadAll(req.Body)
	f.mu.Lock()
	f.reached = true
	f.body = string(b)
	f.mu.Unlock()

	var err error
	select {
	case <-f.release:
	case <-req.Context().Done():
		err = req.Context().Err()
	}
	f.mu.Lock()
	f.finished = true
	f.err = err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

// newBlockingForwardingTransport returns a forwarding transport with one
// additional endpoint served by a blockingAdditionalTransport, and a function
// that releases the additional request. Cleanup releases it too.
func newBlockingForwardingTransport(t *testing.T) (*forwardingTransport, *blockingAdditionalTransport, func()) {
	mainEndpoint, _ := url.Parse("https://main.example")
	fake := &blockingAdditionalTransport{mainHost: mainEndpoint.Host, release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(fake.release) }) }
	t.Cleanup(release)
	ft := newForwardingTransport(fake, mainEndpoint, "test-key", map[string][]string{"https://additional.example": {"key1"}}, 25*1024*1024)
	return ft, fake, release
}

func TestForwardingTransport_AdditionalSurvivesClientCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ft, fake, release := newBlockingForwardingTransport(t)
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

		synctest.Wait()
		fake.mu.Lock()
		reached := fake.reached
		fake.mu.Unlock()
		if !reached {
			t.Fatal("the additional endpoint was not reached")
		}

		cancel()
		synctest.Wait()
		fake.mu.Lock()
		finished := fake.finished
		fake.mu.Unlock()
		if finished {
			t.Fatal("the additional endpoint request ended when the client went away")
		}

		release()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("RoundTrip did not return after the additional endpoint answered")
		}
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if fake.err != nil {
			t.Errorf("expected the additional endpoint request to complete after the client went away, got %v", fake.err)
		}
		if fake.body != "payload" {
			t.Errorf("expected the additional endpoint to receive the payload, got %q", fake.body)
		}
	})
}

func TestForwardingTransport_AdditionalKeepsDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ft, fake, _ := newBlockingForwardingTransport(t)
		const timeout = 200 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		if elapsed := time.Since(start); elapsed != timeout {
			t.Errorf("expected the additional endpoint request to stop at the request deadline (%s), took %s", timeout, elapsed)
		}

		fake.mu.Lock()
		defer fake.mu.Unlock()
		if !errors.Is(fake.err, context.DeadlineExceeded) {
			t.Errorf("expected the additional endpoint request to hit the request deadline, got %v", fake.err)
		}
	})
}
