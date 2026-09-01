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

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
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
		ConfigKey: key, Destination: dest, Directive: "DELA(org-a, aws)",
	}, orgA)
	d.registerProvider(delegatedauth.InstanceParams{
		ConfigKey: key, Destination: dest, Directive: "DELA(org-b, aws)",
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
		ConfigKey: "additional_endpoints", Destination: "https://app.datadoghq.com", Directive: "DELA(org-a, aws)",
	}, newInstanceProvider())

	assert.Nil(t, d.ProviderForDirective("additional_endpoints", "https://app.datadoghq.com", "DELA(org-z, aws)"))
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

// Matching directives share one provider instead of starting duplicate refresh loops.
func TestCredentialCacheSharesMatchingLifecycle(t *testing.T) {
	d := &delegatedAuthComponent{
		instances:       make(map[string]*authInstance),
		providers:       make(map[providerKey][]registeredProvider),
		credentialCache: make(map[credentialCacheKey]*instanceProvider),
	}

	orgA := newInstanceProvider()
	orgA.setResolved("shared-key-for-org-a")
	params := delegatedauth.InstanceParams{OrgUUID: "org-a", RefreshInterval: 60}
	providerConfig := &cloudauthconfig.AWSProviderConfig{Region: "us-east-1"}
	cacheKey := newCredentialCacheKey(params, "https://app.datadoghq.com", providerConfig)
	d.credentialCache[cacheKey] = orgA

	// A second directive for the same org+site should reuse orgA's provider, not create a new one.
	d.registerProvider(delegatedauth.InstanceParams{
		ConfigKey: "additional_endpoints", Destination: "https://app.datadoghq.com", Directive: "DELA(org-a, aws)",
	}, orgA)
	d.registerProvider(delegatedauth.InstanceParams{
		ConfigKey: "apm_config.additional_endpoints", Destination: "https://app.datadoghq.com", Directive: "DELA(org-a, aws)",
	}, orgA)

	// Both config keys should find the same provider.
	p1 := d.ProvidersFor("additional_endpoints", "https://app.datadoghq.com")
	p2 := d.ProvidersFor("apm_config.additional_endpoints", "https://app.datadoghq.com")
	require.Len(t, p1, 1)
	require.Len(t, p2, 1)

	// Same underlying provider — one WIF exchange, one goroutine.
	h1, h2 := http.Header{}, http.Header{}
	require.True(t, p1[0].Authorize(h1))
	require.True(t, p2[0].Authorize(h2))
	assert.Equal(t, "shared-key-for-org-a", h1.Get(apiKeyHeader))
	assert.Equal(t, "shared-key-for-org-a", h2.Get(apiKeyHeader))

	// A different org gets its own provider.
	orgB := newInstanceProvider()
	orgB.setResolved("org-b-key")
	d.credentialCache[newCredentialCacheKey(delegatedauth.InstanceParams{OrgUUID: "org-b", RefreshInterval: 60}, "https://app.datadoghq.com", providerConfig)] = orgB
	d.registerProvider(delegatedauth.InstanceParams{
		ConfigKey: "additional_endpoints", Destination: "https://app.datadoghq.com", Directive: "DELA(org-b, aws)",
	}, orgB)

	providers := d.ProvidersFor("additional_endpoints", "https://app.datadoghq.com")
	assert.Len(t, providers, 2, "two different orgs on the same domain should have two providers")
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

	params := delegatedauth.InstanceParams{ConfigKey: "additional_endpoints", Destination: "https://example.com"}
	d.registerProvider(params, first)
	params.Directive = "DELA(second, aws)"
	d.registerProvider(params, second)

	require.True(t, d.RefreshFor("additional_endpoints", "https://example.com", "second-key"))
	assert.Empty(t, firstTrigger)
	assert.Len(t, secondTrigger, 1)
	assert.False(t, d.RefreshFor("additional_endpoints", "https://example.com", "unknown-key"))
}

func TestCredentialCacheKeyIncludesLifecycleConfiguration(t *testing.T) {
	base := delegatedauth.InstanceParams{OrgUUID: "org-a", RefreshInterval: 60, FallbackAPIKey: "fallback-a"}
	baseConfig := &cloudauthconfig.AWSProviderConfig{Region: "us-east-1"}
	baseKey := newCredentialCacheKey(base, "https://app.datadoghq.com", baseConfig)

	regionKey := newCredentialCacheKey(base, "https://app.datadoghq.com", &cloudauthconfig.AWSProviderConfig{Region: "eu-west-1"})
	assert.NotEqual(t, baseKey, regionKey)

	refreshParams := base
	refreshParams.RefreshInterval = 30
	assert.NotEqual(t, baseKey, newCredentialCacheKey(refreshParams, "https://app.datadoghq.com", baseConfig))

	fallbackParams := base
	fallbackParams.FallbackAPIKey = "fallback-b"
	assert.NotEqual(t, baseKey, newCredentialCacheKey(fallbackParams, "https://app.datadoghq.com", baseConfig))
}
