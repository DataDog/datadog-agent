// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultMaxResponseBodyBytes caps the user-app response body that PassThrough
// will surface to the platform. Lifecycle responses are expected to be small
// or empty; a 1 MiB cap prevents a misbehaving user app from OOMing the agent
// or wedging the handler with an unbounded chunked body.
const defaultMaxResponseBodyBytes int64 = 1 << 20

const (
	defaultForwardTimeout  = 1 * time.Second
	defaultReadyTimeout    = 60 * time.Second
	defaultValidateTimeout = 1 * time.Second
)

// Forwarder POSTs lifecycle hooks to the user app. It is constructed only
// when DD_AWS_MICROVM_USER_APP_PORT is set and we're in init-container
// mode + MicroVM origin.
type Forwarder struct {
	target               string        // e.g. "http://127.0.0.1:8080"
	client               *http.Client  // shared; no client-level Timeout (per-call deadlines via ctx)
	forwardTimeout       time.Duration // default 1s, used for suspend/terminate/run/resume
	readyTimeout         time.Duration // default 60s, used for /ready
	validateTimeout      time.Duration // default 1s, used for /validate
	maxResponseBodyBytes int64         // default defaultMaxResponseBodyBytes; cap on user-app body surfaced to platform
}

// NewForwarder constructs a Forwarder targeting 127.0.0.1:<port>. The forwardTimeout
// is used for /run, /resume, /suspend, and /terminate; readyTimeout for /ready;
// validateTimeout for /validate. Callers should pass the default* constants above
// or values parsed from the DD_AWS_MICROVM_*_TIMEOUT_MS env vars.
func NewForwarder(port int, forwardTimeout, readyTimeout, validateTimeout time.Duration) *Forwarder {
	return &Forwarder{
		target: fmt.Sprintf("http://127.0.0.1:%d", port),
		// DisableKeepAlives prevents the transport from caching idle connections.
		// MicroVM instances are restored from a Firecracker snapshot, so any
		// connection the transport pooled before the snapshot is stale on resume.
		// Since Go's HTTP transport does not auto-retry POST on a stale connection,
		// disabling keep-alives ensures every lifecycle hook uses a fresh dial.
		// The overhead is negligible for loopback calls at this frequency.
		client: &http.Client{
			// DisableCompression prevents the transport from adding its own
			// Accept-Encoding: gzip and transparently decompressing a gzipped
			// response, which would strip Content-Encoding/Content-Length and
			// change the body the platform sees. Responses must be mirrored
			// verbatim, so compression negotiation is left to the user app.
			Transport: &http.Transport{DisableKeepAlives: true, DisableCompression: true},
			// Lifecycle hooks are mirrored to the platform verbatim (status, body,
			// Content-Type). A 3xx from the user app is not part of the documented
			// hook contract (only 200, or 503 for /ready and /validate) — silently
			// following it would report the redirect target's response instead of
			// the hook's own, and for 301/302/303 would replay the request as a
			// GET with the body dropped, letting the actual hook handler's logic
			// run before we ever observe it. Returning the 3xx as-is preserves
			// both the mirroring contract and the platform's hook status semantics.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		forwardTimeout:       forwardTimeout,
		readyTimeout:         readyTimeout,
		validateTimeout:      validateTimeout,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}
}

// PassThroughWaiting waits for the user app to accept TCP connections bounded
// by timeout, then forwards the request and mirrors the response. Used for:
//   - /ready    ("I am booted and ready to be snapshotted"): the platform
//     retries on non-200, so the TCP wait absorbs the startup race.
//   - /validate ("I was resumed from a snapshot and everything is good"): the
//     TCP wait handles a crash-then-restart between resume and this
//     call.
//
// Body is buffered before the TCP wait so the bytes survive waitForUserApp.
// Deadline exceeded maps to 504. Body Close contract documented on PassThrough.
func (f *Forwarder) PassThroughWaiting(timeout time.Duration, path string, headers http.Header, body io.Reader) *http.Response {
	var bodyBytes []byte
	if body != nil {
		var err error
		// Read the full inbound body before the TCP wait. A read error is a
		// server-side failure (network, OS, memory) — not a client mistake —
		// so return 500 rather than 400. We still forward nothing to the user
		// app to avoid passing a partial body (which could make it answer
		// /validate "healthy" off incomplete data).
		if bodyBytes, err = io.ReadAll(body); err != nil {
			return statusOnlyResponse(http.StatusInternalServerError)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	if err := f.waitForUserApp(ctx); err != nil {
		cancel()
		return statusOnlyResponse(mapErrToStatus(err))
	}
	resp, err := f.do(ctx, path, headers, bytes.NewReader(bodyBytes))
	if err != nil {
		cancel()
		return statusOnlyResponse(mapErrToStatus(err))
	}
	resp.Body = wrapResponseBody(resp.Body, f.maxResponseBodyBytes, cancel)
	return resp
}

// waitForUserApp polls the user app's TCP port until a connection succeeds or
// ctx is cancelled. Returns nil when the port is reachable, ctx.Err() when
// the deadline is exceeded. Polls every 50ms.
func (f *Forwarder) waitForUserApp(ctx context.Context) error {
	addr := strings.TrimPrefix(f.target, "http://")
	dialer := &net.Dialer{}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// PassThrough forwards a per-MicroVM lifecycle hook (/run, /resume,
// /suspend, /terminate) to the user app and returns the user-app response.
// Bounded by forwardTimeout (default 1s) — sized as <50% of the tightest
// AWS Lambda MicroVM platform bound (/terminate's 60s) so the agent always
// returns within the platform's window. Dial errors map to 503; deadline
// exceeded maps to 504. The returned *http.Response always has a non-nil
// Body that the caller MUST Close. On the error path the Body is an empty
// NopCloser (Close is a no-op). On the happy path the Body is wrapped
// twice: io.LimitReader (inner) caps reads at maxResponseBodyBytes;
// cancelOnCloseReader (outer) keeps the ctx alive across the caller's body
// read and cancels it on Close. The cap reader MUST be the inner layer;
// inverting the layering leaks the ctx cancel.
func (f *Forwarder) PassThrough(path string, headers http.Header, body io.Reader) *http.Response {
	return f.passThroughWith(f.forwardTimeout, path, headers, body)
}

func (f *Forwarder) passThroughWith(timeout time.Duration, path string, headers http.Header, body io.Reader) *http.Response {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	resp, err := f.do(ctx, path, headers, body)
	if err != nil {
		cancel()
		return statusOnlyResponse(mapErrToStatus(err))
	}
	resp.Body = wrapResponseBody(resp.Body, f.maxResponseBodyBytes, cancel)
	return resp
}

// wrapResponseBody applies the LimitReader-inner / cancelOnCloseReader-outer
// layering required by PassThrough. cap <= 0 disables the cap (used in tests
// that need the raw body); production callers always pass a positive cap.
func wrapResponseBody(body io.ReadCloser, cap int64, cancel context.CancelFunc) io.ReadCloser {
	inner := body
	if cap > 0 {
		inner = &limitedReadCloser{
			Reader: io.LimitReader(body, cap),
			Closer: body,
		}
	}
	return &cancelOnCloseReader{ReadCloser: inner, cancel: cancel}
}

// limitedReadCloser combines io.LimitReader's Read with the original body's
// Close. Needed because io.LimitReader returns a Reader, not a ReadCloser.
type limitedReadCloser struct {
	io.Reader
	io.Closer
}

func (f *Forwarder) do(ctx context.Context, path string, headers http.Header, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.target+path, body)
	if err != nil {
		return nil, err
	}
	if ct := headers.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if cl := headers.Get("Content-Length"); cl != "" {
		// Parse the platform-supplied Content-Length and assign it to
		// req.ContentLength. Without this, net/http treats the body as having
		// an unknown length and falls back to chunked transfer encoding, which
		// some user-app HTTP servers reject or misparse on lifecycle endpoints.
		// ParseInt args: base 10 (decimal string), 64-bit result size (fits
		// int64, the type of req.ContentLength). Malformed values are logged
		// and the request is sent without a Content-Length header.
		if n, parseErr := strconv.ParseInt(cl, 10, 64); parseErr == nil {
			req.ContentLength = n
		} else {
			log.Debugf("MicroVM lifecycle: could not parse platform Content-Length %q: %s", cl, parseErr)
		}
	}
	return f.client.Do(req)
}

func mapErrToStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusServiceUnavailable
}

func statusOnlyResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
	}
}

type cancelOnCloseReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnCloseReader) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
