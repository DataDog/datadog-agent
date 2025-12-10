// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// containsAny checks if s contains any of the substrings
func containsAny(items []string, substrings ...string) bool {
	for _, item := range items {
		for _, substr := range substrings {
			if strings.Contains(item, substr) {
				return true
			}
		}
	}
	return false
}

// buildClusterPattern is a test helper to build cluster patterns
func buildClusterPattern(events []AnomalyEvent) ClusterPattern {
	return ClusterPattern{
		MetricPattern: ExtractMetricPattern(events),
		TagPartition:  PartitionTags(events),
	}
}

func TestSummarize_VaryingDimension(t *testing.T) {
	// Create a cluster with 6 events, varying device tag
	baseTime := time.Now()
	devices := []string{"disk3s1s1", "disk3s2", "disk3s4", "disk3s5", "disk3s6", "disk3s7"}

	events := make([]AnomalyEvent, len(devices))
	for i, device := range devices {
		events[i] = AnomalyEvent{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device": device,
				"env":    "prod",
			},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 100.0,
		}
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(5 * time.Second),
	}

	summary := Summarize(cluster)

	// Headline should mention the number of devices
	assert.Contains(t, summary.Headline, "6", "headline should mention 6 devices")
	assert.Contains(t, strings.ToLower(summary.Headline), "device", "headline should mention device dimension")

	// Headline should include metric family
	assert.Contains(t, summary.Headline, "system.disk", "headline should mention metric family")
}

func TestSummarize_SymmetryIncluded(t *testing.T) {
	// Create cluster with inverse symmetry (disk.free/disk.used)
	baseTime := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: baseTime,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"device": "disk1", "env": "prod"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 500.0,
		},
		{
			Timestamp: baseTime.Add(time.Second),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"device": "disk1", "env": "prod"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 480.0,
		},
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(time.Second),
	}

	symmetry := &SymmetryPattern{
		Type:       Inverse,
		Metrics:    [2]string{"system.disk.free", "system.disk.used"},
		Confidence: 0.95,
	}

	summary := SummarizeWithSymmetry(cluster, symmetry)

	// Details should include pattern description
	assert.True(t, containsAny(summary.Details, "inverse", "Inverse", "↑", "↓", "Pattern"),
		"details should include symmetry pattern information")

	// Should mention both metrics
	detailsStr := strings.Join(summary.Details, " ")
	assert.Contains(t, detailsStr, "free", "details should mention free metric")
	assert.Contains(t, detailsStr, "used", "details should mention used metric")
}

func TestSummarize_ConstantTags(t *testing.T) {
	// Cluster where all events have env:prod
	baseTime := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: baseTime,
			Metric:    "system.cpu.usage",
			Tags: map[string]string{
				"env":    "prod",
				"region": "us-east-1",
				"host":   "web-01",
			},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 50.0,
		},
		{
			Timestamp: baseTime.Add(time.Second),
			Metric:    "system.cpu.usage",
			Tags: map[string]string{
				"env":    "prod",
				"region": "us-east-1",
				"host":   "web-02",
			},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 48.0,
		},
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(time.Second),
	}

	summary := Summarize(cluster)

	// Details should mention constant tags
	detailsStr := strings.Join(summary.Details, " ")
	assert.Contains(t, detailsStr, "prod", "details should mention constant env tag")
	assert.Contains(t, detailsStr, "us-east-1", "details should mention constant region tag")
}

