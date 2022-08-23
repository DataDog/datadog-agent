// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

const (
	// DefaultStopTimeout is the default stop timeout in seconds
	DefaultStopTimeout = 5

	// DefaultAggregatorFlushInterval is the default flush interval in seconds
	DefaultAggregatorFlushInterval = 300 // 5min

	// DefaultAggregatorFlowContextTTL is the default flow context TTL in seconds
	DefaultAggregatorFlowContextTTL = 300 // 5min

	// DefaultAggregatorBufferSize is the default aggregator buffer size interval
	DefaultAggregatorBufferSize = 100

	// DefaultBindHost is the default bind host used for flow listeners
	DefaultBindHost = "0.0.0.0"
)
