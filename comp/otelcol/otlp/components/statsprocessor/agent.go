// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package statsprocessor

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// OtelStatsWriter implements the trace-agent's `stats.Writer` interface via an `out` channel
// This provides backwards compatibility for otel components that do not yet use the latest agent version
// where these channels have been dropped
type OtelStatsWriter struct {
	out chan *pb.StatsPayload
}

// Write this payload to the `out` channel
func (a *OtelStatsWriter) Write(payload *pb.StatsPayload) {
	a.out <- payload
}

// NewOtelStatsWriter makes an OtelStatsWriter that writes to the given `out` chan
func NewOtelStatsWriter(out chan *pb.StatsPayload) *OtelStatsWriter {
	return &OtelStatsWriter{out}
}
