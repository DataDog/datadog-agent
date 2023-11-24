// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmListener            = telemetry.NewHistogramNoOp()
	defaultListenerBuckets = []float64{300, 500, 1000, 1500, 2000, 2500, 3000, 10000, 20000, 50000}
)

// InitTelemetry initialize the telemetry.Histogram buckets for the internal
// telemetry. This will be called once the first dogstatsd server is created
// since we need the configuration to be fully loaded.
func InitTelemetry(buckets []float64) {
	if buckets == nil {
		buckets = defaultListenerBuckets
	}

	tlmListener = telemetry.NewHistogram(
		"dogstatsd",
		"listener_read_latency",
		[]string{"listener_id", "transport", "listener_type"},
		"Time in nanoseconds while the listener is not reading data",
		buckets)
}

type listenerTelemetry struct {
	packetReadingErrors expvar.Int
	packets             expvar.Int
	bytes               expvar.Int
	expvars             *expvar.Map
	tlmPackets          telemetry.Counter
	tlmPacketsBytes     telemetry.Counter
}

func newListenerTelemetry(metricName string, name string) *listenerTelemetry {
	expvars := expvar.NewMap("dogstatsd-" + metricName)
	packetReadingErrors := expvar.Int{}
	packets := expvar.Int{}
	bytes := expvar.Int{}

	tlmPackets := telemetry.NewCounter("dogstatsd", metricName+"_packets",
		[]string{"state"}, fmt.Sprintf("Dogstatsd %s packets count", name))
	tlmPacketsBytes := telemetry.NewCounter("dogstatsd", metricName+"_packets_bytes",
		nil, fmt.Sprintf("Dogstatsd %s packets bytes count", name))
	expvars.Set("PacketReadingErrors", &packetReadingErrors)
	expvars.Set("Packets", &packets)
	expvars.Set("Bytes", &bytes)

	return &listenerTelemetry{
		expvars:             expvars,
		packetReadingErrors: packetReadingErrors,
		tlmPackets:          tlmPackets,
		packets:             packets,
		bytes:               bytes,
		tlmPacketsBytes:     tlmPacketsBytes,
	}
}

func (t *listenerTelemetry) onReadSuccess(n int) {
	t.packets.Add(1)
	t.tlmPackets.Inc("ok")
	t.bytes.Add(int64(n))
	t.tlmPacketsBytes.Add(float64(n))
}

func (t *listenerTelemetry) onReadError() {
	t.packets.Add(1)
	t.packetReadingErrors.Add(1)
	t.tlmPackets.Inc("error")
}
