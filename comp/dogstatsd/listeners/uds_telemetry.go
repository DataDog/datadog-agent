// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type udsTelemetry struct {
	expvars                  *expvar.Map
	readingErrors            expvar.Int
	originDetectionErrors    expvar.Int
	packets                  expvar.Int
	bytes                    expvar.Int
	tlmPackets               telemetry.Counter
	tlmPacketsBytes          telemetry.Counter
	tlmOriginDetectionErrors telemetry.Counter
	tlmConnections           telemetry.Gauge
}

func newUDSTelemetry() *udsTelemetry {
	expvars := expvar.NewMap("dogstatsd-uds")
	udsOriginDetectionErrors := expvar.Int{}
	udsPacketReadingErrors := expvar.Int{}
	udsPackets := expvar.Int{}
	udsBytes := expvar.Int{}

	tlmUDSPackets := telemetry.NewCounter("dogstatsd", "uds_packets",
		[]string{"listener_id", "transport", "state"}, "Dogstatsd UDS packets count")
	tlmUDSOriginDetectionError := telemetry.NewCounter("dogstatsd", "uds_origin_detection_error",
		[]string{"listener_id", "transport"}, "Dogstatsd UDS origin detection error count")
	tlmUDSPacketsBytes := telemetry.NewCounter("dogstatsd", "uds_packets_bytes",
		[]string{"listener_id", "transport"}, "Dogstatsd UDS packets bytes")
	tlmUDSConnections := telemetry.NewGauge("dogstatsd", "uds_connections",
		[]string{"listener_id", "transport"}, "Dogstatsd UDS connections count")

	expvars.Set("OriginDetectionErrors", &udsOriginDetectionErrors)
	expvars.Set("PacketReadingErrors", &udsPacketReadingErrors)
	expvars.Set("Packets", &udsPackets)
	expvars.Set("Bytes", &udsBytes)

	return &udsTelemetry{
		expvars:                  expvars,
		readingErrors:            udsPacketReadingErrors,
		originDetectionErrors:    udsOriginDetectionErrors,
		packets:                  udsPackets,
		bytes:                    udsBytes,
		tlmPackets:               tlmUDSPackets,
		tlmPacketsBytes:          tlmUDSPacketsBytes,
		tlmOriginDetectionErrors: tlmUDSOriginDetectionError,
		tlmConnections:           tlmUDSConnections,
	}
}

func (t *udsTelemetry) incrementConnection(id, transport string) {
	t.tlmConnections.Inc(id, transport)
}

func (t *udsTelemetry) decrementConnection(id, transport string) {
	t.tlmConnections.Dec(id, transport)
}

func (t *udsTelemetry) onDetectionError(id, transport string) {
	t.packets.Add(1)
	t.originDetectionErrors.Add(1)
	t.tlmOriginDetectionErrors.Inc(id, transport)
}

func (t *udsTelemetry) onReadError(id, transport string) {
	t.packets.Add(1)
	t.readingErrors.Add(1)
	t.tlmPackets.Inc(id, transport, "error")
}

func (t *udsTelemetry) onReadSuccess(id, transport string, n int) {
	t.packets.Add(1)
	t.tlmPackets.Inc(id, transport, "ok")
	t.bytes.Add(int64(n))
	t.tlmPacketsBytes.Add(float64(n), id, transport)
}

func (t *udsTelemetry) clearTelemetry(id, transport string) {
	t.tlmConnections.Delete(id, transport)
	t.tlmPackets.Delete(id, transport, "error")
	t.tlmPackets.Delete(id, transport, "ok")
	t.tlmPacketsBytes.Delete(id, transport)
}
