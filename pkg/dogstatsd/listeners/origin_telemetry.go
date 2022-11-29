package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmOriginBytes = telemetry.NewCounter("dogstatsd", "origin_bytes",
		[]string{"origin"}, "Bytes count per origin")
	tlmOriginPackets = telemetry.NewCounter("dogstatsd", "origin_packets",
		[]string{"origin"}, "Packets count per origin")
	tlmOriginTags = telemetry.NewCounter("dogstatsd", "origin_tags",
		[]string{"origin"}, "Tags count per origin")
	tlmOriginMetrics = telemetry.NewCounter("dogstatsd", "origin_metrics",
		[]string{"origin"}, "Metrics count per origin")
)

type OriginTelemetryMode string

const OriginTelemetrySimple OriginTelemetryMode = "simple"
const OriginTelemetryComplete OriginTelemetryMode = "complete"

// OriginTelemetryEntry is created while processing a packet, meaning we have
// one OriginTelemetryEntry per packet.
type OriginTelemetryEntry struct {
	Origin       string
	BytesCount   uint
	TagsCount    uint
	MetricsCount uint
}

type OriginTelemetryTracker struct {
	Mode         OriginTelemetryMode
	ch           chan OriginTelemetryEntry
	stopChan     chan bool
	packetsCount map[string]uint
	bytesCount   map[string]uint
	tagsCount    map[string]uint
	metricsCount map[string]uint
}

// StartOriginTelemetry reports origin telemetry using the internal telemetry system.
// We can consider storing a per origin rate in order to implement rate-limit
// in the future.
func StartOriginTelemetry(stopChan chan bool, mode OriginTelemetryMode) *OriginTelemetryTracker {
	trackingCh := make(chan OriginTelemetryEntry, 8192)
	tracker := &OriginTelemetryTracker{
		ch:           trackingCh,
		stopChan:     stopChan,
		packetsCount: make(map[string]uint),
		bytesCount:   make(map[string]uint),
		tagsCount:    make(map[string]uint),
		metricsCount: make(map[string]uint),
		Mode:         mode,
	}
	go func() {
		tracker.run(trackingCh)
	}()
	return tracker
}

func (t *OriginTelemetryTracker) processEntry(entry OriginTelemetryEntry) {
	tlmOriginBytes.Add(float64(entry.BytesCount), entry.Origin)
	tlmOriginPackets.Inc(entry.Origin)

	if t.Mode == OriginTelemetryComplete {
		tlmOriginTags.Add(float64(entry.TagsCount), entry.Origin)
		tlmOriginMetrics.Add(float64(entry.MetricsCount), entry.Origin)
	}
}

func countMetricsAndTags(b []byte) (uint, uint) {
	// results
	var metrics uint = 0
	var tags uint = 0

	// parser states
	var countingTag = false
	var enteringNew = true

	for _, c := range b {
		if enteringNew {
			// resets the parser and count a metric
			enteringNew = false
			countingTag = false
			metrics += 1
		}

		if c == '#' {
			// we're parsing a metric and we start parsing its tags
			countingTag = true
			// there is at least one (that's a simplification, there could be
			// an immediate | and no tags to count. Let's not mind about this
			// edge case to avoid a new parser state.
			tags += 1
		} else if countingTag && c == ',' {
			// we're currently couting tags and we parsed a ',', means we'll
			// start seeing a next one. Counts one (same simplification as above).
			tags += 1
		} else if c == '|' {
			// We're leaving a part of the metrics parsing, in all cases, it means
			// we are not counting tags anymore.
			countingTag = false
		} else if c == '\n' {
			// We're done with current metric
			enteringNew = true
		}
	}

	return metrics, tags
}

func (t *OriginTelemetryTracker) run(trackingCh chan OriginTelemetryEntry) {
	log.Debug("Starting the origin telemetry tracker")
	for {
		select {
		case entry := <-trackingCh:
			t.processEntry(entry)
		case <-t.stopChan:
			log.Debug("Closing the origin telemetry tracker")
			return
		}
	}
}
