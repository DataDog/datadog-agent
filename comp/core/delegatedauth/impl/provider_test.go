// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
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

func TestProviderUsesFallbackAfterRejectedCredentialRefreshFails(t *testing.T) {
	p := newInstanceProvider()
	p.setRefreshTrigger(make(chan struct{}, 1))
	p.setResolved("rejected-key")

	require.True(t, p.Refresh())
	require.True(t, p.setFallback("static-fallback"))

	h := http.Header{}
	require.True(t, p.Authorize(h))
	assert.Equal(t, "static-fallback", h.Get(apiKeyHeader))
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

// Two orgs dual-shipping to one domain is the case this feature exists for. Looking a provider up
// by destination alone cannot tell them apart, so consumers that own one directive each must be
// able to find their own credential - otherwise both ship under the first org's key and the second
// org receives nothing.
func TestProviderForDirectiveDistinguishesTwoOrgsOnOneDestination(t *testing.T) {
	d := &delegatedAuthComponent{}
	const key, dest = "additional_endpoints", "https://app.datadoghq.com"

	orgA := newInstanceProvider()
	orgB := newInstanceProvider()
	orgA.setResolved("org-a-key")
	orgB.setResolved("org-b-key")

	d.registerProvider(delegatedauth.InstanceParams{
		APIKeyConfigKey: "org-a", ConfigKey: key, Destination: dest, Directive: "DELA(org-a, aws)",
	}, orgA)
	d.registerProvider(delegatedauth.InstanceParams{
		APIKeyConfigKey: "org-b", ConfigKey: key, Destination: dest, Directive: "DELA(org-b, aws)",
	}, orgB)

	for _, tc := range []struct{ directive, want string }{
		{"DELA(org-a, aws)", "org-a-key"},
		{"DELA(org-b, aws)", "org-b-key"},
	} {
		p := d.ProviderForDirective(key, dest, tc.directive)
		require.NotNil(t, p, "each directive must resolve to its own instance")

		h := http.Header{}
		require.True(t, p.Authorize(h))
		assert.Equal(t, tc.want, h.Get("DD-Api-Key"), "directive %q got the wrong org's credential", tc.directive)
	}

	// The forwarder still sees both, because it fans one payload out to every slot on the domain.
	assert.Len(t, d.ProvidersFor(key, dest), 2)
}

// An unknown directive must resolve to nothing so the caller refuses to send, rather than falling
// back to some other org's credential that happens to share the destination.
func TestProviderForDirectiveReturnsNilForAnUnregisteredDirective(t *testing.T) {
	d := &delegatedAuthComponent{}
	d.registerProvider(delegatedauth.InstanceParams{
		APIKeyConfigKey: "org-a", ConfigKey: "additional_endpoints", Destination: "https://app.datadoghq.com", Directive: "DELA(org-a, aws)",
	}, newInstanceProvider())

	assert.Nil(t, d.ProviderForDirective("additional_endpoints", "https://app.datadoghq.com", "DELA(org-z, aws)"))
}

func TestReplaceInstanceRemovesOldProviderRoute(t *testing.T) {
	const instanceKey = "additional_endpoints[old][org]"
	oldDone := make(chan struct{})
	close(oldDone)
	oldProvider := newInstanceProvider()
	oldProvider.setResolved("old-key")
	d := &delegatedAuthComponent{
		instances: map[string]*authInstance{instanceKey: {
			credProvider: oldProvider,
			done:         oldDone,
		}},
		providers: make(map[providerKey][]registeredProvider),
	}
	d.registerProvider(delegatedauth.InstanceParams{
		APIKeyConfigKey: instanceKey,
		ConfigKey:       "additional_endpoints",
		Destination:     "https://old.datadoghq.com",
		Directive:       "DELA(old-org, aws)",
	}, oldProvider)
	require.Same(t, oldProvider, d.ProviderForDirective("additional_endpoints", "https://old.datadoghq.com", "DELA(old-org, aws)"))

	require.NoError(t, d.replaceInstance(context.Background(), instanceKey, &authInstance{done: make(chan struct{})}))
	assert.Nil(t, d.ProviderForDirective("additional_endpoints", "https://old.datadoghq.com", "DELA(old-org, aws)"))
	assert.False(t, oldProvider.Authorize(http.Header{}))

	newProvider := newInstanceProvider()
	d.registerProvider(delegatedauth.InstanceParams{
		APIKeyConfigKey: instanceKey,
		ConfigKey:       "additional_endpoints",
		Destination:     "https://new.datadoghq.com",
		Directive:       "DELA(new-org, aws)",
	}, newProvider)
	require.Same(t, newProvider, d.ProviderForDirective("additional_endpoints", "https://new.datadoghq.com", "DELA(new-org, aws)"))
}

func TestCanceledReplacementInvalidatesOldProvider(t *testing.T) {
	const instanceKey = "additional_endpoints[org-a]"
	config := mock.New(t)
	config.SetDefault("api_key", "original-key")
	d := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
		providers: make(map[providerKey][]registeredProvider),
		config:    config,
	}
	oldProvider := newInstanceProvider()
	oldProvider.setResolved("old-key")
	old := &authInstance{
		apiKeyConfigKey: "api_key",
		credProvider:    oldProvider,
		fallbackAPIKey:  "fallback-key",
		refreshCancel:   func() {},
		done:            make(chan struct{}),
	}
	d.instances[instanceKey] = old
	d.registerProvider(delegatedauth.InstanceParams{
		APIKeyConfigKey: instanceKey,
		ConfigKey:       "additional_endpoints",
		Destination:     "https://old.datadoghq.com",
		Directive:       "DELA(old-org, aws)",
	}, oldProvider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	replacementDone := make(chan struct{})
	require.ErrorIs(t, d.replaceInstance(ctx, instanceKey, &authInstance{
		refreshCancel: func() {},
		done:          replacementDone,
	}), context.Canceled)

	assert.Nil(t, d.ProviderForDirective("additional_endpoints", "https://old.datadoghq.com", "DELA(old-org, aws)"))
	assert.False(t, oldProvider.Authorize(http.Header{}))
	oldProvider.setResolved("late-key")
	assert.False(t, oldProvider.Authorize(http.Header{}))
	assert.False(t, oldProvider.Refresh())
	d.deliverFallbackAPIKey(old)
	assert.Equal(t, "original-key", config.GetString("api_key"))
	assert.Same(t, old, d.instances[instanceKey])
	select {
	case <-replacementDone:
	default:
		t.Fatal("replacement was not stopped")
	}
}

func TestCanceledReplacementWithStoppedInstanceIsNotPublished(t *testing.T) {
	oldDone := make(chan struct{})
	close(oldDone)
	oldProvider := newInstanceProvider()
	oldProvider.setResolved("old-key")
	d := &delegatedAuthComponent{
		instances: map[string]*authInstance{"api_key": {
			credProvider:  oldProvider,
			refreshCancel: func() {},
			done:          oldDone,
		}},
		providers: make(map[providerKey][]registeredProvider),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	replacementDone := make(chan struct{})
	require.ErrorIs(t, d.replaceInstance(ctx, "api_key", &authInstance{
		refreshCancel: func() {},
		done:          replacementDone,
	}), context.Canceled)

	assert.NotNil(t, d.instances["api_key"])
	assert.False(t, oldProvider.Authorize(http.Header{}))
}

func TestInvalidatedProviderRejectsLateConfigDelivery(t *testing.T) {
	config := mock.New(t)
	config.SetDefault("api_key", "original-key")
	provider := newInstanceProvider()
	instance := &authInstance{
		apiKeyConfigKey: "api_key",
		credProvider:    provider,
	}
	d := &delegatedAuthComponent{config: config}

	provider.invalidate()
	d.deliverAPIKey(instance, "late-key")

	assert.Equal(t, "original-key", config.GetString("api_key"))
	assert.False(t, provider.Authorize(http.Header{}))
}

// Refresh returns false when no background refresh goroutine is running (no trigger channel),
// so the caller knows to drop the transaction rather than reschedule it.
func TestProviderRefreshReturnsFalseWithoutTrigger(t *testing.T) {
	p := newInstanceProvider()
	assert.False(t, p.Refresh(), "Refresh must return false when no background goroutine is configured")
}

// Refresh resets the credential to buffering so Authorize returns false — no further sends under
// the stale key — and sends a non-blocking signal to the trigger channel. A burst of calls
// coalesces: the channel has capacity 1, so a second Refresh while the first signal is still
// pending is silently dropped.
func TestProviderRefreshResetsToBufferingAndSignals(t *testing.T) {
	p := newInstanceProvider()
	p.setResolved("stale-key")
	p.setRefreshTrigger(make(chan struct{}, 1))

	// Before Refresh, the provider authorizes.
	h := http.Header{}
	require.True(t, p.Authorize(h))
	assert.Equal(t, "stale-key", h.Get(apiKeyHeader))

	// After Refresh, it buffers and the trigger channel has one pending signal.
	require.True(t, p.Refresh())
	h = http.Header{}
	assert.False(t, p.Authorize(h), "Authorize must return false after Refresh resets to buffering")
	assert.Empty(t, h)

	// A second Refresh coalesces: the channel is full, so the non-blocking send is dropped.
	require.True(t, p.Refresh())

	// Exactly one signal in the channel, not two.
	select {
	case <-p.refreshTrigger:
		// expected: the first signal
	default:
		t.Fatal("Refresh must send to the trigger channel")
	}
	select {
	case <-p.refreshTrigger:
		t.Fatal("second Refresh must not add a second signal to a full channel")
	default:
		// expected: channel is empty, the second Refresh was dropped
	}
}

func TestRefreshForTargetsMatchingCredential(t *testing.T) {
	d := &delegatedAuthComponent{providers: make(map[providerKey][]registeredProvider)}
	first := newInstanceProvider()
	first.setResolved("first-key")
	firstTrigger := make(chan struct{}, 1)
	first.setRefreshTrigger(firstTrigger)
	second := newInstanceProvider()
	second.setResolved("second-key")
	secondTrigger := make(chan struct{}, 1)
	second.setRefreshTrigger(secondTrigger)

	params := delegatedauth.InstanceParams{APIKeyConfigKey: "first", ConfigKey: "additional_endpoints", Destination: "https://example.com"}
	d.registerProvider(params, first)
	params.APIKeyConfigKey = "second"
	params.Directive = "DELA(second, aws)"
	d.registerProvider(params, second)

	require.True(t, d.RefreshFor("additional_endpoints", "https://example.com", "second-key"))
	assert.Empty(t, firstTrigger)
	assert.Len(t, secondTrigger, 1)
	assert.False(t, d.RefreshFor("additional_endpoints", "https://example.com", "unknown-key"))
}
