package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// packet buffer
	tlmPacketsBufferFlushedTimer = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_timer",
		nil, "Count of packets buffer flush triggered by the timer")
	tlmPacketsBufferFlushedFull = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_full",
		nil, "Count of packets buffer flush triggered because the buffer is full")
	tlmPacketsChannelSize = telemetry.NewGauge("dogstatsd", "packets_channel_size",
		nil, "Number of packets in the packets channel")

	// packet pool
	tlmPacketPoolGet = telemetry.NewCounter("dogstatsd", "packet_pool_get",
		nil, "Count of get done in the packet pool")
	tlmPacketPoolPut = telemetry.NewCounter("dogstatsd", "packet_pool_put",
		nil, "Count of put done in the packet pool")
	tlmPacketPool = telemetry.NewGauge("dogstatsd", "packet_pool",
		nil, "Usage of the packet pool in dogstatsd")

	// UDP
	tlmUDPPackets = telemetry.NewCounter("dogstatsd", "udp_packets",
		[]string{"state"}, "Dogstatsd UDP packets count")
	tlmUDPPacketsBytes = telemetry.NewCounter("dogstatsd", "udp_packets_bytes",
		nil, "Dogstatsd UDP packets bytes count")

	// UDS
	tlmUDSPackets = telemetry.NewCounter("dogstatsd", "uds_packets",
		[]string{"state"}, "Dogstatsd UDS packets count")
	tlmUDSOriginDetectionError = telemetry.NewCounter("dogstatsd", "uds_origin_detection_error",
		nil, "Dogstatsd UDS origin detection error count")
	tlmUDSPacketsBytes = telemetry.NewCounter("dogstatsd", "uds_packets_bytes",
		nil, "Dogstatsd UDS packets bytes")

	tlmListenerChannel    telemetry.Histogram
	defaultChannelBuckets = []float64{250, 500, 750, 1000, 10000}

	tlmListener            telemetry.Histogram
	defaultListenerBuckets = []float64{300, 500, 1000, 1500, 2000, 2500, 3000, 10000, 20000, 50000}
)

func init() {
	get := func(option string, defaultData []float64) []float64 {
		if !config.Datadog.IsSet(option) {
			return defaultData
		}

		buckets, err := config.Datadog.GetFloat64SliceE(option)
		if err != nil {
			log.Errorf("%s, falling back to default values", err)
			return defaultData
		}
		if len(buckets) == 0 {
			log.Debugf("'%s' is empty, falling back to default values", option)
			return defaultData
		}
		return buckets
	}

	tlmListener = telemetry.NewHistogram(
		"dogstatsd",
		"listener_read_latency",
		[]string{"listener_type"},
		"Time in nanoseconds while the listener is not reading data",
		get("telemetry.dogstatsd.listeners_latency_buckets", defaultListenerBuckets))

	tlmListenerChannel = telemetry.NewHistogram(
		"dogstatsd",
		"listener_channel_latency",
		nil,
		"Time in nanoseconds to push a packets from a listeners to dogstatsd pipeline",
		get("telemetry.dogstatsd.listeners_channel_latency_buckets", defaultChannelBuckets))
}
