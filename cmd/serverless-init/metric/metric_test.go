// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	mockStartMetricName    = "datadog.serverless_agent.enhanced.cold_start"
	mockShutdownMetricName = "datadog.serverless_agent.enhanced.shutdown"
	mockMetricSource       = metrics.MetricSourceServerless
)

func TestAdd(t *testing.T) {
	demux := createDemultiplexer(t)
	mockAgent := serverlessMetrics.ServerlessMetricAgent{
		Demux: demux,
	}
	Add("a.super.metric", 1.0, mockMetricSource, mockAgent)
	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Equal(t, 0, len(timedMetrics))
	assert.Equal(t, 1, len(generatedMetrics))
	metric := generatedMetrics[0]
	assert.Equal(t, metric.Name, "a.super.metric")
	assert.Equal(t, 2, len(metric.Tags))
	assert.Equal(t, float64(timestamp.UnixNano())/float64(time.Second), metric.Timestamp)
	assert.Equal(t, metric.Tags[0].Value(), "taga:valuea")
	assert.Equal(t, metric.Tags[1].Value(), "tagb:valueb")
}

func TestAddStartMetric(t *testing.T) {
	demux := createDemultiplexer(t)
	mockAgent := serverlessMetrics.ServerlessMetricAgent{
		Demux: demux,
	}
	Add(mockStartMetricName, 1.0, mockMetricSource, mockAgent)
	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Equal(t, 0, len(timedMetrics))
	assert.Equal(t, 1, len(generatedMetrics))
	metric := generatedMetrics[0]
	assert.Equal(t, metric.Name, "gcp.run.enhanced.cold_start")
	assert.Equal(t, 2, len(metric.Tags))
	assert.Equal(t, metric.Tags[0].Value(), "taga:valuea")
	assert.Equal(t, metric.Tags[1].Value(), "tagb:valueb")
	assert.Equal(t, metric.Source, metrics.MetricSourceGoogleCloudRunEnhanced)
}

func TestAddShutdownMetric(t *testing.T) {
	demux := createDemultiplexer(t)
	mockAgent := serverlessMetrics.ServerlessMetricAgent{
		Demux: demux,
	}
	Add(mockShutdownMetricName, 1.0, mockMetricSource, mockAgent)
	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Equal(t, 0, len(timedMetrics))
	assert.Equal(t, 1, len(generatedMetrics))
	metric := generatedMetrics[0]
	assert.Equal(t, metric.Name, "gcp.run.enhanced.shutdown")
	assert.Equal(t, 2, len(metric.Tags))
	assert.Equal(t, metric.Tags[0].Value(), "taga:valuea")
	assert.Equal(t, metric.Tags[1].Value(), "tagb:valueb")
	assert.Equal(t, metric.Source, metrics.MetricSourceGoogleCloudRunEnhanced)
}

func TestNilDemuxDoesNotPanic(t *testing.T) {
	demux := createDemultiplexer(t)
	mockAgent := serverlessMetrics.ServerlessMetricAgent{
		Demux: nil, // Pass nil for demux to mimic when a port is blocked and dogstatsd does not start properly.
	}
	mockAgent.SetExtraTags([]string{"taga:valuea", "tagb:valueb"})
	// This previously led to a panic and segmentation fault
	Add("metric", 1.0, mockMetricSource, mockAgent)
	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Equal(t, 0, len(timedMetrics))
	assert.Equal(t, 0, len(generatedMetrics))
}

func createDemultiplexer(t *testing.T) demultiplexer.FakeSamplerMock {
	return fxutil.Test[demultiplexer.FakeSamplerMock](t, fx.Provide(func() log.Component { return logmock.New(t) }), logscompression.MockModule(), metricscompression.MockModule(), demultiplexerimpl.FakeSamplerMockModule(), hostnameimpl.MockModule())
}
