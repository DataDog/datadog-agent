// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextension

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/collector/component"
	"google.golang.org/grpc"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// dogtelExtension implements the extension.Extension interface
type dogtelExtension struct {
	config     *Config
	log        log.Component
	coreConfig coreconfig.Component

	// Core components injected from FX
	serializer serializer.MetricSerializer
	hostname   hostnameinterface.Component
	tagger     tagger.Component
	ipc        ipc.Component
	telemetry  telemetry.Component

	// Tagger gRPC server
	taggerServer     *grpc.Server
	taggerServerPort int
	taggerListener   net.Listener

	// Metric submission goroutine management
	metricCtx    context.Context
	metricCancel context.CancelFunc
}

// Start implements extension.Extension
func (e *dogtelExtension) Start(ctx context.Context, host component.Host) error {
	// Check if running in standalone mode
	standalone := e.coreConfig.GetBool("otel_standalone")
	if !standalone {
		e.log.Warn("dogtelextension is enabled but DD_OTEL_STANDALONE is false")
		e.log.Warn("This extension should only be used in standalone mode (DD_OTEL_STANDALONE=true)")
		e.log.Warn("In bundled mode, the core Datadog Agent provides these functionalities")
		e.log.Info("dogtelextension disabled (not in standalone mode)")
		return nil
	}

	e.log.Info("Starting dogtelextension in standalone mode")

	// Start tagger gRPC server if enabled
	if e.config.EnableTaggerServer {
		if err := e.startTaggerServer(); err != nil {
			e.log.Errorf("Failed to start tagger server: %v", err)
			return err
		}
	}

	// Start periodic metric submission
	e.startMetricSubmission()

	e.log.Infof("dogtelextension started successfully (tagger_port=%d)", e.taggerServerPort)

	return nil
}

// Shutdown implements extension.Extension
func (e *dogtelExtension) Shutdown(ctx context.Context) error {
	e.log.Info("Shutting down dogtelextension")

	// Stop metric submission goroutine
	e.stopMetricSubmission()

	// Stop tagger server gracefully
	e.stopTaggerServer()

	e.log.Info("dogtelextension shutdown complete")
	return nil
}

// GetTaggerServerPort implements dogtelextension.Component
func (e *dogtelExtension) GetTaggerServerPort() int {
	return e.taggerServerPort
}

// startMetricSubmission starts the goroutine that periodically submits the running metric
func (e *dogtelExtension) startMetricSubmission() {
	if e.config.MetadataInterval <= 0 {
		e.log.Info("Metric submission disabled (metadata_interval <= 0)")
		return
	}

	e.metricCtx, e.metricCancel = context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(time.Duration(e.config.MetadataInterval) * time.Second)
		defer ticker.Stop()

		// Submit immediately on start
		e.submitRunningMetric()

		for {
			select {
			case <-ticker.C:
				e.submitRunningMetric()
			case <-e.metricCtx.Done():
				e.log.Debug("Metric submission goroutine stopped")
				return
			}
		}
	}()

	e.log.Infof("Started periodic metric submission (interval: %ds)", e.config.MetadataInterval)
}

// stopMetricSubmission stops the metric submission goroutine
func (e *dogtelExtension) stopMetricSubmission() {
	if e.metricCancel != nil {
		e.metricCancel()
		e.log.Debug("Stopped metric submission goroutine")
	}
}

// submitRunningMetric submits the dogtel_extension.running metric
func (e *dogtelExtension) submitRunningMetric() {
	hostname, err := e.hostname.Get(context.Background())
	if err != nil {
		e.log.Warnf("Failed to get hostname for running metric: %v", err)
		hostname = "unknown"
	}

	// Create the metric series
	series := metrics.NewIterableSeries(func(*metrics.Serie) {}, 1, 1)
	series.Append(&metrics.Serie{
		Name:   "dogtel_extension.running",
		Points: []metrics.Point{{Ts: float64(time.Now().Unix()), Value: 1.0}},
		Tags:   tagset.CompositeTagsFromSlice([]string{fmt.Sprintf("host:%s", hostname)}),
		MType:  metrics.APIGaugeType,
		Host:   hostname,
		Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
	})

	// Send the metric
	if err := e.serializer.SendIterableSeries(series); err != nil {
		e.log.Warnf("Failed to submit running metric: %v", err)
	}
}
