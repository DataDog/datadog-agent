// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package crashreceiver

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/xreceiver"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/reporter/samples"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CrashReporter implements reporter.Reporter for crash-origin traces.
// Its consumer is set by the OTel factory; on ReportTraceEvent it builds a
// pprofile and delivers it immediately to the crash pipeline.
type CrashReporter struct {
	mu       sync.RWMutex
	consumer xconsumer.Profiles
}

func NewCrashReporter() *CrashReporter {
	return &CrashReporter{}
}

func (r *CrashReporter) setConsumer(c xconsumer.Profiles) {
	r.mu.Lock()
	r.consumer = c
	r.mu.Unlock()
	log.Info("crash receiver: consumer registered, crash events will be forwarded to the crash pipeline")
}

func (r *CrashReporter) Start(_ context.Context) error { return nil }
func (r *CrashReporter) Stop()                          {}

func (r *CrashReporter) ReportTraceEvent(trace *libpf.Trace, meta *samples.TraceEventMeta) error {
	r.mu.RLock()
	c := r.consumer
	r.mu.RUnlock()
	if c == nil {
		log.Warnf("crash receiver: dropping crash event for pid %d (no consumer registered)", meta.PID)
		return nil
	}
	log.Infof("crash receiver: captured crash event — pid=%d comm=%s signal=%d frames=%d",
		meta.PID, meta.Comm, meta.Value, len(trace.Frames))
	return c.ConsumeProfiles(context.Background(), buildCrashProfile(trace, meta))
}

type noopReceiver struct{}

func (noopReceiver) Start(_ context.Context, _ component.Host) error { return nil }
func (noopReceiver) Shutdown(_ context.Context) error                 { return nil }

type crashConfig struct{}

func (c *crashConfig) Validate() error { return nil }

func NewFactory(cr *CrashReporter) receiver.Factory {
	return xreceiver.NewFactory(
		component.MustNewType("crash"),
		func() component.Config { return &crashConfig{} },
		xreceiver.WithProfiles(
			func(_ context.Context, _ receiver.Settings, _ component.Config, nextConsumer xconsumer.Profiles) (xreceiver.Profiles, error) {
				cr.setConsumer(nextConsumer)
				return noopReceiver{}, nil
			},
			component.StabilityLevelAlpha,
		),
	)
}
