// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"testing"
	"time"

	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
)

func TestIntegration_ConvertAnomaly(t *testing.T) {
	// Create an mh.Anomaly with known values
	now := time.Now().Unix()
	anomaly := mh.Anomaly{
		SeriesKey: mh.SeriesKey{
			Name: "system.disk.free",
			Tags: []string{"device:/dev/sda", "host:web-01", "env:prod"},
		},
		DetectorName: "bayesian_changepoint",
		Timestamp:    now,
		Type:         "changepoint",
		Severity:     0.64,
		Message:      "Changepoint detected (Bayesian): delta=12.00, direction=increase, probability=0.64",
	}

	// Convert to AnomalyEvent
	event := convertAnomaly(anomaly)

	// Verify all fields are correctly mapped
	if event.Timestamp.Unix() != now {
		t.Errorf("Expected timestamp %d, got %d", now, event.Timestamp.Unix())
	}

	if event.Metric != "system.disk.free" {
		t.Errorf("Expected metric 'system.disk.free', got '%s'", event.Metric)
	}

	if event.Severity != 0.64 {
		t.Errorf("Expected severity 0.64, got %f", event.Severity)
	}

	if event.Direction != "increase" {
		t.Errorf("Expected direction 'increase', got '%s'", event.Direction)
	}

	if event.Magnitude != 6.4 { // 0.64 * 10
		t.Errorf("Expected magnitude 6.4, got %f", event.Magnitude)
	}

	// Check tags
	expectedTags := map[string]string{
		"device": "/dev/sda",
		"host":   "web-01",
		"env":    "prod",
	}

	if len(event.Tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(event.Tags))
	}

	for key, expectedVal := range expectedTags {
		if actualVal, ok := event.Tags[key]; !ok {
			t.Errorf("Missing tag '%s'", key)
		} else if actualVal != expectedVal {
			t.Errorf("Tag '%s': expected '%s', got '%s'", key, expectedVal, actualVal)
		}
	}
}

func TestIntegration_ProcessSingleAnomaly(t *testing.T) {
	system := NewAnomalySummarySystem()
	now := time.Now()

	// Process one anomaly
	anomalies := []mh.Anomaly{
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.cpu.user",
				Tags: []string{"host:web-01"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Unix(),
			Type:         "changepoint",
			Severity:     0.5,
			Message:      "Changepoint detected: direction=increase",
		},
	}

	summaries := system.ProcessAnomalies(anomalies, now)

	// Should return no summaries yet (goes to pending)
	if len(summaries) != 0 {
		t.Errorf("Expected 0 summaries for single anomaly, got %d", len(summaries))
	}

	// Verify it's in pending
	pending := system.clusters.Pending()
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending event, got %d", len(pending))
	}
}

func TestIntegration_ProcessClusterableAnomalies(t *testing.T) {
	system := NewAnomalySummarySystem()
	now := time.Now()

	// Process two compatible anomalies (same metric family, close in time)
	anomalies := []mh.Anomaly{
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.cpu.user",
				Tags: []string{"host:web-01"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Unix(),
			Type:         "changepoint",
			Severity:     0.5,
			Message:      "Changepoint detected: direction=increase",
		},
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.cpu.system",
				Tags: []string{"host:web-01"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Add(5 * time.Second).Unix(),
			Type:         "changepoint",
			Severity:     0.4,
			Message:      "Changepoint detected: direction=increase",
		},
	}

	summaries := system.ProcessAnomalies(anomalies, now.Add(5*time.Second))

	// Should form cluster and return summary
	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary for clusterable anomalies, got %d", len(summaries))
	}

	if len(summaries) > 0 {
		summary := summaries[0]
		// Verify the headline contains the metric family
		if summary.Headline == "" {
			t.Error("Expected non-empty headline")
		}
		// Should mention "system.cpu"
		// Note: The exact format depends on implementation
	}

	// Verify cluster was created
	clusters := system.clusters.Clusters()
	if len(clusters) != 1 {
		t.Errorf("Expected 1 cluster, got %d", len(clusters))
	}

	if len(clusters) > 0 {
		cluster := clusters[0]
		if len(cluster.Events) != 2 {
			t.Errorf("Expected cluster with 2 events, got %d", len(cluster.Events))
		}
	}
}

