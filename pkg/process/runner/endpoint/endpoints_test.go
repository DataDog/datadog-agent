// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package endpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

func TestCheckAPIKeysResolved(t *testing.T) {
	for _, tc := range []struct {
		name    string
		keys    []string
		wantErr bool
	}{
		{name: "plain", keys: []string{"abcdef0123456789abcdef0123456789"}},
		{name: "handle", keys: []string{"ENC[api_key]"}, wantErr: true},
		{name: "handle with padding", keys: []string{" \tENC[api_key] "}, wantErr: true},
		{name: "handle in additional endpoint", keys: []string{"abcdef0123456789abcdef0123456789", "ENC[api_key]"}, wantErr: true},
		{name: "empty", keys: []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eps := make([]apicfg.Endpoint, 0, len(tc.keys))
			for _, k := range tc.keys {
				eps = append(eps, apicfg.Endpoint{APIKey: k, ConfigSettingPath: "api_key"})
			}

			err := CheckAPIKeysResolved(eps)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unresolved secret handle")
		})
	}
}

func TestCheckAPIKeysResolvedRejectsPendingDelegatedAuth(t *testing.T) {
	err := CheckAPIKeysResolved([]apicfg.Endpoint{{
		ConfigSettingPath:       "process_config.additional_endpoints",
		HasPendingDelegatedAuth: true,
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delegated-auth directive")
}

func TestGetAPIEndpointsHoldsPendingDelegatedAuthDirectives(t *testing.T) {
	cfg := pkgconfigmock.New(t)
	cfg.SetInTest("api_key", "00000000000000000000000000000000")
	cfg.SetInTest("process_config.additional_endpoints", map[string][]string{
		"https://second-org.datadoghq.com": {"DELA[org-uuid-1, aws]"},
		"https://third-org.datadoghq.com":  {"00000000000000000000000000000001", "DELA[org-uuid-2, aws]"},
	})

	eps, err := GetAPIEndpoints(cfg)
	require.NoError(t, err)

	var pendingSecond, pendingThird, realThird int
	for _, ep := range eps {
		assert.False(t, utils.IsDelaDirective(ep.APIKey), "a directive must never ship as an API key")
		switch ep.Endpoint.Host {
		case "second-org.datadoghq.com":
			if ep.HasPendingDelegatedAuth {
				pendingSecond++
				assert.Empty(t, ep.APIKey)
			} else {
				t.Fatal("a DELA-only domain must only carry the pending marker")
			}
		case "third-org.datadoghq.com":
			if ep.HasPendingDelegatedAuth {
				pendingThird++
			} else {
				realThird++
			}
		}
	}
	assert.Equal(t, 1, pendingSecond, "DELA-only domain keeps exactly one pending marker")
	assert.Equal(t, 1, pendingThird)
	assert.Equal(t, 1, realThird, "static key on a mixed domain is kept")

	// The pending flag must survive into the resolver input so the domain stays
	// alive until delegated auth writes the resolved key back.
	kpd := apicfg.KeysPerDomains(eps)
	assert.True(t, anyPendingDelegatedAuth(kpd["https://second-org.datadoghq.com"]))
	assert.True(t, anyPendingDelegatedAuth(kpd["https://third-org.datadoghq.com"]))
}

func anyPendingDelegatedAuth(keys []utils.APIKeys) bool {
	for _, k := range keys {
		if k.HasPendingDelegatedAuth {
			return true
		}
	}
	return false
}
