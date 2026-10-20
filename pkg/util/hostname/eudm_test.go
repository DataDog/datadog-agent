// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"errors"
	"testing"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/stretchr/testify/require"
)

func TestEUDMResolution(t *testing.T) {
	oldGet := getEUDMHostname
	t.Cleanup(func() { getEUDMHostname = oldGet })
	for _, tc := range []struct {
		name, mode, override string
		file, sidecar, fail  bool
		want, provider       string
	}{
		{name: "device beats cloud and fqdn", mode: "end_user_device", want: "device-abc123", provider: eudmProviderName},
		{name: "explicit hostname", mode: "end_user_device", override: "explicit", want: "explicit", provider: hostnameinterface.ConfigProvider},
		{name: "hostname file", mode: "end_user_device", file: true, want: "from-file", provider: "hostnameFile"},
		{name: "non EUDM", mode: "host", want: "hostname-from-gce", provider: "gce"},
		{name: "missing serial", mode: "end_user_device", fail: true, want: "hostname-from-gce", provider: "gce"},
		{name: "sidecar", mode: "end_user_device", sidecar: true, want: "", provider: hostnameinterface.FargateProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupHostnameTest(t, testCase{GCE: true, azure: true, FQDN: true, OS: true, EC2: true, EC2Proritized: true, fargate: tc.sidecar})
			cfg := configmock.New(t)
			cfg.SetInTest("infrastructure_mode", tc.mode)
			cfg.SetInTest("hostname", tc.override)
			cfg.SetInTest("ec2_prefer_imdsv2", false)
			cfg.SetInTest("ec2_imdsv2_transition_payload_enabled", true)
			if tc.file {
				setupHostnameFile(t, "from-file")
			}
			getEUDMHostname = func() (string, error) {
				if tc.fail {
					return "", errors.New("missing serial")
				}
				return "device-abc123", nil
			}
			data, err := GetWithProvider(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, data.Hostname)
			require.Equal(t, tc.provider, data.Provider)
			if data.Provider == eudmProviderName {
				// Legacy resolution uses the provider chain and maintains its own cache.
				legacy, err := GetWithLegacyResolutionProvider(context.Background())
				require.NoError(t, err)
				require.Equal(t, data, legacy)
				getEUDMHostname = func() (string, error) {
					t.Fatal("identity was recollected despite cache")
					return "", nil
				}
				cached, err := GetWithProvider(context.Background())
				require.NoError(t, err)
				require.Equal(t, data, cached)
				cachedLegacy, err := GetWithLegacyResolutionProvider(context.Background())
				require.NoError(t, err)
				require.Equal(t, legacy, cachedLegacy)
			}
			cache.Cache.Delete(cache.BuildAgentKey("legacy_resolution_hostname"))
		})
	}
}

func TestEUDMSkippedOutsideEUDMMode(t *testing.T) {
	old := getEUDMHostname
	t.Cleanup(func() { getEUDMHostname = old })
	getEUDMHostname = func() (string, error) {
		t.Fatal("device identity was collected outside end_user_device mode")
		return "", nil
	}
	cfg := configmock.New(t)
	cfg.SetInTest("infrastructure_mode", "full")
	_, err := fromEUDM(context.Background(), "")
	require.Error(t, err)
}

func TestEUDMUnsupportedPlatform(t *testing.T) {
	old := getEUDMHostname
	t.Cleanup(func() { getEUDMHostname = old })
	getEUDMHostname = func() (string, error) { return "", errors.ErrUnsupported }
	cfg := configmock.New(t)
	cfg.SetInTest("infrastructure_mode", "end_user_device")
	_, err := fromEUDM(context.Background(), "")
	require.ErrorIs(t, err, errors.ErrUnsupported)
}
