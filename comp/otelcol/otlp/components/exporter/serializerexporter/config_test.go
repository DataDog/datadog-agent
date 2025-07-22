// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

func TestUnmarshalDefaultConfig(t *testing.T) {
	factory := newFactory()
	cfg := factory.CreateDefaultConfig()
	require.NoError(t, confmap.New().Unmarshal(&cfg))
	assert.Equal(t, factory.CreateDefaultConfig(), cfg)
}

func TestUnmarshalConfig(t *testing.T) {
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)
	sub, err := cm.Sub(component.MustNewID(TypeStr).String())
	require.NoError(t, err)
	factory := newFactory()
	got := factory.CreateDefaultConfig()
	require.NoError(t, sub.Unmarshal(&got))
	assert.NoError(t, xconfmap.Validate(got))

	want := factory.CreateDefaultConfig().(*ExporterConfig)
	want.TimeoutConfig.Timeout = 10 * time.Second
	want.HTTPConfig.Timeout = 10 * time.Second
	want.QueueBatchConfig.QueueSize = 100
	want.Metrics.APMStatsReceiverAddr = "localhost:1234"
	want.Metrics.TagCardinality = "high"
	want.Metrics.Tags = "tag"
	want.Metrics.Metrics.DeltaTTL = 200
	want.Metrics.Metrics.Endpoint = "localhost:5678"
	want.Metrics.Metrics.ExporterConfig.ResourceAttributesAsTags = true
	want.Metrics.Metrics.ExporterConfig.InstrumentationScopeMetadataAsTags = true
	want.Metrics.Metrics.HistConfig.Mode = datadogconfig.HistogramModeCounters
	want.Metrics.Metrics.HistConfig.SendAggregations = true
	want.Metrics.Metrics.SumConfig.CumulativeMonotonicMode = datadogconfig.CumulativeMonotonicSumModeRawValue
	want.Metrics.Metrics.SummaryConfig.Mode = datadogconfig.SummaryModeNoQuantiles
	want.API.Key = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	want.API.Site = "localhost"
	want.API.FailOnInvalidKey = true
	want.HostMetadata.Enabled = true

	assert.Equal(t, want, got)
}
