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

// TestPartitionTags_SingleDimension tests the common case where anomalies vary
// along a single dimension (e.g., different disk devices on the same host).
// This validates that both device and device_name are correctly identified as varying.
func TestPartitionTags_SingleDimension(t *testing.T) {
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/",
				"device_name": "disk3s1s1",
			},
			Severity:  0.8,
			Direction: "decrease",
			Magnitude: 1000000000,
		},
		{
			Timestamp: now.Add(1 * time.Second),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/System/Volumes/Data",
				"device_name": "disk3s5",
			},
			Severity:  0.85,
			Direction: "decrease",
			Magnitude: 2000000000,
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/System/Volumes/Preboot",
				"device_name": "disk3s2",
			},
			Severity:  0.75,
			Direction: "decrease",
			Magnitude: 500000000,
		},
		{
			Timestamp: now.Add(3 * time.Second),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/System/Volumes/VM",
				"device_name": "disk3s4",
			},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 3000000000,
		},
		{
			Timestamp: now.Add(4 * time.Second),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/System/Volumes/Update",
				"device_name": "disk3s6",
			},
			Severity:  0.82,
			Direction: "increase",
			Magnitude: 1500000000,
		},
		{
			Timestamp: now.Add(5 * time.Second),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/private/var/vm",
				"device_name": "disk3s3",
			},
			Severity:  0.88,
			Direction: "increase",
			Magnitude: 2500000000,
		},
	}

	partition := PartitionTags(events)

	// Both device and device_name should be varying (different values across events)
	assert.Empty(t, partition.ConstantTags, "Expected no constant tags")
	assert.Contains(t, partition.VaryingTags, "device", "device should be varying")
	assert.Contains(t, partition.VaryingTags, "device_name", "device_name should be varying")

	// Verify we captured all the distinct values
	assert.ElementsMatch(t, partition.VaryingTags["device"],
		[]string{"/", "/System/Volumes/Data", "/System/Volumes/Preboot", "/System/Volumes/VM", "/System/Volumes/Update", "/private/var/vm"},
		"Should contain all distinct device values")
	assert.ElementsMatch(t, partition.VaryingTags["device_name"],
		[]string{"disk3s1s1", "disk3s5", "disk3s2", "disk3s4", "disk3s6", "disk3s3"},
		"Should contain all distinct device_name values")
}

// TestPartitionTags_MultiDimension tests anomalies that vary along multiple dimensions.
// For example, issues affecting different devices on different hosts.
// Both dimensions should be identified as varying.
func TestPartitionTags_MultiDimension(t *testing.T) {
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"host":   "web-01",
				"device": "/",
			},
			Severity:  0.7,
			Direction: "decrease",
		},
		{
			Timestamp: now.Add(1 * time.Second),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"host":   "web-02",
				"device": "/data",
			},
			Severity:  0.8,
			Direction: "decrease",
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"host":   "web-03",
				"device": "/var",
			},
			Severity:  0.75,
			Direction: "decrease",
		},
	}

	partition := PartitionTags(events)

	// Both host and device vary, so no constant tags
	assert.Empty(t, partition.ConstantTags, "Expected no constant tags")
	assert.Contains(t, partition.VaryingTags, "host", "host should be varying")
	assert.Contains(t, partition.VaryingTags, "device", "device should be varying")

	assert.ElementsMatch(t, partition.VaryingTags["host"],
		[]string{"web-01", "web-02", "web-03"},
		"Should contain all distinct host values")
	assert.ElementsMatch(t, partition.VaryingTags["device"],
		[]string{"/", "/data", "/var"},
		"Should contain all distinct device values")
}

// TestPartitionTags_MixedConstantVarying tests the critical case where some tags
// are constant (providing context) while others vary (defining the dimension).
// This is the most common real-world scenario.
func TestPartitionTags_MixedConstantVarying(t *testing.T) {
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.cpu.user",
			Tags: map[string]string{
				"env":          "prod",
				"region":       "us-east-1",
				"container_id": "abc123",
			},
			Severity:  0.6,
			Direction: "increase",
		},
		{
			Timestamp: now.Add(1 * time.Second),
			Metric:    "system.cpu.user",
			Tags: map[string]string{
				"env":          "prod",
				"region":       "us-east-1",
				"container_id": "def456",
			},
			Severity:  0.65,
			Direction: "increase",
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.cpu.user",
			Tags: map[string]string{
				"env":          "prod",
				"region":       "us-east-1",
				"container_id": "ghi789",
			},
			Severity:  0.7,
			Direction: "increase",
		},
	}

	partition := PartitionTags(events)

	// env and region are constant (same across all events)
	assert.Len(t, partition.ConstantTags, 2, "Expected 2 constant tags")
	assert.Equal(t, "prod", partition.ConstantTags["env"], "env should be constant with value 'prod'")
	assert.Equal(t, "us-east-1", partition.ConstantTags["region"], "region should be constant with value 'us-east-1'")

	// container_id varies
	assert.Len(t, partition.VaryingTags, 1, "Expected 1 varying tag")
	assert.Contains(t, partition.VaryingTags, "container_id", "container_id should be varying")
	assert.ElementsMatch(t, partition.VaryingTags["container_id"],
		[]string{"abc123", "def456", "ghi789"},
		"Should contain all distinct container_id values")
}

// TestPartitionTags_NoTags tests the edge case where events have no tags.
// This should return empty partitions without errors.
func TestPartitionTags_NoTags(t *testing.T) {
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.cpu.idle",
			Tags:      map[string]string{},
			Severity:  0.5,
			Direction: "decrease",
		},
		{
			Timestamp: now.Add(1 * time.Second),
			Metric:    "system.cpu.idle",
			Tags:      nil, // nil tags should be handled gracefully
			Severity:  0.55,
			Direction: "decrease",
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.cpu.idle",
			Tags:      map[string]string{},
			Severity:  0.6,
			Direction: "decrease",
		},
	}

	partition := PartitionTags(events)

	// No tags means empty partition
	assert.Empty(t, partition.ConstantTags, "Expected no constant tags")
	assert.Empty(t, partition.VaryingTags, "Expected no varying tags")
}