func TestSummarize_LikelyCause_Disk(t *testing.T) {
	// Disk cluster with inverse symmetry + multiple volumes
	baseTime := time.Now()
	devices := []string{"disk3s1s1", "disk3s2", "disk3s4"}

	events := make([]AnomalyEvent, 0)
	for _, device := range devices {
		events = append(events,
			AnomalyEvent{
				Timestamp: baseTime,
				Metric:    "system.disk.free",
				Tags:      map[string]string{"device": device, "fstype": "apfs"},
				Severity:  0.9,
				Direction: "increase",
				Magnitude: 500.0,
			},
			AnomalyEvent{
				Timestamp: baseTime.Add(time.Second),
				Metric:    "system.disk.used",
				Tags:      map[string]string{"device": device, "fstype": "apfs"},
				Severity:  0.9,
				Direction: "decrease",
				Magnitude: 480.0,
			},
		)
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(time.Second),
	}

	symmetry := &SymmetryPattern{
		Type:       Inverse,
		Metrics:    [2]string{"system.disk.free", "system.disk.used"},
		Confidence: 0.95,
	}

	summary := SummarizeWithSymmetry(cluster, symmetry)

	// LikelyCause should suggest volume operation
	assert.NotEmpty(t, summary.LikelyCause, "likely cause should not be empty for disk pattern")
	likelyCauseLower := strings.ToLower(summary.LikelyCause)
	assert.True(t,
		strings.Contains(likelyCauseLower, "volume") ||
			strings.Contains(likelyCauseLower, "apfs") ||
			strings.Contains(likelyCauseLower, "snapshot") ||
			strings.Contains(likelyCauseLower, "reclamation"),
		"likely cause should suggest volume/APFS operation: got %q", summary.LikelyCause)
}

func TestSummarize_LikelyCause_Unknown(t *testing.T) {
	// Generic cluster with no recognizable pattern
	baseTime := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: baseTime,
			Metric:    "custom.application.metric",
			Tags:      map[string]string{"service": "api"},
			Severity:  0.7,
			Direction: "increase",
			Magnitude: 30.0,
		},
		{
			Timestamp: baseTime.Add(time.Second),
			Metric:    "custom.application.other",
			Tags:      map[string]string{"service": "api"},
			Severity:  0.6,
			Direction: "decrease",
			Magnitude: 25.0,
		},
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(time.Second),
	}

	summary := Summarize(cluster)

	// LikelyCause should be empty (don't make up causes)
	assert.Empty(t, summary.LikelyCause, "likely cause should be empty for unknown patterns")
}

func TestSummarize_SingleEvent(t *testing.T) {
	// Cluster with just 1 event
	baseTime := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: baseTime,
			Metric:    "system.mem.used",
			Tags:      map[string]string{"host": "server-01"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 200.0,
		},
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime,
	}

	summary := Summarize(cluster)

	// Should still produce valid summary
	assert.NotEmpty(t, summary.Headline, "headline should not be empty for single event")

	// Headline should mention the metric
	assert.Contains(t, summary.Headline, "system.mem", "headline should mention metric")

	// Should mention the host
	assert.True(t,
		strings.Contains(summary.Headline, "server-01") || containsAny(summary.Details, "server-01"),
		"summary should mention the host")
}

func TestSummarize_NoTags(t *testing.T) {
	// Cluster with events that have no tags
	baseTime := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: baseTime,
			Metric:    "test.metric",
			Tags:      map[string]string{},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 100.0,
		},
		{
			Timestamp: baseTime.Add(time.Second),
			Metric:    "test.metric",
			Tags:      map[string]string{},
			Severity:  0.75,
			Direction: "increase",
			Magnitude: 95.0,
		},
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(time.Second),
	}

	// Should not crash
	summary := Summarize(cluster)

	// Should produce basic summary
	assert.NotEmpty(t, summary.Headline, "headline should not be empty even without tags")
	assert.Contains(t, summary.Headline, "test.metric", "headline should mention metric")
}

