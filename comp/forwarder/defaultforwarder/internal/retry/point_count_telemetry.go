// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// Report the metric as a gauge because metrics may be dropped if connection with datadog is
// interrupted for a long time. When the connection recovers, the gauge will give the total number
// of points dropped during this time.
var tlmPointsDropped = telemetry.NewGaugeWithOpts("point", "dropped", []string{"domain"}, "", telemetry.Options{DefaultMetric: true})
var tlmPointsSent = telemetry.NewGaugeWithOpts("point", "sent", []string{"domain"}, "", telemetry.Options{DefaultMetric: true})

// PointCountTelemetry sends the number of points successfully sent and the number of points dropped.
type PointCountTelemetry struct {
	dropped telemetry.SimpleGauge
	sent    telemetry.SimpleGauge
}

// NewPointCountTelemetry creates a new instance of PointCountTelemetry.
func NewPointCountTelemetry(domain string) *PointCountTelemetry {
	return &PointCountTelemetry{
		dropped: tlmPointsDropped.WithValues(domain),
		sent:    tlmPointsSent.WithValues(domain),
	}
}

// OnPointDropped increases the telemetry that counts the number of points droppped
func (t *PointCountTelemetry) OnPointDropped(count int) {
	t.dropped.Add(float64(count))
}

// OnPointSuccessfullySent increases the telemetry that counts the number of points successfully sent.
func (t *PointCountTelemetry) OnPointSuccessfullySent(count int) {
	t.sent.Add(float64(count))
}
