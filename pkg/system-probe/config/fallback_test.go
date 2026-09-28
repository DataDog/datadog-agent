// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

func TestConfirmedConfigFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    any
		source   model.Source
		want     int64
		reported bool
	}{
		{"missing", nil, model.SourceFile, defaultMaxTrackedConnections, false},
		{"valid", 100, model.SourceFile, 100, false},
		{"zero invalid here", 0, model.SourceFile, defaultMaxTrackedConnections, true},
		{"negative", -1, model.SourceFile, defaultMaxTrackedConnections, true},
		{"higher priority blocks replacement", -1, model.SourceCLI, -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			key := spNS("max_tracked_connections")
			if tc.value != nil {
				cfg.Set(key, tc.value, tc.source)
			}
			Adjust(cfg)
			require.Equal(t, tc.want, cfg.GetInt64(key))
			var found []model.ConfigFallback
			for _, fallback := range cfg.GetConfigFallbacks() {
				if fallback.Key == key {
					found = append(found, fallback)
				}
			}
			if !tc.reported {
				require.Empty(t, found)
				return
			}
			require.Len(t, found, 1)
			require.Equal(t, "must be positive", found[0].Reason)
			require.EqualValues(t, defaultMaxTrackedConnections, found[0].DefaultValue)
		})
	}
	cfg := mock.NewSystemProbe(t)
	cfg.Set(spNS("max_tracked_connections"), 12345, model.SourceFile)
	cfg.Set(spNS("max_closed_connections_buffered"), -1, model.SourceFile)
	Adjust(cfg)
	for _, fallback := range cfg.GetConfigFallbacks() {
		if fallback.Key == spNS("max_closed_connections_buffered") {
			require.Nil(t, fallback.DefaultValue)
			require.Equal(t, spNS("max_tracked_connections"), fallback.ReplacementSetting)
			return
		}
	}
	t.Fatal("derived replacement was not recorded")
}
