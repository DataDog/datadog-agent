// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestForwarder(target string) *Forwarder {
	return &Forwarder{
		target:          target,
		client:          &http.Client{},
		forwardTimeout:  200 * time.Millisecond,
		readyTimeout:    200 * time.Millisecond,
		validateTimeout: 200 * time.Millisecond,
	}
}

func TestForwarder_PassThrough_MirrorsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"ready":false}`))
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	resp := f.PassThrough("/x", http.Header{"Content-Type": []string{"application/json"}}, bytes.NewReader([]byte("{}")))
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, 503, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, `{"ready":false}`, string(body))
}

func TestForwarder_PassThrough_TimeoutMapsTo504(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	resp := f.PassThrough("/x", nil, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
}

func TestForwarder_PassThrough_ConnRefusedMapsTo503(t *testing.T) {
	f := newTestForwarder("http://127.0.0.1:1") // unbound port
	resp := f.PassThrough("/x", nil, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

// User-app responses that exceed the configured body cap must be truncated
// at the cap; a misbehaving or malicious user app cannot OOM the agent or
// wedge the handler with an unbounded body. Status code is preserved.
func TestForwarder_PassThrough_TruncatesBodyAtCap(t *testing.T) {
	const cap = 256
	payload := bytes.Repeat([]byte("A"), cap*4) // 4x the cap
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	f.maxResponseBodyBytes = cap
	resp := f.PassThrough("/x", nil, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, cap, len(body), "body must be truncated at maxResponseBodyBytes")
}

// TestNewForwarder_DisableKeepAlives_OpensNewConnectionPerRequest is the
// behavioral counterpart to TestNewForwarder_Defaults. It verifies that
// DisableKeepAlives actually prevents TCP connection reuse by counting the
// number of new connections the test server accepts: with keep-alives disabled
// each request must open a fresh connection, so the connection count must equal
// the request count. A keep-alives-enabled client would reuse one connection
// for all three requests and the counter would stay at 1.
//
// This matters for MicroVM snapshot/restore: any connection pooled inside a
// Firecracker snapshot is stale on resume, and Go's HTTP transport does not
// auto-retry POST on a stale connection. Forcing a fresh dial per call
// eliminates the class of "first /run hook fails with 503/504" failures.
func TestNewForwarder_DisableKeepAlives_OpensNewConnectionPerRequest(t *testing.T) {
	var newConns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)

	f := NewForwarder(port, defaultForwardTimeout, defaultReadyTimeout, defaultValidateTimeout)
	const requests = 3
	for i := 0; i < requests; i++ {
		resp := f.PassThrough("/x", nil, nil)
		require.NotNil(t, resp)
		resp.Body.Close()
	}
	assert.Equal(t, int32(requests), newConns.Load(),
		"DisableKeepAlives must open a fresh TCP connection per request; with keep-alives enabled only 1 connection would be accepted")
}

// NewForwarder defaults reflect the configured wire.go constants. /ready keeps
// a 60s budget (matches the platform hook timeout); the remaining hooks default
// to 1s. Changing these defaults is a deliberate behavior change — the test
// failure is intentional.
func TestNewForwarder_Defaults(t *testing.T) {
	f := NewForwarder(8080, defaultForwardTimeout, defaultReadyTimeout, defaultValidateTimeout)
	assert.Equal(t, 1*time.Second, f.forwardTimeout, "forwardTimeout default must be 1s")
	assert.Equal(t, 60*time.Second, f.readyTimeout, "readyTimeout default must be 60s (matches platform /ready hook timeout)")
	assert.Equal(t, 1*time.Second, f.validateTimeout, "validateTimeout default must be 1s")
	assert.Equal(t, defaultMaxResponseBodyBytes, f.maxResponseBodyBytes, "maxResponseBodyBytes default must be 1 MiB")
	tr, ok := f.client.Transport.(*http.Transport)
	require.True(t, ok, "transport must be *http.Transport")
	assert.True(t, tr.DisableKeepAlives, "DisableKeepAlives must be true: MicroVM instances resume from a Firecracker snapshot, so pooled connections are stale on resume and POST is not auto-retried")
	assert.True(t, tr.DisableCompression, "DisableCompression must be true: responses are mirrored verbatim, and transparent gzip decompression would strip Content-Encoding/Content-Length")
}

// PassThrough must not silently follow redirects returned by the user app.
// The hook contract only recognizes 200 (success) and 503 (retry, for /ready
// and /validate) — a 3xx is outside that contract, and Go's default client
// would replay a POST redirect as a GET with the body dropped, letting a
// different handler answer for the hook instead of the one the platform
// actually invoked. PassThrough must mirror the 3xx as-is.
func TestForwarder_PassThrough_DoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x" {
			t.Errorf("redirect target %s must not be contacted", r.URL.Path)
			return
		}
		w.Header().Set("Location", "/x/")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)

	f := NewForwarder(port, defaultForwardTimeout, defaultReadyTimeout, defaultValidateTimeout)
	resp := f.PassThrough("/x", nil, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode, "redirect must be mirrored, not followed")
	assert.Equal(t, "/x/", resp.Header.Get("Location"))
}

// On the error path (dial error, deadline) PassThrough returns a non-nil
// response with an empty body whose Close is a no-op. Deferred Close in
// callers MUST be safe regardless of which path produced the response.
func TestForwarder_PassThrough_ErrorStubBodyCloseIsNoop(t *testing.T) {
	f := newTestForwarder("http://127.0.0.1:1") // dial-error path
	resp := f.PassThrough("/x", nil, nil)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Body)
	// Two consecutive closes must not error or panic — pin no-op semantics.
	assert.NoError(t, resp.Body.Close())
	assert.NoError(t, resp.Body.Close())
}

// PassThrough must propagate the incoming Content-Type to the user app.
// Header forwarding is the only caller-visible piece of the request shape
// (path is fixed, body is opaque), so it gets a dedicated pin.
func TestForwarder_PassThrough_ForwardsContentType(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Content-Type")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	resp := f.PassThrough("/h", http.Header{"Content-Type": []string{"application/x-test"}}, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	select {
	case ct := <-got:
		assert.Equal(t, "application/x-test", ct)
	case <-time.After(time.Second):
		t.Fatal("user app handler never invoked")
	}
}

// A malformed platform Content-Length must not block the request: do() logs
// and skips setting req.ContentLength, so net/http falls back to chunked
// transfer instead of failing the forward.
func TestForwarder_PassThrough_MalformedContentLengthIsIgnored(t *testing.T) {
	got := make(chan int64, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.ContentLength
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	// io.NopCloser hides the concrete *bytes.Reader type from net/http's
	// length auto-detection, so ContentLength can only come from the header.
	body := io.NopCloser(bytes.NewReader([]byte("payload")))
	resp := f.PassThrough("/h", http.Header{"Content-Length": []string{"not-a-number"}}, body)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	select {
	case cl := <-got:
		assert.Equal(t, int64(-1), cl, "malformed Content-Length must not be forwarded")
	case <-time.After(time.Second):
		t.Fatal("user app handler never invoked")
	}
}

// PassThroughWaiting mirrors the user-app's status, Content-Type, and body.
// The platform expects the user-app's actual signal, not an agent-synthesized
// response — this pins the mirror contract shared by /ready and /validate.
func TestForwarder_PassThroughWaiting_MirrorsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ready")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ready":true}`))
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	resp := f.PassThroughWaiting(200*time.Millisecond, "/ready", nil, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/x-ready", resp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, `{"ready":true}`, string(body))
}

