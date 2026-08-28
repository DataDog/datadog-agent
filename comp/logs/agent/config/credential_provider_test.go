// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// stubProvider is a CredentialProvider whose readiness the test controls.
type stubProvider struct {
	key   string
	ready bool
}

func (p *stubProvider) Authorize(h http.Header) bool {
	if !p.ready {
		return false
	}
	h.Set("DD-API-KEY", p.key)
	return true
}

func (p *stubProvider) Refresh() bool { return false }

// An ordinary endpoint must be completely unaffected by any of this.
func TestAuthorizeStampsTheConfiguredKey(t *testing.T) {
	e := NewEndpoint("plain-key", "logs_config.api_key", "host", 0, "", false)

	h := http.Header{}
	require.True(t, e.Authorize(h))
	assert.Equal(t, "plain-key", h.Get("DD-API-KEY"))
}

// A provider replaces the API key entirely: a delegated-auth endpoint has no key of its own.
func TestAuthorizeUsesTheProviderWhenSet(t *testing.T) {
	e := NewEndpoint("", "logs_config.additional_endpoints", "host", 0, "", false)
	e.SetCredentialProvider(&stubProvider{key: "delegated-key", ready: true})

	h := http.Header{}
	require.True(t, e.Authorize(h))
	assert.Equal(t, "delegated-key", h.Get("DD-API-KEY"))
}

// While the credential is resolving the endpoint must refuse, so the destination retries instead
// of shipping the payload with no key.
func TestAuthorizeRefusesWhileTheCredentialResolves(t *testing.T) {
	e := NewEndpoint("", "logs_config.additional_endpoints", "host", 0, "", false)
	e.SetCredentialProvider(&stubProvider{key: "delegated-key"})

	h := http.Header{}
	assert.False(t, e.Authorize(h))
	assert.Empty(t, h, "nothing may be stamped without a credential")
}

// An endpoint built from a directive that never produced an instance - an unsupported cloud
// provider, or a subsystem with no provider lookup wired - has no key and no provider. It must
// refuse rather than stamp the empty string, which would reach the intake unauthenticated.
func TestAuthorizeRefusesForADirectiveWithNoInstance(t *testing.T) {
	e := NewEndpoint("", "logs_config.additional_endpoints", "host", 0, "", false)
	e.credentialDirective = "DELA(org-uuid-1, aws)"

	h := http.Header{}
	assert.False(t, e.Authorize(h))
	assert.Empty(t, h)
}

// A genuinely empty API key with no directive keeps its long-standing behaviour, so this change
// does not start silently dropping traffic for an unrelated misconfiguration.
func TestAuthorizeStillAllowsAnEmptyKeyWithoutADirective(t *testing.T) {
	e := NewEndpoint("", "logs_config.api_key", "host", 0, "", false)

	assert.True(t, e.Authorize(http.Header{}))
}

func TestIsDelaDirective(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"DELA(org, aws)", true},
		{"   DELA(org, aws)  ", true},
		{"dela(org, aws)", false},
		{"abcdef0123456789", false},
		{"", false},
	} {
		assert.Equal(t, tc.want, isDelaDirective(tc.value), "isDelaDirective(%q)", tc.value)
	}
}

// buildAdditionalWithDirective loads one additional endpoint whose api_key is a DELA directive.
func buildAdditionalWithDirective(t *testing.T, lookup CredentialProviderLookup) Endpoint {
	t.Helper()
	cfg := mock.New(t)
	cfg.SetInTest("logs_config.additional_endpoints", []map[string]interface{}{
		{"host": "org2.datadoghq.com", "api_key": "DELA(org-uuid-2, aws)"},
	})

	keys := defaultLogsConfigKeys(cfg)
	if lookup != nil {
		keys = keys.WithCredentialProviders(lookup)
	}

	endpoints := loadHTTPAdditionalEndpoints(newHTTPEndpoint(keys, false), keys, "", "", "", false)
	require.Len(t, endpoints, 1)
	return endpoints[0]
}

// The directive is a placeholder, never a credential. Before this change it was stored verbatim as
// the API key and stamped onto every request, sending the literal text "DELA(org-uuid-2, aws)" to
// that org's intake in the DD-API-KEY header.
func TestDirectiveIsNeverUsedAsAnAPIKey(t *testing.T) {
	e := buildAdditionalWithDirective(t, nil)

	assert.Empty(t, e.GetAPIKey(), "the directive must not become the endpoint's API key")

	h := http.Header{}
	assert.False(t, e.Authorize(h), "with no provider wired the endpoint must not send at all")
	assert.Empty(t, h.Get("DD-API-KEY"))
}

// With a lookup wired, the endpoint gets its own credential and starts authorizing once it lands.
func TestDirectiveTakesItsCredentialFromTheLookup(t *testing.T) {
	p := &stubProvider{key: "org2-key"}
	var gotConfigKey, gotHost, gotDirective string

	e := buildAdditionalWithDirective(t, func(configKey, host, directive string) CredentialProvider {
		gotConfigKey, gotHost, gotDirective = configKey, host, directive
		return p
	})

	// The lookup is keyed on the directive, not just the host, so two orgs sharing a host each get
	// their own credential.
	assert.Equal(t, "logs_config.additional_endpoints", gotConfigKey)
	assert.Equal(t, "org2.datadoghq.com", gotHost)
	assert.Equal(t, "DELA(org-uuid-2, aws)", gotDirective)

	assert.False(t, e.Authorize(http.Header{}), "must buffer until the credential arrives")

	p.ready = true
	h := http.Header{}
	require.True(t, e.Authorize(h))
	assert.Equal(t, "org2-key", h.Get("DD-API-KEY"))
}

