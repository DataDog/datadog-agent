// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package converternoagent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestConverterNoAgentConvert(t *testing.T) {
	yaml := `
processors:
  infraattributes/default:
    enabled: true
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - infraattributes/default
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
