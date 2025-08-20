// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
)

// trackingBody is an io.ReadCloser that tracks whether Close was called.
type trackingBody struct {
	closed bool
}

func (b *trackingBody) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

// errorRoundTripper always returns an error.
type errorRoundTripper struct{}

func (e errorRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("test transport error")
}

func TestSingleTargetClosesBodyOnError(t *testing.T) {
	// Prepare a single-target multiTransport with an erroring RoundTripper.
	u, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	mt := &multiTransport{
		rt:      errorRoundTripper{},
		targets: []*url.URL{u},
		keys:    []string{"dummy"},
	}

	// Request with a tracking body.
	body := &trackingBody{}
	req, err := http.NewRequest(http.MethodPost, "http://agent/proxy", nil)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	// Set our tracking body directly so Close() is invoked by the code under test
	req.Body = body

	resp, rerr := mt.RoundTrip(req)
	if rerr == nil {
		t.Fatalf("expected error, got nil (resp=%v)", resp)
	}
	if resp != nil {
		resp.Body.Close()
		t.Fatalf("expected nil response on transport error, got: %v", resp)
	}
	if !body.closed {
		t.Fatalf("expected request body to be closed on error")
	}
}
