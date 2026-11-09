// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func enableMetricMetadataGate(t *testing.T) {
	previous := metricMetadataGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(metricMetadataGate.ID(), true))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(metricMetadataGate.ID(), previous))
	})
}

func TestSubmitMetricMetadataNeedsAppKey(t *testing.T) {
	enableMetricMetadataGate(t)
	f := NewFactoryForAgent(&metricRecorder{}, func(context.Context) (string, error) {
		return "agent-host", nil
	}, nil, TelemetryStore{})
	_, err := f.CreateMetrics(context.Background(), exportertest.NewNopSettings(component.MustNewType("serializer")), f.CreateDefaultConfig())
	assert.ErrorContains(t, err, "needs api::app_key")
}

func TestSubmitMetricMetadata(t *testing.T) {
	enableMetricMetadataGate(t)

	requests := make(chan string, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- r.Method + " " + r.URL.Path + " " + string(body)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	f := NewFactoryForAgent(&metricRecorder{}, func(context.Context) (string, error) {
		return "agent-host", nil
	}, nil, TelemetryStore{})
	cfg := f.CreateDefaultConfig().(*ExporterConfig)
	cfg.QueueBatchConfig = configoptional.None[exporterhelper.QueueBatchConfig]()
	cfg.API.AppKey = "app-key"
	cfg.Metrics.Metrics.Endpoint = srv.URL
	exp, err := f.CreateMetrics(ctx, exportertest.NewNopSettings(component.MustNewType("serializer")), cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, exp.Shutdown(context.Background())) })

	md := pmetric.NewMetrics()
	m := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("test.metric")
	m.SetDescription("A gauge.")
	m.SetUnit("By")
	m.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
	require.NoError(t, exp.ConsumeMetrics(ctx, md))

	select {
	case got := <-requests:
		assert.Equal(t, `PUT /api/v1/metrics/test.metric {"description":"A gauge.","unit":"byte"}`, got)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "timed out waiting for metric metadata")
	}
}
