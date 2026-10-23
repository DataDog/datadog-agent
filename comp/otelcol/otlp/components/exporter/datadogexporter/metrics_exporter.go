// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter

import (
	"context"

	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/tracestats"
)

// sdkMetricsExporter separates SDK stats before the serializer's queue and retries.
type sdkMetricsExporter struct {
	exporter.Metrics
	host   func(context.Context) (string, error)
	send   func([]byte)
	logger *zap.Logger
}

func (e *sdkMetricsExporter) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sdkMetrics := 0
	for _, rm := range md.ResourceMetrics().All() {
		for _, sm := range rm.ScopeMetrics().All() {
			for _, metric := range sm.Metrics().All() {
				if metric.Name() == tracestats.SDKTraceMetricName {
					sdkMetrics++
				}
			}
		}
	}
	if sdkMetrics == 0 {
		return e.Metrics.ConsumeMetrics(ctx, md)
	}
	host, err := e.host(ctx)
	if err != nil {
		return err
	}
	var payloads [][]byte
	for _, rm := range md.ResourceMetrics().All() {
		attrs := rm.Resource().Attributes()
		for _, sm := range rm.ScopeMetrics().All() {
			for _, metric := range sm.Metrics().All() {
				if metric.Name() != tracestats.SDKTraceMetricName {
					continue
				}
				payload, conversionErrors := tracestats.BuildSDKTraceStatsPayload(attributes.GetHost(attrs, host), "otlp-intake-metrics", attrs, metric)
				for _, err := range conversionErrors {
					e.logger.Debug("Invalid SDK trace metric", zap.Error(err))
				}
				if payload != nil {
					raw, err := proto.Marshal(payload)
					if err != nil {
						e.logger.Debug("Invalid SDK trace stats payload", zap.Error(err))
					} else {
						payloads = append(payloads, raw)
					}
				}
			}
		}
	}
	if sdkMetrics != md.MetricCount() {
		// Preserve mixed input for upstream retries; the serializer mutates its subset.
		remaining := pmetric.NewMetrics()
		md.CopyTo(remaining)
		remaining.ResourceMetrics().RemoveIf(func(rm pmetric.ResourceMetrics) bool {
			rm.ScopeMetrics().RemoveIf(func(sm pmetric.ScopeMetrics) bool {
				sm.Metrics().RemoveIf(func(metric pmetric.Metric) bool { return metric.Name() == tracestats.SDKTraceMetricName })
				return sm.Metrics().Len() == 0
			})
			return rm.ScopeMetrics().Len() == 0
		})
		if err := e.Metrics.ConsumeMetrics(ctx, remaining); err != nil {
			return err
		}
	}
	// Only the generic subset enters exporterhelper retries. No SDK queue needs draining.
	for _, payload := range payloads {
		e.send(payload)
	}
	return nil
}