// PassThroughWaiting must honor the timeout it receives. The caller (server.go)
// passes readyTimeout or validateTimeout — PassThroughWaiting must not apply
// any other field. This test uses a 50ms timeout against a 200ms upstream: the
// context expires and the call returns 504.
func TestForwarder_PassThroughWaiting_HonorsProvidedTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	resp := f.PassThroughWaiting(50*time.Millisecond, "/ready", nil, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode,
		"PassThroughWaiting must time out on the provided timeout")
}

// On the happy path the response body is wrapped by cancelOnCloseReader.
// Closing the body MUST cancel the per-call context — without this, the
// timeout goroutine and any associated transport state leaks for the
// remainder of the timeout window. This pins the cleanup contract that the
// passThroughWith / wrapResponseBody layering relies on.
func TestForwarder_PassThrough_BodyCloseCancelsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	resp := f.PassThrough("/x", nil, nil)
	require.NotNil(t, resp)

	cor, ok := resp.Body.(*cancelOnCloseReader)
	require.True(t, ok, "happy-path body must be wrapped by cancelOnCloseReader")

	called := atomic.Int32{}
	original := cor.cancel
	cor.cancel = func() {
		called.Add(1)
		original()
	}
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, int32(1), called.Load(), "Close must invoke the per-call cancel exactly once")
}

