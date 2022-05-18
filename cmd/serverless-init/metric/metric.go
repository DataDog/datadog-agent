package metric

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func ColdStart(tags []string, timestamp time.Time, demux aggregator.Demultiplexer) {
	add("gcp.run.enhanced.cold_start", tags, time.Now(), demux)
}

func Shutdown(tags []string, timestamp time.Time, demux aggregator.Demultiplexer) {
	add("gcp.run.enhanced.shutdown", tags, time.Now(), demux)
}

func add(name string, tags []string, timestamp time.Time, demux aggregator.Demultiplexer) {
	metricTimestamp := float64(timestamp.UnixNano()) / float64(time.Second)
	demux.AddTimeSample(metrics.MetricSample{
		Name:       name,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  metricTimestamp,
	})
}
