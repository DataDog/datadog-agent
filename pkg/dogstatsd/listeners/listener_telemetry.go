package listeners

import (
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type listenerTelemetry struct {
	packetReadingErrors expvar.Int
	packets             expvar.Int
	bytes               expvar.Int
	tlmPackets          telemetry.Counter
	tlmPacketsBytes     telemetry.Counter
}

func newListenerTelemetry() *listenerTelemetry {
	expvars := expvar.NewMap("dogstatsd-named-pipe")
	packetReadingErrors := expvar.Int{}
	packets := expvar.Int{}
	bytes := expvar.Int{}

	tlmPackets := telemetry.NewCounter("dogstatsd", "named_pipe_packets",
		[]string{"state"}, "Dogstatsd named pipe packets count")
	tlmPacketsBytes := telemetry.NewCounter("dogstatsd", "named_pipe_packets_bytes",
		nil, "Dogstatsd named pipe packets bytes count")
	expvars.Set("PacketReadingErrors", &packetReadingErrors)
	expvars.Set("Packets", &packets)
	expvars.Set("Bytes", &bytes)

	return &listenerTelemetry{
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
	t.packetReadingErrors.Add(1)
	t.tlmPackets.Inc("error")
}
