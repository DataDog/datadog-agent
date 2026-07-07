// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

// TestValidateContainerTagPromotion checks that Config.Validate accepts the
// empty value (treated as off) plus the three documented modes, and rejects any
// other value with a self-describing error.
func TestValidateContainerTagPromotion(t *testing.T) {
	tests := []struct {
		name        string
		mode        ContainerTagPromotionMode
		wantErr     bool
		errContains string
	}{
		{name: "empty (default off)", mode: ""},
		{name: "off", mode: ContainerTagPromotionOff},
		{name: "duplicate", mode: ContainerTagPromotionDuplicate},
		{name: "rename", mode: ContainerTagPromotionRename},
		{name: "invalid", mode: "foo", wantErr: true, errContains: "invalid trace_container_tag_promotion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{TraceContainerTagPromotion: tt.mode}
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestLoadingConfigStrictLogs tests loading testdata/logs_strict.yaml
func TestLoadingConfigStrictLogs(t *testing.T) {
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "logs_strict.yaml"))
	require.NoError(t, err)

	tests := []struct {
		id       component.ID
		expected *Config
	}{
		{
			id:       component.MustNewIDWithName("filter", "empty"),
			expected: &Config{TraceContainerTagPromotion: ContainerTagPromotionOff},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id.String(), func(t *testing.T) {
			tc := testutil.NewTestTaggerClient()
			f := NewFactoryForAgent(tc, func(_ context.Context) (string, error) {
				return "test-host", nil
			})
			cfg := f.CreateDefaultConfig()

			sub, err := cm.Sub(tt.id.String())
			require.NoError(t, err)
			require.NoError(t, sub.Unmarshal(&cfg))

			assert.NoError(t, xconfmap.Validate(cfg))
			assert.Equal(t, tt.expected, cfg)
		})
	}
}
