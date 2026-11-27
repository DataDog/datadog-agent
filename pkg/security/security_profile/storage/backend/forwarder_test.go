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
	"testing/synctest"
	"time"

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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
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

// pendingEndpointForServer builds a plain-HTTP Endpoint whose API-key slot holds a DELA
// directive, so it starts pending delegated auth.
func pendingEndpointForServer(t *testing.T, srv *httptest.Server, apiKeyPath string) logsconfig.Endpoint {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return logsconfig.NewEndpoint("DELA[org-uuid, aws]", apiKeyPath, host, port, "", false /* useSSL */)
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

func TestSendToEndpointRetriesTransientFailures(t *testing.T) {
	header := []byte(`{"meta":"data"}`)
	dump := []byte("this is the raw protobuf dump payload")

	t.Run("transient HTTP status codes succeed on retry", func(t *testing.T) {
		statuses := []int{
			http.StatusServiceUnavailable,
			http.StatusRequestTimeout,
			http.StatusAccepted,
		}
		attempts := atomic.NewInt64(0)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			idx := int(attempts.Inc() - 1)
			ev, dp := decodeGzipMultipart(t, r)
			assert.Equal(t, header, ev)
			assert.Equal(t, dump, dp)
			w.WriteHeader(statuses[idx])
		}))
		defer srv.Close()

		backend := &ActivityDumpRemoteBackend{
			tooLargeEntities: atomic.NewUint64(0),
			client:           srv.Client(),
			maxRetries:       3,
			retryBaseDelay:   time.Millisecond,
		}

		require.NoError(t, backend.sendToEndpoint(srv.URL+"/api/v2/secdump", "api-key", header, dump))
		assert.Equal(t, int64(3), attempts.Load())
	})

	t.Run("transport EOF retries with fresh body and succeeds", func(t *testing.T) {
		attempts := atomic.NewInt64(0)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt := attempts.Inc()
			ev, dp := decodeGzipMultipart(t, r)
			assert.Equal(t, header, ev)
			assert.Equal(t, dump, dp)

			if attempt == 1 {
				hj, ok := w.(http.Hijacker)
				require.True(t, ok)
				conn, _, err := hj.Hijack()
				require.NoError(t, err)
				_ = conn.Close()
				return
			}
			w.WriteHeader(http.StatusAccepted)
		}))
		defer srv.Close()

		backend := &ActivityDumpRemoteBackend{
			tooLargeEntities: atomic.NewUint64(0),
			client:           srv.Client(),
			maxRetries:       3,
			retryBaseDelay:   time.Millisecond,
		}

		require.NoError(t, backend.sendToEndpoint(srv.URL+"/api/v2/secdump", "api-key", header, dump))
		assert.Equal(t, int64(2), attempts.Load())
	})

	t.Run("exhausting all retries returns final error", func(t *testing.T) {
		attempts := atomic.NewInt64(0)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Inc()
			ev, dp := decodeGzipMultipart(t, r)
			assert.Equal(t, header, ev)
			assert.Equal(t, dump, dp)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		backend := &ActivityDumpRemoteBackend{
			tooLargeEntities: atomic.NewUint64(0),
			client:           srv.Client(),
			maxRetries:       2,
			retryBaseDelay:   time.Millisecond,
		}

		err := backend.sendToEndpoint(srv.URL+"/api/v2/secdump", "api-key", header, dump)
		require.ErrorContains(t, err, "503 Service Unavailable")
		assert.Equal(t, int64(3), attempts.Load())
		assert.Equal(t, uint64(0), backend.tooLargeEntities.Load())
	})

	t.Run("non-retryable status codes fail on first attempt", func(t *testing.T) {
		cases := []struct {
			name             string
			status           int
			wantTooLargeHits uint64
		}{
			{
				name:             "413 Request Entity Too Large",
				status:           http.StatusRequestEntityTooLarge,
				wantTooLargeHits: 1,
			},
			{
				name:             "400 Bad Request",
				status:           http.StatusBadRequest,
				wantTooLargeHits: 0,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				attempts := atomic.NewInt64(0)

				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts.Inc()
					_, _ = decodeGzipMultipart(t, r)
					w.WriteHeader(tc.status)
				}))
				defer srv.Close()

				backend := &ActivityDumpRemoteBackend{
					tooLargeEntities: atomic.NewUint64(0),
					client:           srv.Client(),
					maxRetries:       3,
					retryBaseDelay:   time.Millisecond,
				}

				err := backend.sendToEndpoint(srv.URL+"/api/v2/secdump", "api-key", header, dump)
				require.Error(t, err)
				assert.Equal(t, int64(1), attempts.Load())
				assert.Equal(t, tc.wantTooLargeHits, backend.tooLargeEntities.Load())
			})
		}
	})

	t.Run("429 honors Retry-After header capped at max delay", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			attempts := atomic.NewInt64(0)
			var attemptTimes []time.Time

			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				attempt := attempts.Inc()
				attemptTimes = append(attemptTimes, time.Now())
				ev, dp := decodeGzipMultipart(t, r)
				assert.Equal(t, header, ev)
				assert.Equal(t, dump, dp)

				resp := &http.Response{
					StatusCode: http.StatusAccepted,
					Status:     "202 Accepted",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}
				if attempt == 1 {
					resp.StatusCode = http.StatusTooManyRequests
					resp.Status = "429 Too Many Requests"
					resp.Header.Set("Retry-After", "2")
				} else if attempt == 2 {
					resp.StatusCode = http.StatusTooManyRequests
					resp.Status = "429 Too Many Requests"
					resp.Header.Set("Retry-After", "60")
				}
				return resp, nil
			})

			backend := &ActivityDumpRemoteBackend{
				tooLargeEntities: atomic.NewUint64(0),
				client:           &http.Client{Transport: rt},
				maxRetries:       3,
				retryBaseDelay:   100 * time.Millisecond,
			}

			require.NoError(t, backend.sendToEndpoint("http://intake.example/api/v2/secdump", "api-key", header, dump))
			require.Len(t, attemptTimes, 3)
			assert.Equal(t, 2*time.Second, attemptTimes[1].Sub(attemptTimes[0]))
			assert.Equal(t, defaultMaxRetryDelay, attemptTimes[2].Sub(attemptTimes[1]))
		})
	})

	t.Run("permanent transport errors fail on first attempt", func(t *testing.T) {
		t.Run("untrusted TLS certificate", func(t *testing.T) {
			attempts := atomic.NewInt64(0)

			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			}))
			srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					attempts.Inc()
				}
			}
			srv.StartTLS()
			defer srv.Close()

			backend := &ActivityDumpRemoteBackend{
				tooLargeEntities: atomic.NewUint64(0),
				client:           &http.Client{},
				maxRetries:       3,
				retryBaseDelay:   time.Second,
			}

			err := backend.sendToEndpoint(srv.URL+"/api/v2/secdump", "api-key", header, dump)
			require.Error(t, err)
			assert.Equal(t, int64(1), attempts.Load())
		})

		t.Run("unsupported protocol scheme", func(t *testing.T) {
			attempts := atomic.NewInt64(0)
			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				attempts.Inc()
				return http.DefaultTransport.RoundTrip(r)
			})
			backend := &ActivityDumpRemoteBackend{
				tooLargeEntities: atomic.NewUint64(0),
				client:           &http.Client{Transport: rt},
				maxRetries:       3,
				retryBaseDelay:   time.Second,
			}

			err := backend.sendToEndpoint("ftp://127.0.0.1/api/v2/secdump", "api-key", header, dump)
			require.ErrorContains(t, err, "unsupported protocol scheme")
			assert.Equal(t, int64(1), attempts.Load())
		})
	})
}

