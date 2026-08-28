// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"net/http"
	"testing"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	h.Set("DD-Api-Key", p.key)
	return true
}

func resolverWithProvider(t *testing.T, staticKeys []string, p CredentialProvider) DomainResolver {
	t.Helper()
	r, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:   "https://example.com",
		APIKeySet: []utils.APIKeys{{ConfigSettingPath: "additional_endpoints", Keys: staticKeys}},
	})
	require.NoError(t, err)
	r.SetCredentialProviders([]CredentialProvider{p})
	return r
}

// A provider gets its own authorization slot even before it has a credential. Without the slot the
// forwarder would create no transaction for that destination and the payload would be dropped at
// creation, which is exactly what the buffering behaviour has to avoid.
func TestProviderGetsAnAuthorizationSlotWhileStillResolving(t *testing.T) {
	r := resolverWithProvider(t, []string{"static-key"}, &stubProvider{ready: false})

	assert.Len(t, r.GetAuthorizers(), 2,
		"a provider without a credential must still occupy a slot so a transaction is created for it")
}

// While the provider has no credential, Authorize must refuse rather than send a request with no
// (or someone else's) key.
func TestAuthorizeReportsNotReadyWhileResolving(t *testing.T) {
	r := resolverWithProvider(t, nil, &stubProvider{ready: false})
	log := logmock.New(t)

	h := http.Header{}
	err := r.Authorize(0, h, log)

	require.ErrorIs(t, err, ErrCredentialNotReady)
	assert.Empty(t, h, "no header may be stamped when the credential is not ready")
}

// Once the credential lands, the same slot starts authorizing - no rebuild, no new resolver.
func TestAuthorizeSucceedsOnceCredentialArrives(t *testing.T) {
	p := &stubProvider{key: "resolved-key"}
	r := resolverWithProvider(t, nil, p)
	log := logmock.New(t)

	require.ErrorIs(t, r.Authorize(0, http.Header{}, log), ErrCredentialNotReady)

	p.ready = true

	h := http.Header{}
	require.NoError(t, r.Authorize(0, h, log))
	assert.Equal(t, "resolved-key", h.Get("DD-Api-Key"))
}

// Static keys and providers coexist on one domain: the static slots keep working regardless of
// whether the provider has resolved. Providers come first in the authorizer list, so the
// provider is at index 0 and the static key at index 1.
func TestStaticKeysUnaffectedByAPendingProvider(t *testing.T) {
	r := resolverWithProvider(t, []string{"static-key"}, &stubProvider{ready: false})
	log := logmock.New(t)

	require.ErrorIs(t, r.Authorize(0, http.Header{}, log), ErrCredentialNotReady,
		"the provider slot (index 0) must refuse while pending")

	h := http.Header{}
	require.NoError(t, r.Authorize(1, h, log), "the static slot (index 1) must authorize independently")
	assert.Equal(t, "static-key", h.Get("DD-Api-Key"))
}

// An out-of-range slot is an error rather than a silent unauthenticated send. On-disk transactions
// serialized when more slots existed can land here after a restart.
func TestAuthorizeRejectsOutOfRangeSlot(t *testing.T) {
	r := resolverWithProvider(t, []string{"static-key"}, &stubProvider{ready: true})
	log := logmock.New(t)

	h := http.Header{}
	require.Error(t, r.Authorize(99, h, log))
	assert.Empty(t, h)
}

// An ENC[...] key resolved by the secrets backend is a normal static key from the resolver's
// perspective. The provider path must not interfere with it.
func TestResolvedEncKeyUnaffectedByProvider(t *testing.T) {
	r := resolverWithProvider(t, []string{"resolved-enc-key"}, &stubProvider{ready: false})
	log := logmock.New(t)

	h := http.Header{}
	require.NoError(t, r.Authorize(1, h, log), "the ENC[] key slot must authorize independently of the provider")
	assert.Equal(t, "resolved-enc-key", h.Get("DD-Api-Key"))
}
