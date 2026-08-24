// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A provider that has never resolved must report "no credential" so the caller buffers. Shipping
// under no credential, or silently under the fallback before an attempt has even failed, are both
// wrong: the fallback is a failure path, not a startup path.
func TestProviderBuffersBeforeFirstExchange(t *testing.T) {
	p := newInstanceProvider()

	h := http.Header{}
	assert.False(t, p.Authorize(h), "must not authorize before the first exchange completes")
	assert.Empty(t, h, "must not stamp any header when it cannot authorize")
	assert.False(t, p.hasCredential())
}

func TestProviderAuthorizesOnceResolved(t *testing.T) {
	p := newInstanceProvider()
	p.setResolved("resolved-key")

	h := http.Header{}
	require.True(t, p.Authorize(h))
	assert.Equal(t, "resolved-key", h.Get(apiKeyHeader))
}

// The fallback is applied only after an exchange has actually failed - that is what setFallback
// being called from the failure paths encodes. Before that the provider buffers even though a
// fallback is configured.
func TestProviderUsesFallbackOnlyAfterFailure(t *testing.T) {
	p := newInstanceProvider()

	h := http.Header{}
	require.False(t, p.Authorize(h), "a configured fallback must not be used before a failure")

	p.setFallback("static-fallback")

	h = http.Header{}
	require.True(t, p.Authorize(h))
	assert.Equal(t, "static-fallback", h.Get(apiKeyHeader))
}

// With no fallback configured, a failure leaves the provider buffering rather than authorizing
// with nothing. Retries continue in the background.
func TestProviderKeepsBufferingWhenFailureHasNoFallback(t *testing.T) {
	p := newInstanceProvider()
	p.setFallback("")

	h := http.Header{}
	assert.False(t, p.Authorize(h))
	assert.Empty(t, h)
}

// A later successful refresh must replace the fallback: the real credential always wins.
func TestProviderResolvedReplacesFallback(t *testing.T) {
	p := newInstanceProvider()
	p.setFallback("static-fallback")
	p.setResolved("resolved-key")

	h := http.Header{}
	require.True(t, p.Authorize(h))
	assert.Equal(t, "resolved-key", h.Get(apiKeyHeader))
}

// A transient refresh failure after a successful resolve must NOT regress to the fallback - the
// last known-good credential is better than a static key the operator supplied as a safety net.
func TestProviderFallbackDoesNotRegressAResolvedCredential(t *testing.T) {
	p := newInstanceProvider()
	p.setResolved("resolved-key")
	p.setFallback("static-fallback")

	h := http.Header{}
	require.True(t, p.Authorize(h))
	assert.Equal(t, "resolved-key", h.Get(apiKeyHeader),
		"a refresh failure must keep the last resolved key, not fall back")
}

// Authorize runs on the request path while the refresh goroutine swaps credentials. Exercised
// under -race in CI; without the atomic swap this is a data race.
func TestProviderConcurrentAuthorizeAndUpdate(t *testing.T) {
	p := newInstanceProvider()

	var wg sync.WaitGroup
	done := make(chan struct{})

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					p.Authorize(http.Header{})
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for i := range 500 {
			if i%2 == 0 {
				p.setResolved("k")
			} else {
				p.setFallback("f")
			}
		}
	}()

	wg.Wait()
	assert.True(t, p.hasCredential())
}