func TestHandleActivityDumpSkipsEndpointWaitingForDelegatedAuth(t *testing.T) {
	primaryHits := atomic.NewInt64(0)
	pendingHits := atomic.NewInt64(0)

	primarySrv := newCountingServer(primaryHits)
	defer primarySrv.Close()
	pendingSrv := newCountingServer(pendingHits)
	defer pendingSrv.Close()

	primaryEp := endpointForServer(t, primarySrv, "api_key")
	// A DELA directive in the API-key slot builds the endpoint as pending delegated auth.
	pendingEp := pendingEndpointForServer(t, pendingSrv, "runtime_security_config.activity_dump.remote_storage.endpoints.additional_endpoints")

	backend := &ActivityDumpRemoteBackend{
		tooLargeEntities: atomic.NewUint64(0),
		client:           primarySrv.Client(),
		endpoints:        logsconfig.NewEndpoints(primaryEp, []logsconfig.Endpoint{pendingEp}, false, true),
	}

	require.NoError(t, backend.HandleActivityDump("image", "tag", []byte(`{}`), []byte("dump")))
	assert.Equal(t, int64(1), primaryHits.Load(), "primary endpoint still receives the dump")
	assert.Equal(t, int64(0), pendingHits.Load(), "a pending delegated-auth endpoint must be skipped, not sent an empty key")
}
