// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package converternoagent

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestConverterNoAgentConvert(t *testing.T) {
	yaml := fmt.Sprintf(`
processors:
  %s:
    enabled: true
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - %s
        - otherProcessor
`, infraAttributesName(), infraAttributesName())
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yaml))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err)
	converter := &converterNoAgent{}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	require.Equal(t, conf.ToStringMap(), map[string]any{
		"processors": map[string]any{
			"otherProcessor": map[string]any{},
		},
		"service": map[string]any{
			"pipelines": map[string]any{
				"profiles": map[string]any{
					"processors": []any{"otherProcessor"},
				},
			},
		},
	})
}

func TestConverterNoAgentConvertNoInfraAttributes(t *testing.T) {
	yaml := `
processors:
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - otherProcessor
`
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yaml))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err)
	converter := &converterNoAgent{}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	require.Equal(t, conf.ToStringMap(), map[string]any{
		"processors": map[string]any{
			"otherProcessor": map[string]any{},
		},
		"service": map[string]any{
			"pipelines": map[string]any{
				"profiles": map[string]any{
					"processors": []any{"otherProcessor"},
				},
			},
		},
	})
}

func TestConverterInfraAttributesName(t *testing.T) {
	config := getDefaultConfig(t)
	require.Equal(t, 3, strings.Count(config, infraAttributesName()))
}

func getDefaultConfig(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	configPath := filepath.Join(filepath.Dir(file), "../../../../..", "cmd", "host-profiler", "dist", "host-profiler-config.yaml")
	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	return string(configData)
}