func TestIntegration_ProcessDiskScenario(t *testing.T) {
	system := NewAnomalySummarySystem()
	now := time.Now()

	// Process 6 disk anomalies matching the demo output pattern:
	// - 3x system.disk.free (increase)
	// - 3x system.disk.used (decrease)
	// - Different devices
	// - All within 15 seconds
	anomalies := []mh.Anomaly{
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.disk.free",
				Tags: []string{"device:/System/Volumes/Data", "device_name:disk3s5"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Unix(),
			Type:         "changepoint",
			Severity:     0.64,
			Message:      "Changepoint detected (Bayesian): delta=12.00, direction=increase, probability=0.64",
		},
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.disk.used",
				Tags: []string{"device:/System/Volumes/Data", "device_name:disk3s5"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Add(2 * time.Second).Unix(),
			Type:         "changepoint",
			Severity:     0.64,
			Message:      "Changepoint detected (Bayesian): delta=-12.00, direction=decrease, probability=0.64",
		},
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.disk.free",
				Tags: []string{"device:/dev/disk3s1s1", "device_name:disk3s1s1"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Add(5 * time.Second).Unix(),
			Type:         "changepoint",
			Severity:     0.79,
			Message:      "Changepoint detected (Bayesian): delta=12.00, direction=increase, probability=0.79",
		},
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.disk.used",
				Tags: []string{"device:/dev/disk3s1s1", "device_name:disk3s1s1"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Add(7 * time.Second).Unix(),
			Type:         "changepoint",
			Severity:     0.79,
			Message:      "Changepoint detected (Bayesian): delta=-12.00, direction=decrease, probability=0.79",
		},
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.disk.free",
				Tags: []string{"device:/private/var/vm", "device_name:disk3s4"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Add(10 * time.Second).Unix(),
			Type:         "changepoint",
			Severity:     0.69,
			Message:      "Changepoint detected (Bayesian): delta=11.50, direction=increase, probability=0.69",
		},
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.disk.used",
				Tags: []string{"device:/private/var/vm", "device_name:disk3s4"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Add(12 * time.Second).Unix(),
			Type:         "changepoint",
			Severity:     0.69,
			Message:      "Changepoint detected (Bayesian): delta=-11.50, direction=decrease, probability=0.69",
		},
	}

	summaries := system.ProcessAnomalies(anomalies, now.Add(15*time.Second))

	// Should return 1 cluster summary
	if len(summaries) != 1 {
		t.Errorf("Expected 1 cluster summary for disk scenario, got %d", len(summaries))
	}

	if len(summaries) > 0 {
		summary := summaries[0]

		// Verify headline mentions disk and multiple devices
		if summary.Headline == "" {
			t.Error("Expected non-empty headline")
		}

		// Should mention something about disk and devices
		// Exact format depends on implementation, but should contain key info
		t.Logf("Headline: %s", summary.Headline)
		t.Logf("Details: %v", summary.Details)
		t.Logf("LikelyCause: %s", summary.LikelyCause)
	}

	// Verify cluster structure
	clusters := system.clusters.Clusters()
	if len(clusters) != 1 {
		t.Errorf("Expected 1 cluster, got %d", len(clusters))
	}

	if len(clusters) > 0 {
		cluster := clusters[0]
		if len(cluster.Events) != 6 {
			t.Errorf("Expected cluster with 6 events, got %d", len(cluster.Events))
		}

		// Verify metric pattern
		if cluster.Pattern.Family != "system.disk" {
			t.Errorf("Expected family 'system.disk', got '%s'", cluster.Pattern.Family)
		}

		// Should have both "free" and "used" variants
		if len(cluster.Pattern.Variants) != 2 {
			t.Errorf("Expected 2 variants, got %d", len(cluster.Pattern.Variants))
		}

		// Should have device as varying tag with 3 distinct values
		devices, ok := cluster.Pattern.VaryingTags["device"]
		if !ok {
			t.Error("Expected 'device' in varying tags")
		} else if len(devices) != 3 {
			t.Errorf("Expected 3 device values, got %d", len(devices))
		}
	}
}