// An ordinary additional endpoint must not be routed through the lookup at all.
func TestPlainAdditionalEndpointIgnoresTheLookup(t *testing.T) {
	cfg := mock.New(t)
	cfg.SetInTest("logs_config.additional_endpoints", []map[string]interface{}{
		{"host": "other.datadoghq.com", "api_key": "a-real-key"},
	})
	called := false
	keys := defaultLogsConfigKeys(cfg).WithCredentialProviders(
		func(_, _, _ string) CredentialProvider { called = true; return nil })

	endpoints := loadHTTPAdditionalEndpoints(newHTTPEndpoint(keys, false), keys, "", "", "", false)
	require.Len(t, endpoints, 1)

	assert.False(t, called, "a plain API key must not consult the provider lookup")
	h := http.Header{}
	require.True(t, endpoints[0].Authorize(h))
	assert.Equal(t, "a-real-key", h.Get("DD-API-KEY"))
}

// The TCP framing prefixes every log line with the API key verbatim, and there is no Authorize
// gate on that path. A directive there would be written to the wire in cleartext, including any
// fallback=<real key> it carries, so the endpoint must not be built at all.
//
// This is the default path for this feature, not an edge case: shouldUseTCP() returns true as soon
// as logs_config.additional_endpoints is set, so TCP is chosen unless HTTP is explicitly forced.
func TestTCPAdditionalEndpointDropsADirectiveRatherThanPuttingItOnTheWire(t *testing.T) {
	cfg := mock.New(t)
	cfg.SetInTest("logs_config.additional_endpoints", []map[string]interface{}{
		{"host": "org2.datadoghq.com", "port": 10516, "api_key": "DELA(org-uuid-2, aws, fallback=abcdef0123456789abcdef0123456789)"},
		{"host": "org3.datadoghq.com", "port": 10516, "api_key": "a-real-key"},
	})
	keys := defaultLogsConfigKeys(cfg)

	endpoints := loadTCPAdditionalEndpoints(newTCPEndpoint(keys, false), keys, false)

	require.Len(t, endpoints, 1, "the delegated-auth endpoint must be dropped, the plain one kept")
	assert.Equal(t, "org3.datadoghq.com", endpoints[0].Host)
	for _, e := range endpoints {
		assert.NotContains(t, e.GetAPIKey(), "DELA(", "a directive must never become a TCP prefix")
		assert.NotContains(t, e.GetAPIKey(), "abcdef0123456789", "the fallback key must never reach the wire")
	}
}

// SkipConfigWriteback leaves the directive in the config tree, so the rotation callback re-reads
// it on every update to additional_endpoints. If that value ever lands in apiKey, Authorize must
// still refuse: the directive is not a credential, and stamping it would send the org UUID and any
// fallback=<real key> it carries to the intake.
//
// Asserted against apiKey directly rather than by driving a config update, because the config
// mock's SetInTest does not fire OnUpdate - a test written that way passes whether or not the
// protection exists.
func TestADirectiveInTheAPIKeyIsStillNeverStamped(t *testing.T) {
	e := NewEndpoint("", "logs_config.additional_endpoints", "org2.datadoghq.com", 0, "", false)
	e.credentialDirective = "DELA(org-uuid-2, aws, fallback=abcdef0123456789abcdef0123456789)"

	// Simulate the rotation callback having written the raw config value back into the key.
	e.apiKey.Store("DELA(org-uuid-2, aws, fallback=abcdef0123456789abcdef0123456789)")

	h := http.Header{}
	assert.False(t, e.Authorize(h), "an endpoint with a directive and no provider must never send")
	assert.Empty(t, h.Get("DD-API-KEY"))
}

// A resolved provider still wins over whatever apiKey happens to hold.
func TestProviderStillWinsWhenTheKeyHoldsADirective(t *testing.T) {
	e := NewEndpoint("", "logs_config.additional_endpoints", "org2.datadoghq.com", 0, "", false)
	e.credentialDirective = "DELA(org-uuid-2, aws)"
	e.apiKey.Store("DELA(org-uuid-2, aws)")
	e.SetCredentialProvider(&stubProvider{key: "org2-key", ready: true})

	h := http.Header{}
	require.True(t, e.Authorize(h))
	assert.Equal(t, "org2-key", h.Get("DD-API-KEY"))
}

// An endpoint with an ENC[...] key resolved by the secrets backend must be unaffected by the
// provider path. It behaves like a plain key from the endpoint's perspective.
func TestAuthorizeStampsResolvedEncKey(t *testing.T) {
	e := NewEndpoint("resolved-enc-key", "logs_config.api_key", "host", 0, "", false)

	h := http.Header{}
	require.True(t, e.Authorize(h))
	assert.Equal(t, "resolved-enc-key", h.Get("DD-API-KEY"))
}
