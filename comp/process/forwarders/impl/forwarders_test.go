// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwardersimpl

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

// Normal API keys flow through createParams without a provider — the resolver
// has only static key slots, no credential providers.
func TestCreateParamsWithNormalAPIKeys(t *testing.T) {
	cfg := configmock.New(t)
	endpoints := []apicfg.Endpoint{
		{APIKey: "plain-key-1", Endpoint: mustParseURL(t, "https://example.com"), ConfigSettingPath: "additional_endpoints"},
		{APIKey: "plain-key-2", Endpoint: mustParseURL(t, "https://example.com"), ConfigSettingPath: "additional_endpoints"},
	}
	opts, err := createParams(cfg, nil, 1024, endpoints, nil)
	require.NoError(t, err)
	assert.NotNil(t, opts)
}

// ENC[...] keys resolved by the secrets backend are normal static keys from
// the forwarder's perspective. The provider path must not interfere.
func TestCreateParamsWithResolvedEncKeys(t *testing.T) {
	cfg := configmock.New(t)
	endpoints := []apicfg.Endpoint{
		{APIKey: "resolved-enc-key-1", Endpoint: mustParseURL(t, "https://example.com"), ConfigSettingPath: "additional_endpoints"},
		{APIKey: "resolved-enc-key-2", Endpoint: mustParseURL(t, "https://example.com"), ConfigSettingPath: "additional_endpoints"},
	}
	opts, err := createParams(cfg, nil, 1024, endpoints, nil)
	require.NoError(t, err)
	assert.NotNil(t, opts)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}
