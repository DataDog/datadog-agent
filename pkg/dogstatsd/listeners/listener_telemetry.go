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