func TestIntegration_TickUpdatesState(t *testing.T) {
	// Use custom config with short timeouts for testing
	cfg := ClusterConfig{
		TimeWindow:         30 * time.Second,
		StabilizingTimeout: 5 * time.Second,
		ResolvedTimeout:    10 * time.Second,
		ExpireTimeout:      20 * time.Second,
	}

	system := NewAnomalySummarySystemWithConfig(cfg)
	now := time.Now()

	// Process anomalies that will cluster
	anomalies := []mh.Anomaly{
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.cpu.user",
				Tags: []string{"host:web-01"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Unix(),
			Type:         "changepoint",
			Severity:     0.5,
			Message:      "Changepoint detected: direction=increase",
		},
		{
			SeriesKey: mh.SeriesKey{
				Name: "system.cpu.system",
				Tags: []string{"host:web-01"},
			},
			DetectorName: "bayesian_changepoint",
			Timestamp:    now.Add(2 * time.Second).Unix(),
			Type:         "changepoint",
			Severity:     0.4,
			Message:      "Changepoint detected: direction=increase",
		},
	}

	summaries := system.ProcessAnomalies(anomalies, now.Add(2*time.Second))

	// Should have 1 active cluster
	if len(summaries) != 1 {
		t.Fatalf("Expected 1 summary, got %d", len(summaries))
	}

	clusters := system.clusters.Clusters()
	if len(clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(clusters))
	}

	cluster := clusters[0]
	if cluster.State != Active {
		t.Errorf("Expected cluster state Active, got %v", cluster.State)
	}

	// Advance time and process empty list (simulate next flush)
	// After 6 seconds, should transition to Stabilizing
	summaries = system.ProcessAnomalies([]mh.Anomaly{}, now.Add(8*time.Second))

	clusters = system.clusters.Clusters()
	if len(clusters) != 1 {
		t.Fatalf("Expected cluster to still exist, got %d clusters", len(clusters))
	}

	cluster = clusters[0]
	if cluster.State != Stabilizing {
		t.Errorf("Expected cluster state Stabilizing after 6s, got %v", cluster.State)
	}

	// Should still return summary for Stabilizing cluster
	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary for stabilizing cluster, got %d", len(summaries))
	}

	// Advance time further - should transition to Resolved
	summaries = system.ProcessAnomalies([]mh.Anomaly{}, now.Add(15*time.Second))

	clusters = system.clusters.Clusters()
	if len(clusters) != 1 {
		t.Fatalf("Expected cluster to still exist, got %d clusters", len(clusters))
	}

	cluster = clusters[0]
	if cluster.State != Resolved {
		t.Errorf("Expected cluster state Resolved after 13s, got %v", cluster.State)
	}

	// Should NOT return summary for Resolved cluster (only Active/Stabilizing)
	if len(summaries) != 0 {
		t.Errorf("Expected 0 summaries for resolved cluster, got %d", len(summaries))
	}

	// Advance time past expiration - cluster should be removed
	summaries = system.ProcessAnomalies([]mh.Anomaly{}, now.Add(25*time.Second))

	clusters = system.clusters.Clusters()
	if len(clusters) != 0 {
		t.Errorf("Expected cluster to be expired, got %d clusters", len(clusters))
	}
}

func TestIntegration_TagParsing(t *testing.T) {
	// Anomaly with tags like ["device:/dev/sda", "host:web-01"]
	anomaly := mh.Anomaly{
		SeriesKey: mh.SeriesKey{
			Name: "system.disk.free",
			Tags: []string{"device:/dev/sda", "host:web-01", "env:production"},
		},
		DetectorName: "test_detector",
		Timestamp:    time.Now().Unix(),
		Type:         "test",
		Severity:     0.5,
		Message:      "Test message",
	}

	// Converted AnomalyEvent should have {"device": "/dev/sda", "host": "web-01", "env": "production"}
	event := convertAnomaly(anomaly)

	expectedTags := map[string]string{
		"device": "/dev/sda",
		"host":   "web-01",
		"env":    "production",
	}

	if len(event.Tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(event.Tags))
	}

	for key, expectedVal := range expectedTags {
		if actualVal, ok := event.Tags[key]; !ok {
			t.Errorf("Missing tag '%s'", key)
		} else if actualVal != expectedVal {
			t.Errorf("Tag '%s': expected '%s', got '%s'", key, expectedVal, actualVal)
		}
	}
}

func TestIntegration_DirectionParsing(t *testing.T) {
	testCases := []struct {
		name              string
		message           string
		expectedDirection string
	}{
		{
			name:              "increase",
			message:           "Changepoint detected: direction=increase",
			expectedDirection: "increase",
		},
		{
			name:              "decrease",
			message:           "Changepoint detected: direction=decrease",
			expectedDirection: "decrease",
		},
		{
			name:              "uppercase increase",
			message:           "Changepoint detected: direction=INCREASE",
			expectedDirection: "increase",
		},
		{
			name:              "no direction",
			message:           "Changepoint detected",
			expectedDirection: "",
		},
		{
			name:              "increase with delta",
			message:           "Changepoint detected (Bayesian): delta=12.00, direction=increase, probability=0.64",
			expectedDirection: "increase",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			anomaly := mh.Anomaly{
				SeriesKey: mh.SeriesKey{
					Name: "test.metric",
					Tags: []string{},
				},
				DetectorName: "test",
				Timestamp:    time.Now().Unix(),
				Type:         "test",
				Severity:     0.5,
				Message:      tc.message,
			}

			event := convertAnomaly(anomaly)

			if event.Direction != tc.expectedDirection {
				t.Errorf("Expected direction '%s', got '%s'", tc.expectedDirection, event.Direction)
			}
		})
	}
}
