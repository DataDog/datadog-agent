// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package nodetreemodel

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestConfigTypeConversions(t *testing.T) {
	for _, tc := range []struct {
		name           string
		defaultValue   any
		yaml           string
		from, to, path string
	}{
		{"quoted integer", 8125, `"9000"`, "string", "integer", ""},
		{"quoted boolean", false, `"true"`, "string", "boolean", ""},
		{"number to string", "", "123", "integer", "string", ""},
		{"fraction truncated", 1, "1.5", "number", "integer", ""},
		{"infinity converted", 1, ".inf", "number", "integer", ""},
		{"list element", []string{}, "[hello, 123]", "integer", "string", "/1"},
		{"failed conversion", 8125, "not-a-port", "", "", ""},
		{"already correct", 8125, "9000", "", "", ""},
		{"duration syntax", time.Second, "2s", "", "", ""},
		{"integer width", int64(1), "2", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := NewNodeTreeConfig("test", "", nil)
			cfg.SetDefault("nested.setting", tc.defaultValue)
			cfg.BuildSchema()
			require.NoError(t, cfg.ReadConfig(strings.NewReader("nested:\n  setting: "+tc.yaml)))
			got := cfg.GetConfigTypeConversions()
			if tc.from == "" {
				require.Empty(t, got)
				return
			}
			require.Equal(t, []model.ConfigTypeConversion{{Key: "nested.setting", Path: tc.path, Source: model.SourceFile, FromType: tc.from, ToType: tc.to}}, got)
		})
	}
}

func TestConfigTypeConversionPrecedence(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "TEST_CONVERSION", nil)
	cfg.SetDefault("port", 8125)
	cfg.BuildSchema()
	require.NoError(t, cfg.ReadConfig(strings.NewReader(`port: "9000"`)))
	require.Equal(t, 9000, cfg.GetInt("port"))
	require.Len(t, cfg.GetConfigTypeConversions(), 1)
	cfg.Set("port", 9000, model.SourceCLI)
	require.Empty(t, cfg.GetConfigTypeConversions())
	cfg.UnsetForSource("port", model.SourceCLI)
	require.Len(t, cfg.GetConfigTypeConversions(), 1)
	cfg.Set("port", 9000, model.SourceFile)
	require.Empty(t, cfg.GetConfigTypeConversions())
	cfg.Set("port", "9010", model.SourceSecret)
	require.Equal(t, model.SourceSecret, cfg.GetConfigTypeConversions()[0].Source)
	cfg.DirectBulkSet([]model.DirectSetting{{Key: "port", Value: float64(9010), Source: model.SourceSecret}}, false)
	require.Empty(t, cfg.GetConfigTypeConversions())
}

func TestConfigTypeConversionEnvironment(t *testing.T) {
	t.Setenv("TEST_CONVERSION_PORT", "9000")
	cfg := NewNodeTreeConfig("test", "TEST_CONVERSION", nil)
	cfg.BindEnvAndSetDefault("port", 8125)
	cfg.BuildSchema()
	require.Equal(t, 9000, cfg.GetInt("port"))
	require.Empty(t, cfg.GetConfigTypeConversions())
}
