// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// resetClient wraps a test server's client in the ResetClient the sender expects.
func resetClient(c *http.Client) *config.ResetClient {
	return config.NewResetClient(0, func() *http.Client { return c })
}

// stubProvider is a CredentialProvider whose readiness the test controls.
type stubProvider struct {
	key   string
	ready atomic.Bool
}

func (p *stubProvider) Authorize(h http.Header) bool {
	if !p.ready.Load() {
		return false
	}
	h.Set("DD-Api-Key", p.key)
	return true
}

// An endpoint with a plain API key must be unaffected by any of this.
func TestAuthorizeStampsThePlainAPIKeyWhenThereIsNoProvider(t *testing.T) {
	m := &apiKeyManager{apiKey: "plain-key"}

	h := http.Header{}
	require.True(t, m.Authorize(h))
	assert.Equal(t, "plain-key", h.Get("DD-Api-Key"))
}

// A provider replaces the API key entirely - the endpoint has no key of its own to fall back to.
func TestAuthorizeUsesTheProviderWhenOneIsSet(t *testing.T) {
	p := &stubProvider{key: "delegated-key"}
	p.ready.Store(true)
	m := &apiKeyManager{provider: p}

	h := http.Header{}
	require.True(t, m.Authorize(h))
	assert.Equal(t, "delegated-key", h.Get("DD-Api-Key"))
}

func TestAuthorizeReportsNotReadyWhileTheCredentialIsResolving(t *testing.T) {
	m := &apiKeyManager{provider: &stubProvider{key: "delegated-key"}}

	h := http.Header{}
	assert.False(t, m.Authorize(h))
	assert.Empty(t, h, "nothing may be stamped when there is no credential")
}

// The payload must not reach the intake before a credential exists, and do must say so with the
// error type that holds it rather than one that burns a retry.
func TestDoDoesNotSendWithoutACredential(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &sender{
		cfg:           &senderConfig{client: resetClient(srv.Client()), userAgent: "test"},
		apiKeyManager: &apiKeyManager{provider: &stubProvider{key: "delegated-key"}},
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, http.NoBody)
	require.NoError(t, err)

	err = s.do(req)

	require.IsType(t, &credentialNotReadyError{}, err,
		"must not be a retriableError, which would count against maxRetries and drop the payload")
	assert.Zero(t, requests.Load(), "nothing may reach the intake without a credential")
}

// Once the credential lands the same sender starts sending, with no rebuild.
func TestDoSendsOnceTheCredentialArrives(t *testing.T) {
	var gotKey atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey.Store(r.Header.Get("DD-Api-Key"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &stubProvider{key: "delegated-key"}
	s := &sender{
		cfg:           &senderConfig{client: resetClient(srv.Client()), userAgent: "test"},
		apiKeyManager: &apiKeyManager{provider: p},
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL, http.NoBody)
	require.NoError(t, err)
	require.IsType(t, &credentialNotReadyError{}, s.do(req))

	p.ready.Store(true)

	req, err = http.NewRequest(http.MethodPost, srv.URL, http.NoBody)
	require.NoError(t, err)
	require.NoError(t, s.do(req))
	assert.Equal(t, "delegated-key", gotKey.Load())
}

// The point of the separate error type: a payload waiting on a credential must survive more
// attempts than maxRetries, because the wait is not a delivery failure. With a retriableError it
// would be dropped after maxRetries and the data lost during startup.
func TestWaitingForACredentialDoesNotExhaustRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &sender{
		cfg:           &senderConfig{client: resetClient(srv.Client()), userAgent: "test", url: mustParseURL(t, srv.URL), recorder: &mockRecorder{}},
		apiKeyManager: &apiKeyManager{provider: &stubProvider{key: "delegated-key"}},
		maxRetries:    3,
		inflight:      atomic.NewInt32(0),
		enabled:       atomic.NewBool(true),
	}
	p := newPayload(nil)

	for i := 0; i < 10; i++ {
		require.False(t, s.sendOnce(p), "attempt %d must keep the payload queued, not drop it", i)
	}
	assert.Zero(t, p.retries.Load(), "waiting on a credential must not consume retries")
}

// A sender shutting down must not hold payloads forever waiting for a credential that will never
// come; it drops them like the retriable path does.
func TestPayloadIsReleasedWhenTheSenderClosesWhileWaiting(t *testing.T) {
	s := &sender{
		cfg:           &senderConfig{client: resetClient(http.DefaultClient), userAgent: "test", url: mustParseURL(t, "http://localhost:0"), recorder: &mockRecorder{}},
		apiKeyManager: &apiKeyManager{provider: &stubProvider{key: "delegated-key"}},
		maxRetries:    3,
		closed:        true,
		inflight:      atomic.NewInt32(0),
		enabled:       atomic.NewBool(true),
	}
	p := newPayload(nil)

	assert.True(t, s.sendOnce(p), "a closed sender must let the payload go rather than wait forever")
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

// An endpoint built from a directive that never produced an instance - an unsupported cloud
// provider, or a consumer with no provider wiring - has neither a provider nor a key. Stamping the
// empty key would send that org's payload to its intake unauthenticated.
func TestAuthorizeRefusesWhenThereIsNeitherProviderNorKey(t *testing.T) {
	m := &apiKeyManager{}

	h := http.Header{}
	assert.False(t, m.Authorize(h))
	assert.Empty(t, h, "an empty API key must never be stamped")
}

// sendPayloads pushes to each sender in turn, and the queue is only one deep, so a blocking send
// on a full queue stalls every sender after it. A sender whose credential never arrives stays full
// indefinitely, which used to stop trace delivery to the primary org entirely. Pushing to such a
// sender must drop for that endpoint alone instead of blocking.
func TestPushDoesNotBlockOnASenderAwaitingACredential(t *testing.T) {
	s := &sender{
		cfg:           &senderConfig{maxQueued: 1},
		apiKeyManager: &apiKeyManager{provider: &stubProvider{key: "delegated-key"}},
		queue:         make(chan *payload, 1),
		inflight:      atomic.NewInt32(0),
		enabled:       atomic.NewBool(true),
		statsd:        &statsd.NoOpClient{},
	}
	s.awaitingCredential.Store(true)
	s.queue <- newPayload(nil) // queue is now full

	done := make(chan struct{})
	go func() {
		s.Push(newPayload(nil))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Push blocked on a sender with no credential; this stalls delivery to every other endpoint")
	}
}

// The drop above must be limited to the no-credential case: an ordinary busy sender must keep its
// blocking behaviour, which is what applies backpressure instead of losing traces.
func TestPushStillBlocksForAnOrdinaryFullSender(t *testing.T) {
	s := &sender{
		cfg:           &senderConfig{maxQueued: 1},
		apiKeyManager: &apiKeyManager{apiKey: "plain-key"},
		queue:         make(chan *payload, 1),
		inflight:      atomic.NewInt32(0),
		enabled:       atomic.NewBool(true),
		statsd:        &statsd.NoOpClient{},
	}
	s.queue <- newPayload(nil)

	pushed := make(chan struct{})
	go func() {
		s.Push(newPayload(nil))
		close(pushed)
	}()

	select {
	case <-pushed:
		t.Fatal("a healthy full sender must block to apply backpressure, not drop")
	case <-time.After(100 * time.Millisecond):
	}
	<-s.queue // unblock the goroutine
	<-pushed
}