// TestPartitionTags_SingleEvent tests the degenerate case of a single event.
// With only one event, all its tags should be considered constant (there's
// nothing to vary against).
func TestPartitionTags_SingleEvent(t *testing.T) {
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.mem.used",
			Tags: map[string]string{
				"host":   "db-primary",
				"region": "eu-west-1",
				"env":    "prod",
			},
			Severity:  0.9,
			Direction: "increase",
		},
	}

	partition := PartitionTags(events)

	// All tags should be constant (no variation possible with single event)
	assert.Len(t, partition.ConstantTags, 3, "Expected 3 constant tags")
	assert.Equal(t, "db-primary", partition.ConstantTags["host"])
	assert.Equal(t, "eu-west-1", partition.ConstantTags["region"])
	assert.Equal(t, "prod", partition.ConstantTags["env"])
	assert.Empty(t, partition.VaryingTags, "Expected no varying tags")
}

// TestPartitionTags_PartialTags tests the case where a tag key appears on some
// events but not others. Such tags should be treated as varying (present vs absent
// is a form of variation).
func TestPartitionTags_PartialTags(t *testing.T) {
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.net.bytes_sent",
			Tags: map[string]string{
				"interface": "eth0",
				"host":      "web-01",
			},
			Severity:  0.7,
			Direction: "increase",
		},
		{
			Timestamp: now.Add(1 * time.Second),
			Metric:    "system.net.bytes_sent",
			Tags: map[string]string{
				"interface": "eth1",
				"host":      "web-01",
				"vlan":      "100", // Only this event has vlan tag
			},
			Severity:  0.75,
			Direction: "increase",
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.net.bytes_sent",
			Tags: map[string]string{
				"interface": "eth2",
				"host":      "web-01",
			},
			Severity:  0.8,
			Direction: "increase",
		},
	}

	partition := PartitionTags(events)

	// host is constant (same value across all events)
	assert.Len(t, partition.ConstantTags, 1, "Expected 1 constant tag")
	assert.Equal(t, "web-01", partition.ConstantTags["host"], "host should be constant")

	// interface varies (different values), vlan varies (present vs absent)
	assert.Len(t, partition.VaryingTags, 2, "Expected 2 varying tags")
	assert.Contains(t, partition.VaryingTags, "interface", "interface should be varying")
	assert.Contains(t, partition.VaryingTags, "vlan", "vlan should be varying (partial presence)")

	assert.ElementsMatch(t, partition.VaryingTags["interface"],
		[]string{"eth0", "eth1", "eth2"},
		"Should contain all distinct interface values")
	assert.ElementsMatch(t, partition.VaryingTags["vlan"],
		[]string{"100"},
		"Should contain vlan value (even though only present on one event)")
}

// TestPartitionTags_RealDiskScenario tests a realistic scenario matching the demo
// output: 6 disk anomalies across APFS volumes on a macOS system.
// This validates the entire pipeline with real-world metric names and tag patterns.
func TestPartitionTags_RealDiskScenario(t *testing.T) {
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/",
				"device_name": "disk3s1s1",
			},
			Severity:  0.82,
			Direction: "decrease",
			Magnitude: 5368709120,
		},
		{
			Timestamp: now.Add(1 * time.Minute),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/System/Volumes/Data",
				"device_name": "disk3s5",
			},
			Severity:  0.85,
			Direction: "decrease",
			Magnitude: 32212254720,
		},
		{
			Timestamp: now.Add(2 * time.Minute),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/System/Volumes/Preboot",
				"device_name": "disk3s2",
			},
			Severity:  0.78,
			Direction: "decrease",
			Magnitude: 2147483648,
		},
		{
			Timestamp: now.Add(3 * time.Minute),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/System/Volumes/VM",
				"device_name": "disk3s4",
			},
			Severity:  0.91,
			Direction: "increase",
			Magnitude: 8589934592,
		},
		{
			Timestamp: now.Add(4 * time.Minute),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/System/Volumes/Update",
				"device_name": "disk3s6",
			},
			Severity:  0.87,
			Direction: "increase",
			Magnitude: 4294967296,
		},
		{
			Timestamp: now.Add(5 * time.Minute),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/private/var/vm",
				"device_name": "disk3s3",
			},
			Severity:  0.89,
			Direction: "increase",
			Magnitude: 6442450944,
		},
	}

	partition := PartitionTags(events)

	// No constant tags - all anomalies are on different devices
	assert.Empty(t, partition.ConstantTags,
		"Expected no constant tags (anomalies span different devices)")

	// Both device and device_name should be varying
	assert.Len(t, partition.VaryingTags, 2, "Expected 2 varying tag keys")
	assert.Contains(t, partition.VaryingTags, "device", "device should be varying")
	assert.Contains(t, partition.VaryingTags, "device_name", "device_name should be varying")

	// Verify all distinct values are captured
	assert.ElementsMatch(t, partition.VaryingTags["device"],
		[]string{"/", "/System/Volumes/Data", "/System/Volumes/Preboot", "/System/Volumes/VM", "/System/Volumes/Update", "/private/var/vm"},
		"Should contain all 6 distinct device mount points")
	assert.ElementsMatch(t, partition.VaryingTags["device_name"],
		[]string{"disk3s1s1", "disk3s5", "disk3s2", "disk3s4", "disk3s6", "disk3s3"},
		"Should contain all 6 distinct device names")
}
