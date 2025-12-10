// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExtractMetricPattern_CommonPrefix(t *testing.T) {
	events := []AnomalyEvent{
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.8,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.7,
		},
	}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "system.disk", pattern.Family)
	assert.Equal(t, []string{"free", "used"}, pattern.Variants)
}

func TestExtractMetricPattern_DeeperNesting(t *testing.T) {
	events := []AnomalyEvent{
		{
			Timestamp: time.Now(),
			Metric:    "system.cpu.user.total",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.8,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.cpu.system.total",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.7,
		},
	}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "system.cpu", pattern.Family)
	assert.Equal(t, []string{"system.total", "user.total"}, pattern.Variants)
}

func TestExtractMetricPattern_SingleMetric(t *testing.T) {
	events := []AnomalyEvent{
		{
			Timestamp: time.Now(),
			Metric:    "system.load.1",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.8,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.load.1",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.7,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.load.1",
			Tags:      map[string]string{"host": "server3"},
			Severity:  0.6,
		},
	}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "system.load.1", pattern.Family)
	assert.Equal(t, []string{}, pattern.Variants)
}

func TestExtractMetricPattern_NoCommonPrefix(t *testing.T) {
	events := []AnomalyEvent{
		{
			Timestamp: time.Now(),
			Metric:    "system.cpu.user",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.8,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.7,
		},
	}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "system", pattern.Family)
	assert.Equal(t, []string{"cpu.user", "disk.free"}, pattern.Variants)
}

func TestExtractMetricPattern_IdenticalMetrics(t *testing.T) {
	events := []AnomalyEvent{
		{
			Timestamp: time.Now(),
			Metric:    "system.mem.used",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.8,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.mem.used",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.7,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.mem.used",
			Tags:      map[string]string{"host": "server3"},
			Severity:  0.6,
		},
	}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "system.mem.used", pattern.Family)
	assert.Equal(t, []string{}, pattern.Variants)
}

func TestExtractMetricPattern_EmptyEvents(t *testing.T) {
	events := []AnomalyEvent{}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "", pattern.Family)
	assert.Equal(t, []string{}, pattern.Variants)
}

func TestExtractMetricPattern_SingleEvent(t *testing.T) {
	events := []AnomalyEvent{
		{
			Timestamp: time.Now(),
			Metric:    "system.cpu.idle",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.8,
		},
	}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "system.cpu.idle", pattern.Family)
	assert.Equal(t, []string{}, pattern.Variants)
}

func TestExtractMetricPattern_RealDiskScenario(t *testing.T) {
	// Simulates the real scenario from the demo with 6 events
	// mixing system.disk.free and system.disk.used
	events := []AnomalyEvent{
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "web-1", "device": "/dev/sda1"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 5.0,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "web-1", "device": "/dev/sda1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 5.0,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "web-2", "device": "/dev/sdb1"},
			Severity:  0.85,
			Direction: "decrease",
			Magnitude: 4.8,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "web-2", "device": "/dev/sdb1"},
			Severity:  0.85,
			Direction: "increase",
			Magnitude: 4.8,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "web-3", "device": "/dev/sdc1"},
			Severity:  0.8,
			Direction: "decrease",
			Magnitude: 4.5,
		},
		{
			Timestamp: time.Now(),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "web-3", "device": "/dev/sdc1"},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 4.5,
		},
	}

	pattern := ExtractMetricPattern(events)

	assert.Equal(t, "system.disk", pattern.Family)
	assert.Equal(t, []string{"free", "used"}, pattern.Variants)
}
