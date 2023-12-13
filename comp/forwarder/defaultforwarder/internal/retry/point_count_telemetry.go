// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.uber.org/atomic"
)

// PointCountTelemetry sends the number of points successfully sent and the number of points dropped.
type PointCountTelemetry struct {
	tags              []string
	provider          *telemetry.StatsTelemetryProvider
	droppedPointCount atomic.Int64
	pointSentCount    atomic.Int64
	startStopAction   *util.StartStopAction
}

// NewPointCountTelemetry creates a new instance of PointCountTelemetry.
func NewPointCountTelemetry(domain string, provider *telemetry.StatsTelemetryProvider) *PointCountTelemetry {
	return &PointCountTelemetry{
		tags:            []string{"domain:" + domain},
		provider:        provider,
		startStopAction: util.NewStartStopAction()}
}

// Start starts sending metrics.
func (t *PointCountTelemetry) Start() {
	t.startStopAction.Start(func(context context.Context) {
		ticker := time.NewTicker(30 * time.Second)
		for {
			select {
			case <-context.Done():
				return
			case <-ticker.C:
				// Report the metric as gauge because metrics may be dropped. When the connexion
				// recovers, the gauge gives the total amount of points dropped.
				count := t.droppedPointCount.Load()
				t.provider.GaugeNoIndex("datadog.agent.point.dropped", float64(count), t.tags)

				count = t.pointSentCount.Load()
				t.provider.GaugeNoIndex("datadog.agent.point.sent", float64(count), t.tags)
			}
		}
	})
}

// Stop stops sending metrics.
func (t *PointCountTelemetry) Stop() {
	t.startStopAction.Stop()
}

// OnPointDropped increases the telemetry that counts the number of points droppped
func (t *PointCountTelemetry) OnPointDropped(count int) {
	t.droppedPointCount.Add(int64(count))
}

// OnPointSuccessfullySent increases the telemetry that counts the number of points successfully sent.
func (t *PointCountTelemetry) OnPointSuccessfullySent(count int) {
	t.pointSentCount.Add(int64(count))
}
