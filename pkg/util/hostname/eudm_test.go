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
	oldGet, oldLegacy := getEUDMHostname, ec2GetLegacyResolutionInstanceID
	t.Cleanup(func() { getEUDMHostname, ec2GetLegacyResolutionInstanceID = oldGet, oldLegacy })
	ec2GetLegacyResolutionInstanceID = func(context.Context) (string, error) { return "hostname-from-ec2", nil }
	for _, tc := range []struct {
		name, mode, override    string
		file, sidecar, fail     bool
		useSerial               bool
		gce, azure, ec2Priority bool
		fqdn, container         bool
		deviceName              string
		want, provider          string
	}{
		{name: "serial disabled", mode: "end_user_device", want: "device", provider: eudmProviderName},
		{name: "device beats OS fallback", mode: "end_user_device", useSerial: true, want: "device-abc123", provider: eudmProviderName},
		{name: "explicit hostname", mode: "end_user_device", useSerial: true, override: "explicit", want: "explicit", provider: hostnameinterface.ConfigProvider},
		{name: "hostname file", mode: "end_user_device", useSerial: true, file: true, want: "from-file", provider: "hostnameFile"},
		{name: "non EUDM", fqdn: true, mode: "host", useSerial: true, want: "hostname-from-fqdn", provider: "fqdn"},
		{name: "missing serial", fqdn: true, mode: "end_user_device", useSerial: true, fail: true, want: "hostname-from-fqdn", provider: "fqdn"},
		{name: "FQDN wins without serial", mode: "end_user_device", fqdn: true, want: "hostname-from-fqdn", provider: "fqdn"},
		{name: "FQDN wins with serial", mode: "end_user_device", fqdn: true, useSerial: true, want: "hostname-from-fqdn", provider: "fqdn"},
		{name: "container wins", mode: "end_user_device", container: true, useSerial: true, want: "container-host", provider: "container"},
		{name: "missing serial falls back to OS", mode: "end_user_device", useSerial: true, fail: true, want: "hostname-from-os", provider: "os"},
		{name: "GCE wins", mode: "end_user_device", useSerial: true, gce: true, want: "hostname-from-gce", provider: "gce"},
		{name: "Azure wins", mode: "end_user_device", useSerial: true, azure: true, want: "hostname-from-azure", provider: "azure"},
		{name: "EC2 prioritized without serial", mode: "end_user_device", ec2Priority: true, want: "hostname-from-ec2", provider: "aws"},
		{name: "EC2 prioritized with serial", mode: "end_user_device", useSerial: true, ec2Priority: true, want: "hostname-from-ec2", provider: "aws"},
		{name: "EC2 default prefix", mode: "end_user_device", useSerial: true, deviceName: "ip-device", want: "hostname-from-ec2", provider: "aws"},
		{name: "EC2 Windows prefix", mode: "end_user_device", useSerial: true, deviceName: "ec2amaz-device", want: "hostname-from-ec2", provider: "aws"},
		{name: "sidecar", mode: "end_user_device", useSerial: true, sidecar: true, want: "", provider: hostnameinterface.FargateProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupHostnameTest(t, testCase{GCE: tc.gce, azure: tc.azure, FQDN: tc.fqdn, OS: true, EC2: true, EC2Proritized: tc.ec2Priority, fargate: tc.sidecar})
			oldContainer := containerProvider.cb
			t.Cleanup(func() { containerProvider.cb = oldContainer })
			containerProvider.cb = func(_ context.Context, _ string) (string, error) {
				if tc.container {
					return "container-host", nil
				}
				return "", errors.New("not containerized")
			}
			cfg := configmock.New(t)
			cfg.SetInTest("infrastructure_mode", tc.mode)
			cfg.SetInTest("hostname", tc.override)
			cfg.SetInTest("hostname_use_serial", tc.useSerial)
			cfg.SetInTest("hostname_fqdn", tc.fqdn)
			cfg.SetInTest("ec2_prioritize_instance_id_as_hostname", tc.ec2Priority)
			cfg.SetInTest("ec2_use_windows_prefix_detection", true)
			cfg.SetInTest("ec2_prefer_imdsv2", false)
			cfg.SetInTest("ec2_imdsv2_transition_payload_enabled", true)
			if tc.file {
				setupHostnameFile(t, "from-file")
			}
			getEUDMHostname = func(useSerial bool) (string, error) {
				require.Equal(t, tc.useSerial, useSerial)
				if tc.fail {
					return "", errors.New("missing serial")
				}
				name := tc.deviceName
				if name == "" {
					name = "device"
				}
				if useSerial {
					name += "-abc123"
				}
				return name, nil
			}
			data, err := GetWithProvider(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, data.Hostname)
			require.Equal(t, tc.provider, data.Provider)
			if data.Provider == eudmProviderName || data.Provider == "aws" {
				// Legacy resolution uses the provider chain and maintains its own cache.
				legacy, err := GetWithLegacyResolutionProvider(context.Background())
				require.NoError(t, err)
				require.Equal(t, data, legacy)
				getEUDMHostname = func(_ bool) (string, error) {
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
	getEUDMHostname = func(_ bool) (string, error) {
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
	getEUDMHostname = func(_ bool) (string, error) { return "", errors.ErrUnsupported }
	cfg := configmock.New(t)
	cfg.SetInTest("infrastructure_mode", "end_user_device")
	_, err := fromEUDM(context.Background(), "")
	require.ErrorIs(t, err, errors.ErrUnsupported)
}

func TestEUDMSerialConfig(t *testing.T) {
	for _, value := range []string{"", "false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DD_HOSTNAME_USE_SERIAL", value)
			cfg := configmock.New(t)
			cfg.SetInTest("infrastructure_mode", "end_user_device")
			old := getEUDMHostname
			t.Cleanup(func() { getEUDMHostname = old })
			getEUDMHostname = func(useSerial bool) (string, error) {
				require.Equal(t, value == "true", useSerial)
				return "device", nil
			}
			_, err := fromEUDM(context.Background(), "")
			require.NoError(t, err)
		})
	}
}