func TestSummarize_RealDiskScenario(t *testing.T) {
	// Full realistic scenario: 6 disk events, inverse symmetry
	baseTime := time.Now()
	devices := []string{"disk3s1s1", "disk3s2", "disk3s4", "disk3s5", "disk3s6", "disk3s7"}

	events := make([]AnomalyEvent, 0)
	for i, device := range devices {
		offset := time.Duration(i) * time.Second
		events = append(events,
			AnomalyEvent{
				Timestamp: baseTime.Add(offset),
				Metric:    "system.disk.free",
				Tags: map[string]string{
					"device": device,
					"env":    "prod",
					"fstype": "apfs",
				},
				Severity:  0.9,
				Direction: "increase",
				Magnitude: 1000.0 + float64(i*10),
			},
			AnomalyEvent{
				Timestamp: baseTime.Add(offset + 500*time.Millisecond),
				Metric:    "system.disk.used",
				Tags: map[string]string{
					"device": device,
					"env":    "prod",
					"fstype": "apfs",
				},
				Severity:  0.85,
				Direction: "decrease",
				Magnitude: 980.0 + float64(i*10),
			},
		)
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(time.Duration(len(devices)-1)*time.Second + 500*time.Millisecond),
	}

	symmetry := &SymmetryPattern{
		Type:       Inverse,
		Metrics:    [2]string{"system.disk.free", "system.disk.used"},
		Confidence: 0.98,
	}

	summary := SummarizeWithSymmetry(cluster, symmetry)

	// Verify headline makes sense
	assert.NotEmpty(t, summary.Headline, "headline should not be empty")
	assert.Contains(t, summary.Headline, "disk", "headline should mention disk")
	assert.Contains(t, summary.Headline, "6", "headline should mention 6 devices")

	// Verify details include key information
	detailsStr := strings.Join(summary.Details, " ")
	assert.NotEmpty(t, summary.Details, "details should not be empty")
	assert.True(t, containsAny(summary.Details, "inverse", "Inverse", "↑", "↓", "Pattern"),
		"details should mention symmetry pattern")
	assert.True(t, containsAny(summary.Details, "device", "disk3s1s1"),
		"details should mention devices")
	assert.Contains(t, detailsStr, "env", "details should mention constant env tag")

	// Verify likely cause makes sense
	assert.NotEmpty(t, summary.LikelyCause, "likely cause should not be empty for recognizable pattern")
	likelyCauseLower := strings.ToLower(summary.LikelyCause)
	assert.True(t,
		strings.Contains(likelyCauseLower, "volume") ||
			strings.Contains(likelyCauseLower, "apfs") ||
			strings.Contains(likelyCauseLower, "disk"),
		"likely cause should be relevant to disk operations: got %q", summary.LikelyCause)
}

func TestSummarize_MultipleVariants(t *testing.T) {
	// Cluster with system.disk.free, system.disk.used, system.disk.total
	baseTime := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: baseTime,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"device": "disk1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 500.0,
		},
		{
			Timestamp: baseTime.Add(time.Second),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"device": "disk1"},
			Severity:  0.85,
			Direction: "decrease",
			Magnitude: 480.0,
		},
		{
			Timestamp: baseTime.Add(2 * time.Second),
			Metric:    "system.disk.total",
			Tags:      map[string]string{"device": "disk1"},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 50.0,
		},
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime.Add(2 * time.Second),
	}

	summary := Summarize(cluster)

	// Details should list all variants
	detailsStr := strings.Join(summary.Details, " ")
	assert.True(t,
		strings.Contains(detailsStr, "free") || strings.Contains(summary.Headline, "free"),
		"should mention 'free' variant")
	assert.True(t,
		strings.Contains(detailsStr, "used") || strings.Contains(summary.Headline, "used"),
		"should mention 'used' variant")
	assert.True(t,
		strings.Contains(detailsStr, "total") || strings.Contains(summary.Headline, "total"),
		"should mention 'total' variant")
}

func TestSummarizeWithSymmetry_Nil(t *testing.T) {
	// Call with nil symmetry
	baseTime := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: baseTime,
			Metric:    "system.cpu.usage",
			Tags:      map[string]string{"host": "web-01"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 80.0,
		},
	}

	cluster := &AnomalyCluster{
		ID:        1,
		Events:    events,
		Pattern:   buildClusterPattern(events),
		FirstSeen: baseTime,
		LastSeen:  baseTime,
	}

	// Should not crash with nil symmetry
	summary := SummarizeWithSymmetry(cluster, nil)

	// Should produce valid summary without symmetry details
	assert.NotEmpty(t, summary.Headline, "headline should not be empty even with nil symmetry")
	assert.NotNil(t, summary.Details, "details should not be nil")

	// Should not mention symmetry-specific terms
	detailsStr := strings.Join(summary.Details, " ")
	assert.False(t,
		strings.Contains(strings.ToLower(detailsStr), "inverse") ||
			strings.Contains(strings.ToLower(detailsStr), "proportional") ||
			strings.Contains(detailsStr, "↑") ||
			strings.Contains(detailsStr, "↓"),
		"should not mention symmetry with nil symmetry pattern")
}
