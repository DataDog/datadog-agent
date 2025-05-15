// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package sender provides a common interface for sending network device
// metrics and metadata to the Datadog Agent.
// It abstracts the underlying sender implementation, allowing for
// different sender types (e.g., for different network device integrations)
// to be used interchangeably.

//go:build test

package sender

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockTimeNow mocks time.Now
var mockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

func TestTimestampExpiration(t *testing.T) {
	TimeNow = mockTimeNow
	ms := NewSender(nil, "mock-integration", "test-ns")

	testTimestamps := map[string]float64{
		"test-id":   1000,
		"test-id-2": 946684700,
	}

	ms.UpdateTimestamps(testTimestamps)
	ms.expireTimeSent()

	// Assert "test-id" is expired
	require.Equal(t, map[string]float64{
		"test-id-2": 946684700,
	}, ms.lastTimeSent)
}
