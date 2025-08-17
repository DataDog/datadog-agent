// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestLegacyIntervalDefault(t *testing.T) {
	for _, tc := range []struct {
		name             string
		checkName        string
		expectedInterval time.Duration
	}{
		{
			name:             "container default",
			checkName:        ContainerCheckName,
			expectedInterval: ContainerCheckDefaultInterval,
		},
		{
			name:             "container rt default",
			checkName:        RTContainerCheckName,
			expectedInterval: RTContainerCheckDefaultInterval,
		},
		{
			name:             "process default",
			checkName:        ProcessCheckName,
			expectedInterval: ProcessCheckDefaultInterval,
		},
		{
			name:             "process rt default",
			checkName:        RTProcessCheckName,
			expectedInterval: RTProcessCheckDefaultInterval,
		},
		{
			name:             "connections default",
			checkName:        ConnectionsCheckName,
			expectedInterval: ConnectionsCheckDefaultInterval,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			assert.Equal(t, tc.expectedInterval, GetInterval(cfg, tc.checkName))
		})
	}
}

func TestLegacyIntervalOverride(t *testing.T) {
	override := 600
	for _, tc := range []struct {
		name             string
		checkName        string
		setting          string
		expectedInterval time.Duration
	}{
		{
			name:      "container default",
			setting:   "process_config.intervals.container",
			checkName: ContainerCheckName,
		},
		{
			name:      "container rt default",
			setting:   "process_config.intervals.container_realtime",
			checkName: RTContainerCheckName,
		},
		{
			name:      "process default",
			setting:   "process_config.intervals.process",
			checkName: ProcessCheckName,
		},
		{
			name:      "process rt default",
			setting:   "process_config.intervals.process_realtime",
			checkName: RTProcessCheckName,
		},
		{
			name:      "connections default",
			setting:   "process_config.intervals.connections",
			checkName: ConnectionsCheckName,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource(tc.setting, override)
			assert.Equal(t, time.Duration(override)*time.Second, GetInterval(cfg, tc.checkName))
		})
	}
}

// TestConnectionsCheckInterval tests the connections check interval logic including dynamic interval behavior
func TestConnectionsCheckInterval(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		customIntervalSeconds int
		enableDynamicInterval bool
		expectedInterval      time.Duration
	}{
		{
			name:                  "default behavior - dynamic interval disabled",
			customIntervalSeconds: 0,
			enableDynamicInterval: false,
			expectedInterval:      ConnectionsCheckDefaultInterval,
		},
		{
			name:                  "dynamic interval enabled",
			customIntervalSeconds: 0,
			enableDynamicInterval: true,
			expectedInterval:      ConnectionsCheckDynamicInterval,
		},
		{
			name:                  "custom interval overrides dynamic setting (disabled)",
			customIntervalSeconds: 10,
			enableDynamicInterval: false,
			expectedInterval:      10 * time.Second,
		},
		{
			name:                  "custom interval overrides dynamic setting (enabled)",
			customIntervalSeconds: 10,
			enableDynamicInterval: true,
			expectedInterval:      10 * time.Second,
		},
		{
			name:                  "custom interval too low with dynamic disabled",
			customIntervalSeconds: 5,
			enableDynamicInterval: false,
			expectedInterval:      ConnectionsCheckMinInterval,
		},
		{
			name:                  "custom interval too low with dynamic enabled",
			customIntervalSeconds: 5,
			enableDynamicInterval: true,
			expectedInterval:      ConnectionsCheckMinInterval,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)

			// Set custom interval if specified
			if tc.customIntervalSeconds > 0 {
				cfg.SetWithoutSource("process_config.intervals.connections", tc.customIntervalSeconds)
			}

			// Set dynamic interval flag
			cfg.SetWithoutSource("process_config.connections.enable_dynamic_interval", tc.enableDynamicInterval)

			result := GetInterval(cfg, ConnectionsCheckName)
			assert.Equal(t, tc.expectedInterval, result)
		})
	}
}

// TestProcessDiscoveryInterval tests to make sure that the process discovery interval validation works properly
func TestProcessDiscoveryInterval(t *testing.T) {
	for _, tc := range []struct {
		name             string
		interval         time.Duration
		expectedInterval time.Duration
	}{
		{
			name:             "allowed interval",
			interval:         8 * time.Hour,
			expectedInterval: 8 * time.Hour,
		},
		{
			name:             "below minimum",
			interval:         0,
			expectedInterval: discoveryMinInterval,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("process_config.process_discovery.interval", tc.interval)

			assert.Equal(t, tc.expectedInterval, GetInterval(cfg, DiscoveryCheckName))
		})
	}
}

func TestProcessEventsInterval(t *testing.T) {
	for _, tc := range []struct {
		name             string
		interval         time.Duration
		expectedInterval time.Duration
	}{
		{
			name:             "allowed interval",
			interval:         30 * time.Second,
			expectedInterval: 30 * time.Second,
		},
		{
			name:             "below minimum",
			interval:         0,
			expectedInterval: pkgconfigsetup.DefaultProcessEventsCheckInterval,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("process_config.event_collection.interval", tc.interval)

			assert.Equal(t, tc.expectedInterval, GetInterval(cfg, ProcessEventsCheckName))
		})
	}
}
