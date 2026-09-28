// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package nodetreemodel

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

func TestConfigFallbackOwnership(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("setting", 1)
	cfg.BuildSchema()
	fallback := model.ConfigFallback{Key: "setting", Consumer: "first", Reason: "must be positive", DefaultValue: 1}
	cfg.RecordConfigFallback(fallback)
	cfg.RecordConfigFallback(fallback)
	require.Equal(t, []model.ConfigFallback{fallback}, cfg.GetConfigFallbacks())
	// A config edit alone does not replace a running consumer's already adopted value.
	cfg.Set("setting", 2, model.SourceCLI)
	require.Len(t, cfg.GetConfigFallbacks(), 1)
	fallback.Consumer = "second"
	cfg.RecordConfigFallback(fallback)
	cfg.ClearConfigFallback("setting", "first")
	require.Equal(t, []model.ConfigFallback{fallback}, cfg.GetConfigFallbacks())
	cfg.ClearConfigFallback("setting", "second")
	require.Empty(t, cfg.GetConfigFallbacks())
}
