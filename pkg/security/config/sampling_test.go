// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestEventSamplingEnabledFor(t *testing.T) {
	profiles := func(types ...model.EventType) RuntimeSecurityConfig {
		return RuntimeSecurityConfig{SecurityProfileV2Enabled: true, SecurityProfileV2EventTypes: types}
	}
	withOpen := func(c RuntimeSecurityConfig) RuntimeSecurityConfig {
		c.EventSamplingOpenEnabled = true
		return c
	}
	withUsage := func(c RuntimeSecurityConfig) RuntimeSecurityConfig {
		c.SBOMResolverUsageEnabled = true
		return c
	}

	for _, tc := range []struct {
		name      string
		cfg       RuntimeSecurityConfig
		eventType model.EventType
		want      bool
	}{
		{"open for profiles", withOpen(profiles(model.FileOpenEventType)), model.FileOpenEventType, true},
		{"open for profiles without opens", withOpen(profiles(model.ConnectEventType)), model.FileOpenEventType, false},
		{"open without profiles", withOpen(RuntimeSecurityConfig{}), model.FileOpenEventType, false},
		{"open for runtime usage", withUsage(withOpen(RuntimeSecurityConfig{})), model.FileOpenEventType, true},
		{"open for runtime usage without opens in profiles", withUsage(withOpen(profiles(model.ConnectEventType))), model.FileOpenEventType, true},
		{"open knob off for runtime usage", withUsage(RuntimeSecurityConfig{}), model.FileOpenEventType, false},
		{"connect for runtime usage", withUsage(RuntimeSecurityConfig{EventSamplingConnectEnabled: true}), model.ConnectEventType, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EventSamplingEnabledFor(tc.eventType); got != tc.want {
				t.Errorf("EventSamplingEnabledFor(%v) = %v, want %v", tc.eventType, got, tc.want)
			}
		})
	}
}
