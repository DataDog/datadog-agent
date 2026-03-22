// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package opamp

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/zap"
)

const ownMetricsScope = "github.com/DataDog/datadog-agent/comp/otelcol/extensions/opamp"

// ownMetricsReporter manages an OTLP metrics pipeline directed by the OpAMP
// server via OwnMetrics ConnectionSettings. It reports a small set of
// agent-level metrics (uptime, health) to the server-specified endpoint.
//
// speky:DDOT#OTELCOL032
type ownMetricsReporter struct {
	mu     sync.Mutex
	logger *zap.Logger
	res    *resource.Resource

	cancel   context.CancelFunc
	provider *sdkmetric.MeterProvider
}

func newOwnMetricsReporter(logger *zap.Logger, serviceName, serviceVersion string) *ownMetricsReporter {
	res, _ := resource.New(
		context.Background(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion),
		),
	)
	return &ownMetricsReporter{logger: logger, res: res}
}

// applySettings tears down any existing OTLP metrics pipeline and starts a new
// one pointing at the endpoint specified in settings. A nil or empty settings
// object simply stops the current pipeline.
func (r *ownMetricsReporter) applySettings(settings *protobufs.TelemetryConnectionSettings) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()

	if settings == nil || settings.DestinationEndpoint == "" {
		return
	}

	endpoint := settings.DestinationEndpoint

	exp, err := r.buildExporter(endpoint)
	if err != nil {
		r.logger.Warn("Could not create OTLP metrics exporter for own metrics",
			zap.String("endpoint", endpoint), zap.Error(err))
		return
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(r.res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp,
			sdkmetric.WithInterval(30*time.Second),
		)),
	)

	ctx, cancel := context.WithCancel(context.Background())
	r.provider = provider
	r.cancel = cancel

	go r.runLoop(ctx, provider)
	r.logger.Info("Own metrics now reporting to OpAMP-directed endpoint",
		zap.String("endpoint", endpoint))
}

// buildExporter creates a gRPC or HTTP OTLP exporter based on the endpoint scheme.
func (r *ownMetricsReporter) buildExporter(endpoint string) (sdkmetric.Exporter, error) {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		opts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpointURL(endpoint)}
		if strings.HasPrefix(endpoint, "http://") {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		return otlpmetrichttp.New(context.Background(), opts...)
	}
	// Default: gRPC transport (insecure for plain host:port endpoints).
	return otlpmetricgrpc.New(context.Background(),
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
}

// runLoop registers an uptime gauge on the provided MeterProvider and blocks
// until ctx is cancelled, then flushes and shuts down the provider.
func (r *ownMetricsReporter) runLoop(ctx context.Context, provider *sdkmetric.MeterProvider) {
	meter := provider.Meter(ownMetricsScope)

	start := time.Now()
	uptime, err := meter.Float64ObservableGauge(
		"otelcol_process_uptime",
		metric.WithDescription("Time in seconds since the agent process started"),
		metric.WithUnit("s"),
	)
	if err != nil {
		r.logger.Warn("Could not register uptime gauge", zap.Error(err))
	} else {
		_, _ = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
			o.ObserveFloat64(uptime, time.Since(start).Seconds())
			return nil
		}, uptime)
	}

	<-ctx.Done()
	_ = provider.Shutdown(context.Background())
}

// stopLocked stops the current pipeline. Must be called with r.mu held.
func (r *ownMetricsReporter) stopLocked() {
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
		r.provider = nil
	}
}

// shutdown stops the reporter; called from the extension's Shutdown method.
func (r *ownMetricsReporter) shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
}
