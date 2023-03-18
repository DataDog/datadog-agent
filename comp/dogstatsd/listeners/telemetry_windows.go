// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package listeners

import (
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

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
