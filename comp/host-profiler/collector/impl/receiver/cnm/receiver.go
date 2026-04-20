// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.uber.org/zap"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/network"
)

// connectionsSource abstracts the network tracer for testability.
// In production this is satisfied by *tracer.Tracer; in tests by a mock.
type connectionsSource interface {
	RegisterClient(clientID string) error
	GetActiveConnections(clientID string) (*network.Connections, func(), error)
}

// stoppable abstracts the Stop method so the receiver can shut down the tracer.
type stoppable interface {
	Stop()
}

const clientID = "cnm-otel-receiver"

// cnmReceiver collects network connection data via eBPF and produces pmetric.Metrics.
type cnmReceiver struct {
	cfg      *Config
	logger   *zap.Logger
	consumer consumer.Metrics

	// Agent Core components (nil in standalone mode)
	tagger   tagger.Component
	hostname hostname.Component
	agentCfg coreconfig.Component

	// Set during Start; the source can be a real tracer or a mock.
	source  connectionsSource
	stopper stoppable

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var _ component.Component = (*cnmReceiver)(nil)

// Start initializes the network tracer and begins the collection loop.
func (r *cnmReceiver) Start(ctx context.Context, _ component.Host) error {
	if r.source == nil {
		if err := r.initTracer(ctx); err != nil {
			return err
		}
	}

	if err := r.source.RegisterClient(clientID); err != nil {
		return err
	}

	collectCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	r.wg.Add(1)
	go r.collectLoop(collectCtx)

	r.logger.Info("CNM receiver started",
		zap.Duration("check_interval", r.cfg.CheckInterval),
		zap.Int("max_tracked_connections", r.cfg.MaxTrackedConnections),
	)
	return nil
}

// Shutdown stops the collection loop and the network tracer.
func (r *cnmReceiver) Shutdown(_ context.Context) error {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()

	if r.stopper != nil {
		r.stopper.Stop()
	}

	r.logger.Info("CNM receiver stopped")
	return nil
}

func (r *cnmReceiver) collectLoop(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.collect(ctx)
		}
	}
}

func (r *cnmReceiver) collect(ctx context.Context) {
	conns, cleanup, err := r.source.GetActiveConnections(clientID)
	if err != nil {
		r.logger.Error("failed to get active connections", zap.Error(err))
		return
	}
	defer cleanup()
	defer network.Reclaim(conns)

	if len(conns.Conns) == 0 {
		return
	}

	metrics := convertToMetrics(conns, r.resolveHostname(ctx))
	if err := r.consumer.ConsumeMetrics(ctx, metrics); err != nil {
		r.logger.Error("failed to consume metrics", zap.Error(err))
	}
}

func (r *cnmReceiver) resolveHostname(ctx context.Context) string {
	if r.hostname == nil {
		return ""
	}
	h, err := r.hostname.Get(ctx)
	if err != nil {
		r.logger.Warn("failed to resolve hostname", zap.Error(err))
		return ""
	}
	return h
}
