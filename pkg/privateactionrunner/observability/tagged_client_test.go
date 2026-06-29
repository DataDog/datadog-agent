// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observability

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTaggedMetricsClient(t *testing.T) {
	tests := []struct {
		name        string
		fixedTags   []string
		wantWrapped bool
	}{
		{
			name:        "returns wrapped client when tags are nil",
			wantWrapped: true,
		},
		{
			name:        "returns wrapped client when tags are empty",
			fixedTags:   []string{},
			wantWrapped: true,
		},
		{
			name:      "returns tagged client when tags are provided",
			fixedTags: []string{"runner_id:runner-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := &recordingTaggedStatsdClient{}

			got := NewTaggedMetricsClient(wrapped, tt.fixedTags)

			if tt.wantWrapped {
				assert.Same(t, wrapped, got)
			} else {
				assert.NotSame(t, wrapped, got)
			}
		})
	}
}

func TestTaggedMetricsClientAppendsFixedTagsToMetricMethods(t *testing.T) {
	fixedTags := []string{
		"runner_id:runner-1",
		"runner_version:7.72.0",
		"mode_pull:true",
	}
	metricTags := []string{"action_fqn:com.datadoghq.http.sendRequest"}
	wantTags := []string{
		"action_fqn:com.datadoghq.http.sendRequest",
		"runner_id:runner-1",
		"runner_version:7.72.0",
		"mode_pull:true",
	}

	tests := []struct {
		name string
		emit func(statsd.ClientInterface, []string) error
	}{
		{
			name: "gauge",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Gauge("metric.gauge", 1.2, tags, 1)
			},
		},
		{
			name: "count",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Count("metric.count", 2, tags, 1)
			},
		},
		{
			name: "histogram",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Histogram("metric.histogram", 3.4, tags, 1)
			},
		},
		{
			name: "distribution",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Distribution("metric.distribution", 5.6, tags, 1)
			},
		},
		{
			name: "timing",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Timing("metric.timing", time.Second, tags, 1)
			},
		},
		{
			name: "time_in_milliseconds",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.TimeInMilliseconds("metric.time_in_milliseconds", 100, tags, 1)
			},
		},
		{
			name: "incr",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Incr("metric.incr", tags, 1)
			},
		},
		{
			name: "decr",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Decr("metric.decr", tags, 1)
			},
		},
		{
			name: "set",
			emit: func(c statsd.ClientInterface, tags []string) error {
				return c.Set("metric.set", "value", tags, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &recordingTaggedStatsdClient{}
			client := NewTaggedMetricsClient(recorder, fixedTags)

			require.NoError(t, tt.emit(client, metricTags))

			require.Len(t, recorder.calls, 1)
			assert.Equal(t, wantTags, recorder.calls[0].tags)
			assert.Equal(t, []string{"action_fqn:com.datadoghq.http.sendRequest"}, metricTags)
		})
	}
}

type taggedMetricCall struct {
	name string
	tags []string
}

type recordingTaggedStatsdClient struct {
	statsd.NoOpClient
	calls []taggedMetricCall
}

func (r *recordingTaggedStatsdClient) record(name string, tags []string) error {
	r.calls = append(r.calls, taggedMetricCall{
		name: name,
		tags: append([]string(nil), tags...),
	})
	return nil
}

func (r *recordingTaggedStatsdClient) Gauge(name string, _ float64, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) Count(name string, _ int64, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) Histogram(name string, _ float64, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) Distribution(name string, _ float64, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) Timing(name string, _ time.Duration, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) TimeInMilliseconds(name string, _ float64, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) Incr(name string, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) Decr(name string, tags []string, _ float64) error {
	return r.record(name, tags)
}

func (r *recordingTaggedStatsdClient) Set(name string, _ string, tags []string, _ float64) error {
	return r.record(name, tags)
}
