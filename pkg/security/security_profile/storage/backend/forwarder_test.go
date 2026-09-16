// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package backend holds files related to forwarder backends for security profiles
package backend

import (
	"compress/gzip"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestMRFFailoverActive(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		failoverLogs bool
		want         bool
	}{
		{name: "mrf disabled", enabled: false, failoverLogs: false, want: false},
		{name: "enabled but no active failover", enabled: true, failoverLogs: false, want: false},
		{name: "failover flag set but mrf disabled", enabled: false, failoverLogs: true, want: false},
		{name: "enabled and failing over", enabled: true, failoverLogs: true, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("multi_region_failover.enabled", tc.enabled)
			cfg.SetInTest("multi_region_failover.failover_logs", tc.failoverLogs)

			assert.Equal(t, tc.want, mrfFailoverActive(cfg))
		})
	}
}

// newCountingServer returns an httptest server that accepts every request (202 Accepted, the
// status sendToEndpoint treats as success) and increments hits on each call.
func newCountingServer(hits *atomic.Int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Inc()
		w.WriteHeader(http.StatusAccepted)
	}))
}

// endpointForServer builds a plain-HTTP Endpoint targeting the given test server so that
// GetEndpointURL produces an http:// URL matching httptest.
func endpointForServer(t *testing.T, srv *httptest.Server, apiKeyPath string) logsconfig.Endpoint {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return logsconfig.NewEndpoint("api-key", apiKeyPath, host, port, "", false /* useSSL */)
}

