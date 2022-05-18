package metric

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"gotest.tools/assert"
)

func TestAdd(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	timestamp := time.Now()
	add("a.super.metric", []string{"taga:valuea", "tagb:valueb"}, timestamp, demux)
	generatedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Equal(t, 1, len(generatedMetrics))
	metric := generatedMetrics[0]
	assert.Equal(t, metric.Name, "a.super.metric")
	assert.Equal(t, 2, len(metric.Tags))
	assert.Equal(t, float64(timestamp.UnixNano())/float64(time.Second), metric.Timestamp)
	assert.Equal(t, metric.Tags[0], "taga:valuea")
	assert.Equal(t, metric.Tags[1], "tagb:valueb")
}

func TestColdStart(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	timestamp := time.Now()
	ColdStart([]string{"taga:valuea", "tagb:valueb"}, timestamp, demux)
	generatedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Equal(t, 1, len(generatedMetrics))
	metric := generatedMetrics[0]
	assert.Equal(t, metric.Name, "gcp.run.enhanced.cold_start")
	assert.Equal(t, 2, len(metric.Tags))
	assert.Equal(t, metric.Tags[0], "taga:valuea")
	assert.Equal(t, metric.Tags[1], "tagb:valueb")
}

func TestShutdown(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	timestamp := time.Now()
	Shutdown([]string{"taga:valuea", "tagb:valueb"}, timestamp, demux)
	generatedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Equal(t, 1, len(generatedMetrics))
	metric := generatedMetrics[0]
	assert.Equal(t, metric.Name, "gcp.run.enhanced.shutdown")
	assert.Equal(t, 2, len(metric.Tags))
	assert.Equal(t, metric.Tags[0], "taga:valuea")
	assert.Equal(t, metric.Tags[1], "tagb:valueb")
}
