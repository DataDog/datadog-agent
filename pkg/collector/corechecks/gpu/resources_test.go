// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"testing"
	"time"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuresources "github.com/DataDog/datadog-agent/pkg/gpu/resources"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/genresources"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestConfigureGPUResourcesFeatureGate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
	}{
		{name: "disabled"}, {name: "enabled", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("gpu.resources.enabled", tc.enabled)
			check := &Check{CheckBase: core.NewCheckBase(CheckName), gpuConfig: gpuconfig.New()}
			require.NoError(t, check.configureResources())
			require.Equal(t, tc.enabled, check.resourcePublisher != nil)
		})
	}
}

type resourceRecorder struct{ payload []byte }

func (r *resourceRecorder) EventPlatformEvent(payload []byte, _ string) { r.payload = payload }

func TestConfigureGPUResourceTiming(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interval    time.Duration
		ttl         string
		expectedTTL time.Duration
		wantError   bool
	}{
		{name: "defaults", interval: 15 * time.Second, ttl: "15m", expectedTTL: 15 * time.Minute},
		{name: "long collection interval", interval: 10 * time.Minute, ttl: "15m", expectedTTL: 30 * time.Minute},
		{name: "longer retention", interval: time.Minute, ttl: "1h", expectedTTL: time.Hour},
		{name: "invalid TTL", ttl: "-1m", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("gpu.resources.enabled", true)
			cfg.SetInTest("gpu.collection_interval_override", int(tc.interval/time.Second))
			cfg.SetInTest("gpu.resources.ttl", tc.ttl)
			check := &Check{CheckBase: core.NewCheckBase(CheckName), gpuConfig: gpuconfig.New()}
			err := check.configureResources()
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			configuredTTL, err := time.ParseDuration(tc.ttl)
			require.NoError(t, err)
			warnings := check.GetWarnings()
			if tc.expectedTTL > configuredTTL {
				require.Len(t, warnings, 1)
				require.ErrorContains(t, warnings[0], "increased from 15m0s to 30m0s")
			} else {
				require.Empty(t, warnings)
			}
			recorder := &resourceRecorder{}
			now := time.Unix(1700000000, 0)
			require.NoError(t, check.resourcePublisher.Publish(now, "host", []gpuresources.Device{{UUID: "gpu-device"}}, recorder))
			var event genresources.GenericResourceEvent
			require.NoError(t, proto.Unmarshal(recorder.payload, &event))
			require.Equal(t, tc.expectedTTL, event.Resource.ExpireAt.AsTime().Sub(event.Resource.SeenAt.AsTime()))
		})
	}
}
