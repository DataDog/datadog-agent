// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
)

func TestIdentifyEvent(t *testing.T) {
	metricSample := []byte("_e{4,5}:title|text|#shell,bash")
	messageType := findMessageType(metricSample)
	assert.Equal(t, eventType, messageType)
}

func TestIdentifyServiceCheck(t *testing.T) {
	metricSample := []byte("_sc|NAME|STATUS|d:TIMESTAMP|h:HOSTNAME|#TAG_KEY_1:TAG_VALUE_1,TAG_2|m:SERVICE_CHECK_MESSAGE")
	messageType := findMessageType(metricSample)
	assert.Equal(t, serviceCheckType, messageType)
}

func TestIdentifyMetricSample(t *testing.T) {
	metricSample := []byte("song.length:240|h|@0.5")
	messageType := findMessageType(metricSample)
	assert.Equal(t, metricSampleType, messageType)
}

func TestIdentifyRandomString(t *testing.T) {
	metricSample := []byte("song.length:240|h|@0.5")
	messageType := findMessageType(metricSample)
	assert.Equal(t, metricSampleType, messageType)
}

func TestParseTags(t *testing.T) {
	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	p := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	rawTags := []byte("tag:test,mytag,good:boy")
	tags := p.parseTags(rawTags)
	expectedTags := []string{"tag:test", "mytag", "good:boy"}
	assert.ElementsMatch(t, expectedTags, tags)
}

func TestParseTagsEmpty(t *testing.T) {
	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	p := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	rawTags := []byte("")
	tags := p.parseTags(rawTags)
	assert.Nil(t, tags)
}

func TestUnsafeParseFloat(t *testing.T) {
	rawFloat := "1.1234"

	unsafeFloat, err := parseFloat64([]byte(rawFloat))
	assert.NoError(t, err)
	float, err := strconv.ParseFloat(rawFloat, 64)
	assert.NoError(t, err)

	assert.Equal(t, float, unsafeFloat)
}

func TestUnsafeParseFloatList(t *testing.T) {
	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	p := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	unsafeFloats, err := p.parseFloat64List([]byte("1.1234:21.5:13"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 3)
	assert.Equal(t, []float64{1.1234, 21.5, 13}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 1)
	assert.Equal(t, []float64{1.1234}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234:41:"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234::41"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte(":1.1234::41"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	_, err = p.parseFloat64List([]byte(""))
	assert.Error(t, err)
}

func TestUnsafeParseInt(t *testing.T) {
	rawInt := "123"

	unsafeInteger, err := parseInt64([]byte(rawInt))
	assert.NoError(t, err)
	integer, err := strconv.ParseInt(rawInt, 10, 64)
	assert.NoError(t, err)

	assert.Equal(t, integer, unsafeInteger)
}

func TestResolveContainerIDFromLocalData(t *testing.T) {
	const (
		localDataPrefix   = "c:"
		containerIDPrefix = "ci-"
		inodePrefix       = "in-"
		containerID       = "abcdef"
		containerInode    = "4242"
	)

	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	p := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)

	// Mock the provider to resolve the container ID from the inode
	mockProvider := mock.NewMetricsProvider()
	containerInodeUint, _ := strconv.ParseUint(containerInode, 10, 64)
	mockProvider.RegisterMetaCollector(&mock.MetaCollector{
		CIDFromInode: map[uint64]string{
			containerInodeUint: containerID,
		},
	})
	p.provider = mockProvider

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "Empty LocalData",
			input:    []byte(localDataPrefix),
			expected: []byte{},
		},
		{
			name:     "LocalData with new container ID",
			input:    []byte(localDataPrefix + containerIDPrefix + containerID),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData with old container ID format",
			input:    []byte(localDataPrefix + containerID),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData with inode",
			input:    []byte(localDataPrefix + inodePrefix + containerInode),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData with invalid inode",
			input:    []byte(localDataPrefix + inodePrefix + "invalid"),
			expected: []byte(nil),
		},
		{
			name:     "LocalData as a list",
			input:    []byte(localDataPrefix + containerIDPrefix + containerID + "," + inodePrefix + containerInode),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData as a list with only inode",
			input:    []byte(localDataPrefix + inodePrefix + containerInode),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData as a list with only container ID",
			input:    []byte(localDataPrefix + containerIDPrefix + containerID),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData as a list with only inode with trailing comma",
			input:    []byte(localDataPrefix + inodePrefix + containerInode + ","),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData as a list with only container ID with trailing comma",
			input:    []byte(localDataPrefix + containerIDPrefix + containerID + ","),
			expected: []byte(containerID),
		},
		{
			name:     "LocalData as a list with only inode surrounded by commas",
			input:    []byte(localDataPrefix + "," + inodePrefix + containerInode + ","), // This is an invalid format, but we should still be able to extract the container ID
			expected: []byte(containerID),
		},
		{
			name:     "LocalData as a list with only inode surrounded by commas",
			input:    []byte(localDataPrefix + "," + containerIDPrefix + containerID + ","), // This is an invalid format, but we should still be able to extract the container ID
			expected: []byte(containerID),
		},
		{
			name:     "LocalData as an invalid list",
			input:    []byte(localDataPrefix + ","),
			expected: []byte(nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, p.resolveContainerIDFromLocalData(tc.input))
		})
	}
}
