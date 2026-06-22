// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"context"

	"go.opentelemetry.io/collector/consumer/xconsumer"

	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/reporter"
	"go.opentelemetry.io/ebpf-profiler/reporter/samples"
	"go.opentelemetry.io/ebpf-profiler/support"
)

// crashAwareReporter routes crash-origin events to a dedicated crash reporter
// and all other events to the regular batch reporter.
type crashAwareReporter struct {
	regular reporter.Reporter
	crash   reporter.Reporter
}

func (r *crashAwareReporter) Start(ctx context.Context) error {
	if err := r.crash.Start(ctx); err != nil {
		return err
	}
	return r.regular.Start(ctx)
}

func (r *crashAwareReporter) Stop() {
	r.crash.Stop()
	r.regular.Stop()
}

func (r *crashAwareReporter) ReportTraceEvent(trace *libpf.Trace, meta *samples.TraceEventMeta) error {
	if meta.Origin == support.TraceOriginCrash {
		return r.crash.ReportTraceEvent(trace, meta)
	}
	return r.regular.ReportTraceEvent(trace, meta)
}

// WithCrashReporter returns a BuildProfilesReceiver option that creates a
// crashAwareReporter using crash as the crash-pipeline reporter.
func WithCrashReporter(crash reporter.Reporter) ebpfcollector.Option {
	return ebpfcollector.WithReporterFactory(
		func(cfg *reporter.Config, regularConsumer xconsumer.Profiles) (reporter.Reporter, error) {
			base, err := reporter.NewCollector(cfg, regularConsumer)
			if err != nil {
				return nil, err
			}
			return &crashAwareReporter{regular: base, crash: crash}, nil
		},
	)
}