// TestSendToEndpointStreamsGzippedMultipart verifies the streaming send path: the request body
// must be gzip-encoded, streamed (chunked, i.e. no precomputed Content-Length), and decode back
// to the exact event/dump parts that were handed in. This locks in the io.Pipe rewrite that
// dropped the full gzipped body buffer — if a refactor reintroduces buffering or breaks the
// encoding chain (multipart -> gzip -> pipe), the round-trip assertions below fail.
func TestSendToEndpointStreamsGzippedMultipart(t *testing.T) {
	header := []byte(`{"meta":"data"}`)
	dump := []byte("this is the raw protobuf dump payload")

	type received struct {
		contentType     string
		contentEncoding string
		contentLength   int64
		event           []byte
		dump            []byte
	}
	got := make(chan received, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := received{
			contentType:     r.Header.Get("Content-Type"),
			contentEncoding: r.Header.Get("Content-Encoding"),
			contentLength:   r.ContentLength,
		}

		// The server does not auto-decompress request bodies, so Content-Encoding: gzip means
		// r.Body is the raw gzip stream we produced.
		gz, err := gzip.NewReader(r.Body)
		require.NoError(t, err)
		defer gz.Close()

		_, params, err := mime.ParseMediaType(rec.contentType)
		require.NoError(t, err)

		mr := multipart.NewReader(gz, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			b, err := io.ReadAll(part)
			require.NoError(t, err)
			switch part.FormName() {
			case "event":
				rec.event = b
			case "dump":
				rec.dump = b
			}
		}

		got <- rec
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	backend := &ActivityDumpRemoteBackend{
		tooLargeEntities: atomic.NewUint64(0),
		client:           srv.Client(),
	}

	require.NoError(t, backend.sendToEndpoint(srv.URL+"/api/v2/secdump", "api-key", header, dump))

	rec := <-got
	assert.Equal(t, "gzip", rec.contentEncoding, "body must be gzip-encoded")
	assert.Equal(t, int64(-1), rec.contentLength, "streamed body should have unknown length (chunked), not a precomputed size")
	assert.True(t, strings.HasPrefix(rec.contentType, "multipart/form-data"), "content type should be multipart form-data, got %q", rec.contentType)
	assert.Equal(t, header, rec.event, "event part must round-trip through the streamed body")
	assert.Equal(t, dump, rec.dump, "dump part must round-trip through the streamed body")
}

// decodeGzipMultipart reads a gzip+multipart request body and returns the event and dump parts.
func decodeGzipMultipart(t *testing.T, r *http.Request) (event []byte, dump []byte) {
	t.Helper()
	gz, err := gzip.NewReader(r.Body)
	require.NoError(t, err)
	defer gz.Close()

	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	require.NoError(t, err)

	mr := multipart.NewReader(gz, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		b, err := io.ReadAll(part)
		require.NoError(t, err)
		switch part.FormName() {
		case "event":
			event = b
		case "dump":
			dump = b
		}
	}
	return event, dump
}

// TestSendToEndpointReplaysBodyOnRedirect verifies the send path sets Request.GetBody so a
// streamed (chunked) body can be regenerated. Without GetBody, net/http refuses to resend a body
// on a 307/308 redirect and the dump would be logged as a failure. The regenerated body must be
// byte-for-byte the same parts under the same multipart boundary as the original attempt.
func TestSendToEndpointReplaysBodyOnRedirect(t *testing.T) {
	header := []byte(`{"meta":"data"}`)
	dump := []byte("this is the raw protobuf dump payload")

	type received struct {
		event []byte
		dump  []byte
	}
	got := make(chan received, 2)

	mux := http.NewServeMux()
	// The first hop 307-redirects to /final while preserving method and body; net/http can only
	// follow it (rather than returning the 307 to the caller) when GetBody is set.
	mux.HandleFunc("/api/v2/secdump", func(w http.ResponseWriter, r *http.Request) {
		ev, dp := decodeGzipMultipart(t, r)
		got <- received{event: ev, dump: dp}
		http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		ev, dp := decodeGzipMultipart(t, r)
		got <- received{event: ev, dump: dp}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	backend := &ActivityDumpRemoteBackend{
		tooLargeEntities: atomic.NewUint64(0),
		client:           srv.Client(),
	}

	require.NoError(t, backend.sendToEndpoint(srv.URL+"/api/v2/secdump", "api-key", header, dump))

	// Both the original request and the redirected replay must carry the identical decoded parts.
	first := <-got
	second := <-got
	assert.Equal(t, header, first.event)
	assert.Equal(t, dump, first.dump)
	assert.Equal(t, header, second.event, "regenerated body (GetBody) must round-trip the event part")
	assert.Equal(t, dump, second.dump, "regenerated body (GetBody) must round-trip the dump part")
}

func TestHandleActivityDumpGatesMRFEndpointOnFailover(t *testing.T) {
	primaryHits := atomic.NewInt64(0)
	mrfHits := atomic.NewInt64(0)

	primarySrv := newCountingServer(primaryHits)
	defer primarySrv.Close()
	mrfSrv := newCountingServer(mrfHits)
	defer mrfSrv.Close()

	primaryEp := endpointForServer(t, primarySrv, "api_key")
	mrfEp := endpointForServer(t, mrfSrv, "multi_region_failover.api_key")
	mrfEp.IsMRF = true

	backend := &ActivityDumpRemoteBackend{
		tooLargeEntities: atomic.NewUint64(0),
		client:           primarySrv.Client(),
		endpoints:        logsconfig.NewEndpoints(primaryEp, []logsconfig.Endpoint{mrfEp}, false, true),
	}

	cfg := configmock.New(t)
	cfg.SetInTest("multi_region_failover.enabled", true)

	// No active failover: the MRF endpoint must be skipped while the primary still receives the dump.
	cfg.SetInTest("multi_region_failover.failover_logs", false)
	require.NoError(t, backend.HandleActivityDump("image", "tag", []byte(`{}`), []byte("dump")))
	assert.Equal(t, int64(1), primaryHits.Load(), "primary endpoint should always receive the dump")
	assert.Equal(t, int64(0), mrfHits.Load(), "MRF endpoint must not be used outside of an active failover")

	// Remote Configuration switches the failover on: the MRF endpoint now receives the dump too.
	cfg.SetInTest("multi_region_failover.failover_logs", true)
	require.NoError(t, backend.HandleActivityDump("image", "tag", []byte(`{}`), []byte("dump")))
	assert.Equal(t, int64(2), primaryHits.Load())
	assert.Equal(t, int64(1), mrfHits.Load(), "MRF endpoint should receive the dump while failing over")
}
