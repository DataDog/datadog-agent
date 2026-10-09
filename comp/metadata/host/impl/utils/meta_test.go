// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestGetMeta(t *testing.T) {
	ctx := context.Background()
	cfg := config.NewMock(t)
	cfg.SetInTest("cloud_provider_metadata", []string{})

	meta := getMeta(ctx, cfg, hostnameimpl.NewHostnameService())
	assert.NotEmpty(t, meta.SocketHostname)
	assert.NotEmpty(t, meta.Timezones)
	assert.NotEmpty(t, meta.SocketFqdn)
}

func TestGetMetaFromCache(t *testing.T) {
	ctx := context.Background()
	cfg := config.NewMock(t)
	cfg.SetInTest("cloud_provider_metadata", []string{})

	cache.Cache.Set(metaCacheKey, &Meta{
		SocketHostname: "socket_test",
		Timezones:      []string{"tz_test"},
	}, cache.NoExpiration)

	m := GetMetaFromCache(ctx, cfg, hostnameimpl.NewHostnameService())
	assert.Equal(t, "socket_test", m.SocketHostname)
	assert.Equal(t, []string{"tz_test"}, m.Timezones)
}

// Embed the component to supply just the provider-aware lookup used by metadata.
type identityHostname struct {
	hostnameinterface.Component
	data hostnameinterface.Data
}

func (h identityHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return h.data, nil
}

func TestCanonicalEUDMMetadata(t *testing.T) {
	for _, tc := range []struct {
		provider string
		force    bool
		want     string
	}{
		{hostnameinterface.EUDMProvider, false, "ip-device-abc123"},
		{hostnameinterface.ConfigProvider, false, ""},
		{hostnameinterface.ConfigProvider, true, "ip-device-abc123"},
		{"os", true, ""},
	} {
		t.Run(tc.provider+fmt.Sprint(tc.force), func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetInTest("cloud_provider_metadata", []string{})
			cfg.SetInTest("ec2_use_dmi", false)
			cfg.SetInTest("ec2_imdsv2_transition_payload_enabled", false)
			cfg.SetInTest("hostname_force_config_as_canonical", tc.force)
			host := identityHostname{data: hostnameinterface.Data{Hostname: "ip-device-abc123", Provider: tc.provider}}
			metadata := getMeta(context.Background(), cfg, host)
			require.Equal(t, tc.want, metadata.AgentHostname)
			require.Empty(t, metadata.LegacyResolutionHostname)
			require.NotContains(t, metadata.HostAliases, "ip-device")
			encoded, err := json.Marshal(metadata)
			require.NoError(t, err)
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(encoded, &payload))
			if tc.want != "" {
				require.Equal(t, tc.want, payload["agent-hostname"])
			} else {
				require.NotContains(t, payload, "agent-hostname")
			}
		})
	}
}
