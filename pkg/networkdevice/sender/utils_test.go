// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMetricKey(t *testing.T) {
	assert.Equal(t, "metric.name:k1,k2", GetMetricKey("metric.name", "k1", "k2"))
	assert.Equal(t, "metric.name:", GetMetricKey("metric.name"))
	assert.Equal(t, "m:single", GetMetricKey("m", "single"))
}

func TestSetNewSentTimestamp(t *testing.T) {
	ts := map[string]float64{}

	// first set
	SetNewSentTimestamp(ts, "key1", 100.0)
	assert.Equal(t, 100.0, ts["key1"])

	// newer timestamp updates
	SetNewSentTimestamp(ts, "key1", 200.0)
	assert.Equal(t, 200.0, ts["key1"])

	// older timestamp does not overwrite
	SetNewSentTimestamp(ts, "key1", 150.0)
	assert.Equal(t, 200.0, ts["key1"])
}

func TestShouldSendEntry(t *testing.T) {
	ms := NewSender(nil, "test", "ns")

	// no previous timestamp — should send
	assert.True(t, ms.ShouldSendEntry("key1", 100.0))

	// record a timestamp
	ms.UpdateTimestamps(map[string]float64{"key1": 100.0})

	// same timestamp — should not send
	assert.False(t, ms.ShouldSendEntry("key1", 100.0))

	// older timestamp — should not send
	assert.False(t, ms.ShouldSendEntry("key1", 50.0))

	// newer timestamp — should send
	assert.True(t, ms.ShouldSendEntry("key1", 200.0))
}

func TestGetDeviceTags(t *testing.T) {
	ms := NewSender(nil, "test", "myns")

	// no tags set — returns default tags
	tags := ms.GetDeviceTags("snmp_device", "10.0.0.1")
	assert.Contains(t, tags, "snmp_device:10.0.0.1")
	assert.Contains(t, tags, "device_namespace:myns")

	// with tags set
	ms.SetDeviceTagsMap(map[string][]string{
		"10.0.0.1": {"tag1:val1", "tag2:val2"},
	})
	tags = ms.GetDeviceTags("snmp_device", "10.0.0.1")
	assert.Equal(t, []string{"tag1:val1", "tag2:val2"}, tags)

	// unknown device still returns default
	tags = ms.GetDeviceTags("snmp_device", "10.0.0.99")
	assert.Contains(t, tags, "snmp_device:10.0.0.99")
}
