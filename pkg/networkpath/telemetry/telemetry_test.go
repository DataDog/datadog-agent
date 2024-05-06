package telemetry

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/stretchr/testify/assert"
)

func TestSubmitNetworkPathTelemetry(t *testing.T) {
	tests := []struct {
		name            string
		path            payload.NetworkPath
		checkDuration   time.Duration
		checkInterval   time.Duration
		tags            []string
		expectedMetrics []metricsender.MockReceivedMetric
	}{
		{
			name: "with hops and interval",
			path: payload.NetworkPath{
				Hops: []payload.NetworkPathHop{
					{Hostname: "hop_1", IPAddress: "1.1.1.1"},
					{Hostname: "hop_2", IPAddress: "1.1.1.2"},
				},
			},
			checkDuration: 10 * time.Second,
			checkInterval: 20 * time.Second,
			tags:          []string{"foo:bar", "tag2:val2"},
			expectedMetrics: []metricsender.MockReceivedMetric{
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.check_duration",
					Value:      float64(10),
					Tags:       []string{"foo:bar", "tag2:val2"},
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.check_interval",
					Value:      float64(20),
					Tags:       []string{"foo:bar", "tag2:val2"},
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.monitored",
					Value:      float64(1),
					Tags:       []string{"foo:bar", "tag2:val2"},
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.reachable",
					Value:      float64(0),
					Tags:       []string{"foo:bar", "tag2:val2"},
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.unreachable",
					Value:      float64(1),
					Tags:       []string{"foo:bar", "tag2:val2"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &metricsender.MockMetricSender{}
			SubmitNetworkPathTelemetry(sender, tt.path, tt.checkDuration, tt.checkInterval, tt.tags)
			assert.Equal(t, tt.expectedMetrics, sender.Metrics)
		})
	}
}