// wrapResponseBody with cap <= 0 must skip the LimitReader layer and return
// the body unmodified through cancelOnCloseReader. Production callers always
// pass a positive cap, but the disable-cap branch is reachable from any test
// that constructs a Forwarder without setting maxResponseBodyBytes — pinning
// it here prevents accidental panic / wrong-layering regressions.
func TestWrapResponseBody_CapZero_DisablesCap(t *testing.T) {
	const payload = "long-enough-to-have-been-truncated-if-cap-applied"
	body := io.NopCloser(bytes.NewReader([]byte(payload)))
	wrapped := wrapResponseBody(body, 0, func() {})
	defer wrapped.Close()

	got, err := io.ReadAll(wrapped)
	require.NoError(t, err)
	assert.Equal(t, payload, string(got),
		"cap=0 must read the full body — LimitReader must NOT be applied")
}

// PassThroughWaiting retries TCP dials until the context expires when the port
// is unbound, so the response is always 504 (deadline exceeded) — never 503.
// This is distinct from PassThrough which returns 503 on the very first
// connect-refused error. The behavioral difference is intentional: /ready and
// /validate must wait for the user app to start, not fail-fast on a transient
// dial error.
func TestForwarder_PassThroughWaiting_UnboundPort_RetriesUntilTimeout504(t *testing.T) {
	f := &Forwarder{
		target: "http://127.0.0.1:1", // unbound — every dial attempt is refused
		client: &http.Client{},
	}
	resp := f.PassThroughWaiting(150*time.Millisecond, "/ready", nil, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode,
		"unbound port must retry until timeout (504), not immediately 503 like PassThrough")
}

// passThroughWaiting buffers the request body before the TCP wait so it is
// still available after waitForUserApp returns. Without buffering, the body
// reader would be exhausted during the wait loop and f.do would send an empty
// body to the user app. This pins the buffer-then-forward contract.
func TestForwarder_PassThroughWaiting_ForwardsBodyAfterTCPWait(t *testing.T) {
	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- b
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	payload := []byte(`{"checksum":"abc123"}`)
	resp := f.PassThroughWaiting(200*time.Millisecond, "/ready", nil, bytes.NewReader(payload))
	require.NotNil(t, resp)
	defer resp.Body.Close()

	select {
	case got := <-received:
		assert.Equal(t, payload, got, "body must survive the TCP wait and arrive at the user app intact")
	case <-time.After(time.Second):
		t.Fatal("user app handler never invoked")
	}
}

// errReader fails on Read, simulating a truncated/aborted inbound platform
// request body (e.g. the platform disconnects mid-transfer).
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// A failed read of the inbound body must NOT forward a partial body to the
// user app. PassThroughWaiting returns 500 (server-side read failure) and
// short-circuits before the TCP wait so no partial body reaches the user app.
func TestForwarder_PassThroughWaiting_BodyReadError_Returns500(t *testing.T) {
	var reached atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := newTestForwarder(srv.URL)
	resp := f.PassThroughWaiting(200*time.Millisecond, "/validate", nil, errReader{})
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"a failed inbound body read is a server-side error and must return 500")
	assert.Equal(t, int32(0), reached.Load(),
		"user app must not be contacted when the inbound body cannot be read")
}
