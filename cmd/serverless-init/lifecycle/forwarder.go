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
	// Per AWS, Run/Resume/Suspend/Terminate: 1 second
	// Ready/Validate: 30 seconds
	defaultForwardTimeout  = 1 * time.Second
	defaultReadyTimeout    = 30 * time.Second
	defaultValidateTimeout = 30 * time.Second
)

// dialCheckTimeout bounds the single TCP dial attempt PassThroughWaiting uses
// to check user-app reachability for /ready and /validate. Per the hook
// contract, the platform — not the hook — owns the retry loop for these two
// hooks, so the check must answer fast rather than block until the app is up.
// A refused loopback connection fails in microseconds (the kernel sends RST
// immediately), so this timeout only matters for a slow/hung connect (e.g.
// host scheduling jitter on an oversubscribed MicroVM host). 200ms is
// generous enough to absorb that jitter while staying well inside the "answer
// fast" contract, and costs at most one extra platform retry (a 503) on the
// rare call where it's hit.
const dialCheckTimeout = 200 * time.Millisecond

// Forwarder POSTs lifecycle hooks to the user app. It is constructed only
// when DD_AWS_MICROVM_USER_APP_PORT is set and we're in init-container
// mode + MicroVM origin.
type Forwarder struct {
	target               string        // e.g. "http://127.0.0.1:8080"
	client               *http.Client  // shared; no client-level Timeout (per-call deadlines via ctx)
	forwardTimeout       time.Duration // default 1s, used for suspend/terminate/run/resume
	readyTimeout         time.Duration // default 30s, used for /ready
	validateTimeout      time.Duration // default 30s, used for /validate
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

// PassThroughWaiting checks user-app reachability with a single fast TCP dial
// (bounded by dialCheckTimeout), then forwards the request bounded by timeout
// and mirrors the response. Used for:
//   - /ready    ("I am booted and ready to be snapshotted"): the platform
//     retries on non-200 until its own configured timeout, so the hook
//     answers fast rather than blocking on the startup race.
//   - /validate ("I was resumed from a snapshot and everything is good"): the
//     same fast-answer contract applies to the crash-then-restart window
//     between resume and this call.
//
// Per the /ready and /validate hook contract, only 200 and 503 are meaningful
// responses — the platform retries on 503 until its own configured timeout,
// while any other non-200 (including 504) fails the build. So unlike
// PassThrough, an unreachable app or a deadline exceeded here always maps to
// 503, never 504. Body Close contract documented on PassThrough.
func (f *Forwarder) PassThroughWaiting(timeout time.Duration, path string, headers http.Header, body io.Reader) *http.Response {
	var bodyBytes []byte
	if body != nil {
		var err error
		// Read the full inbound body before the reachability check. A read
		// error is a server-side failure (network, OS, memory) — not a client
		// mistake — so return 500 rather than 400. We still forward nothing to
		// the user app to avoid passing a partial body (which could make it
		// answer /validate "healthy" off incomplete data).
		if bodyBytes, err = io.ReadAll(body); err != nil {
			return statusOnlyResponse(http.StatusInternalServerError)
		}
	}
	if !f.reachable(dialCheckTimeout) {
		return statusOnlyResponse(http.StatusServiceUnavailable)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	resp, err := f.do(ctx, path, headers, bytes.NewReader(bodyBytes))
	if err != nil {
		cancel()
		return statusOnlyResponse(http.StatusServiceUnavailable)
	}
	resp.Body = wrapResponseBody(resp.Body, f.maxResponseBodyBytes, cancel)
	return resp
}

// reachable performs a single TCP dial attempt to the user app, bounded by
// timeout. No polling/retrying: per the hook contract, the platform (not the
// hook) owns the retry loop for /ready and /validate.
func (f *Forwarder) reachable(timeout time.Duration) bool {
	addr := strings.TrimPrefix(f.target, "http://")
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
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
