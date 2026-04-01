// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/receiver"

	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/controller"
	"go.opentelemetry.io/ebpf-profiler/metrics"
	"go.opentelemetry.io/ebpf-profiler/reporter"
	"go.opentelemetry.io/ebpf-profiler/times"
	"go.opentelemetry.io/ebpf-profiler/vc"
)

const (
	ctrlName = "go.opentelemetry.io/ebpf-profiler"
)

// Controller is a bridge between the Collector's [receiverprofiles.Profiles]
// interface and our [internal.Controller]
type Controller struct {
	ctlr       *controller.Controller
	onShutdown func() error
}

func NewController(cfg *controller.Config, rs receiver.Settings,
	nextConsumer xconsumer.Profiles,
) (*Controller, error) {
	intervals := times.New(cfg.ReporterInterval,
		cfg.MonitorInterval, cfg.ProbabilisticInterval)

	if cfg.ReporterFactory == nil {
		cfg.ReporterFactory = func(cfg *reporter.Config, nextConsumer xconsumer.Profiles) (reporter.Reporter, error) {
			return reporter.NewCollector(cfg, nextConsumer)
		}
	}

	rep, err := cfg.ReporterFactory(&reporter.Config{
		Name:                   ctrlName,
		Version:                vc.Version(),
		MaxRPCMsgSize:          cfg.MaxRPCMsgSize,
		MaxGRPCRetries:         cfg.MaxGRPCRetries,
		GRPCOperationTimeout:   intervals.GRPCOperationTimeout(),
		GRPCStartupBackoffTime: intervals.GRPCStartupBackoffTime(),
		GRPCConnectionTimeout:  intervals.GRPCConnectionTimeout(),
		ReportInterval:         intervals.ReportInterval(),
		ReportJitter:           cfg.ReporterJitter,
		SamplesPerSecond:       cfg.SamplesPerSecond,
	}, nextConsumer)
	if err != nil {
		return nil, err
	}
	cfg.Reporter = rep

	// Provide internal metrics via the collectors telemetry.
	meter := rs.MeterProvider.Meter(ctrlName)
	metrics.Start(meter)

	return &Controller{
		onShutdown: cfg.OnShutdown,
		ctlr:       controller.New(cfg),
	}, nil
}

// Start starts the receiver.
func (c *Controller) Start(ctx context.Context, _ component.Host) error {
	return c.ctlr.Start(ctx)
}

// Shutdown stops the receiver.
func (c *Controller) Shutdown(_ context.Context) error {
	c.ctlr.Shutdown()
	if c.onShutdown != nil {
		return c.onShutdown()
	}
	return nil
}
