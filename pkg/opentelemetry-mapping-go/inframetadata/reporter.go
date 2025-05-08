// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package inframetadata

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/internal/hostmap"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/payload"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

const (
	// AttributeDatadogHostUseAsMetadata states if a payload should be used for host metadata.
	// It overrides the default behavior (see below).
	AttributeDatadogHostUseAsMetadata = "datadog.host.use_as_metadata"

	// shouldUseByDefault specifies if the payloads should be used by default when the
	// use_as_metadata resource attribute is missing.
	shouldUseByDefault = false
)

// Pusher is the interface for pushing host metadata to a remote endpoint.
type Pusher interface {
	// Push host metadata to a remote endpoint.
	// MUST be safe to call concurrently.
	Push(context.Context, payload.HostMetadata) error
}

// Reporter of host metadata based on pcommon.Resource payloads.
type Reporter struct {
	// logger (sampled) for warnings.
	logger *zap.Logger
	// hostMap storing the host metadata.
	hostMap *hostmap.HostMap
	// pusher of host metadata.
	pusher Pusher
	// closeCh can stop the host metadata reporting.
	closeCh chan struct{}
	// ticker for periodic host metadata reporting.
	ticker *time.Ticker
}

// Copied over from github.com/open-telemetry/opentelemetry-collector/blob/14c039d/exporter/exporterhelper/queued_retry.go#L269
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
func createSampledLogger(logger *zap.Logger) *zap.Logger {
	if logger.Core().Enabled(zapcore.DebugLevel) {
		// Debugging is enabled. Don't do any sampling.
		return logger
	}

	// Sample all messages to 1 per 10 seconds initially,
	// and 1/100 of messages after that.
	opts := zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(core, 10*time.Second, 1, 100)
	})
	return logger.WithOptions(opts)
}

// NewReporter creates a new host metadata reporter.
// The reporter consumes pcommon.Resources through its 'Consume' method and merges them into payload.HostMetadata payloads.
// It then exports the payloads through the pusher with a specified period.
func NewReporter(logger *zap.Logger, pusher Pusher, period time.Duration) (*Reporter, error) {
	hostMap := hostmap.New()
	return &Reporter{
		logger:  createSampledLogger(logger),
		hostMap: hostMap,
		pusher:  pusher,
		closeCh: make(chan struct{}),
		ticker:  time.NewTicker(period),
	}, nil
}

// hasHostMetadata to see if it should be used by default.
// A resource is usable if 'AttributeDatadogHostUseAsMetadata' is true or shouldUseByDefault is true.
func hasHostMetadata(res pcommon.Resource) (bool, error) {
	shouldUse := shouldUseByDefault
	if val, ok := res.Attributes().Get(AttributeDatadogHostUseAsMetadata); ok {
		if val.Type() != pcommon.ValueTypeBool {
			return false, fmt.Errorf("%q has type %q, expected \"Bool\"", AttributeDatadogHostUseAsMetadata, val.Type())
		}
		shouldUse = val.Bool()
	}
	return shouldUse, nil
}

func (r *Reporter) pushAndLog(ctx context.Context, hm payload.HostMetadata) {
	if err := r.pusher.Push(ctx, hm); err != nil {
		r.logger.Error("Failed to send host metadata",
			zap.String("host", hm.Meta.Hostname),
			zap.Error(err),
			zap.Any("payload", hm),
		)
	}
}

func (r *Reporter) hostname(res pcommon.Resource) (string, bool) {
	src, ok := attributes.SourceFromAttrs(res.Attributes(), nil)
	if !ok {
		r.logger.Warn("resource does not have host-identifying attributes",
			zap.Any("attributes", res.Attributes().AsRaw()),
			zap.String("further info", "https://docs.datadoghq.com/opentelemetry/schema_semantics/hostname/?tab=datadogexporter"),
		)
		return "", false
	}
	if src.Kind != source.HostnameKind {
		// The resource does not identify a host (e.g. serverless resource)
		return "", false
	}
	return src.Identifier, true
}

// ConsumeResource for host metadata reporting purposes.
// The resource will be used only if it is usable (see 'hasHostMetadata') and it has a host attribute.
func (r *Reporter) ConsumeResource(res pcommon.Resource) error {
	if ok, err := hasHostMetadata(res); err != nil {
		return fmt.Errorf("failed to check resource: %w", err)
	} else if !ok {
		// The resource should not be used for host metadata.
		return nil
	}

	hostname, ok := r.hostname(res)
	if !ok {
		return nil
	}

	changed, payload, err := r.hostMap.Update(hostname, res)
	if changed {
		r.logger.Debug("Host metadata changed for host after payload",
			zap.String("host", hostname), zap.Any("attributes", res.Attributes()),
		)
		r.pushAndLog(context.Background(), payload)
	}
	if err != nil {
		return err
	}
	return nil
}

// ConsumeMetrics checks if a metric is tracked by the reporter
// and if so updates the host metadata accordingly.
func (r *Reporter) ConsumeMetrics(md pmetric.Metrics) error {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		res := rm.Resource()
		if ok, err := hasHostMetadata(res); err != nil {
			return fmt.Errorf("failed to check resource: %w", err)
		} else if !ok {
			// The resource should not be used for host metadata.
			// Go to next resource.
			continue
		}

		host, ok := r.hostname(res)
		if !ok {
			continue
		}
		ilms := rm.ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			metricsArray := ilms.At(j).Metrics()
			for k := 0; k < metricsArray.Len(); k++ {
				metric := metricsArray.At(k)
				if _, ok := hostmap.TrackedMetrics[metric.Name()]; ok {
					r.hostMap.UpdateFromMetric(host, metric)
				}
			}
		}
	}
	return nil
}

// ConsumeHostMetadata consumes a host metadata payload and pushes it.
func (r *Reporter) ConsumeHostMetadata(hm payload.HostMetadata) error {
	if err := r.hostMap.Set(hm); err != nil {
		return err
	}
	r.pushAndLog(context.Background(), hm)

	return nil
}

// Run the reporter to periodically export
func (r *Reporter) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	for {
		select {
		case <-r.ticker.C:
			// extract payloads from hostMap and report them.
			for host, payload := range r.hostMap.Flush() {
				r.logger.Info("Sending host metadata",
					zap.String("host", host))
				r.pushAndLog(ctx, payload)
			}
		case <-r.closeCh:
			cancel()
			r.logger.Info("Stopped reporter")
			return nil
		}
	}
}

// Stop the reporter.
func (r *Reporter) Stop() {
	close(r.closeCh)
}
