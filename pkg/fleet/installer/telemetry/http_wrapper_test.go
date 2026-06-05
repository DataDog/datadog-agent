// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorRoundTripper struct {
	err error
}

func (rt errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, rt.err
}

func roundTripWithClosedBody(t *testing.T, rt http.RoundTripper, req *http.Request) error {
	t.Helper()
	res, err := rt.RoundTrip(req)
	if res != nil && res.Body != nil {
		require.NoError(t, res.Body.Close())
	}
	require.Nil(t, res)
	return err
}

func TestRoundTripDNSNotFoundErrorExpected(t *testing.T) {
	globalTracer = &tracer{spans: make(map[uint64]*Span)}
	host := "install.datadoghq.com.internal.dda-testing.com"
	rtErr := &net.DNSError{
		Err:        "no such host",
		Name:       host,
		IsNotFound: true,
	}
	rt := WrapRoundTripper(errorRoundTripper{
		err: &url.Error{Op: "Get", URL: "https://" + host + "/v2/", Err: rtErr},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+host+"/v2/", nil)
	require.NoError(t, err)

	err = roundTripWithClosedBody(t, rt, req)
	require.Error(t, err)

	spans := globalTracer.flushCompletedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, int32(0), spans[0].span.Error)
	assert.Equal(t, err.Error(), spans[0].span.Meta["http.errors"])
	assert.Equal(t, "dns_not_found", spans[0].span.Meta["expected_error"])
	assert.NotContains(t, spans[0].span.Meta, "error.message")
}

func TestRoundTripDNSNotFoundErrorUnexpectedForRegularHost(t *testing.T) {
	globalTracer = &tracer{spans: make(map[uint64]*Span)}
	host := "missing.registry.example.com"
	rtErr := &net.DNSError{
		Err:        "no such host",
		Name:       host,
		IsNotFound: true,
	}
	rt := WrapRoundTripper(errorRoundTripper{
		err: &url.Error{Op: "Get", URL: "https://" + host + "/v2/", Err: rtErr},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+host+"/v2/", nil)
	require.NoError(t, err)

	err = roundTripWithClosedBody(t, rt, req)
	require.Error(t, err)

	spans := globalTracer.flushCompletedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, int32(1), spans[0].span.Error)
	assert.Equal(t, err.Error(), spans[0].span.Meta["http.errors"])
	assert.Equal(t, err.Error(), spans[0].span.Meta["error.message"])
	assert.NotContains(t, spans[0].span.Meta, "expected_error")
}

func TestRoundTripDNSNotFoundErrorUnexpectedForProxyHost(t *testing.T) {
	globalTracer = &tracer{spans: make(map[uint64]*Span)}
	host := "install.datadoghq.com.internal.dda-testing.com"
	proxyHost := "proxy.example.com"
	rtErr := &net.DNSError{
		Err:        "no such host",
		Name:       proxyHost,
		IsNotFound: true,
	}
	rt := WrapRoundTripper(errorRoundTripper{
		err: &url.Error{Op: "proxyconnect", URL: "https://" + proxyHost + "/", Err: rtErr},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+host+"/v2/", nil)
	require.NoError(t, err)

	err = roundTripWithClosedBody(t, rt, req)
	require.Error(t, err)

	spans := globalTracer.flushCompletedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, int32(1), spans[0].span.Error)
	assert.Equal(t, err.Error(), spans[0].span.Meta["http.errors"])
	assert.Equal(t, err.Error(), spans[0].span.Meta["error.message"])
	assert.NotContains(t, spans[0].span.Meta, "expected_error")
}

func TestRoundTripNonDNSTransportErrorUnexpected(t *testing.T) {
	globalTracer = &tracer{spans: make(map[uint64]*Span)}
	rtErr := errors.New("connection refused")
	rt := WrapRoundTripper(errorRoundTripper{err: rtErr})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)

	err = roundTripWithClosedBody(t, rt, req)
	require.ErrorIs(t, err, rtErr)

	spans := globalTracer.flushCompletedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, int32(1), spans[0].span.Error)
	assert.Equal(t, rtErr.Error(), spans[0].span.Meta["http.errors"])
	assert.Equal(t, rtErr.Error(), spans[0].span.Meta["error.message"])
	assert.NotContains(t, spans[0].span.Meta, "expected_error")
}
