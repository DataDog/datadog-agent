// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model is the types for the TCP Queue Length check
package model

// TCPQueueLengthStatsKey is the type of the `TCPQueueLengthStats` map key: the container ID
type TCPQueueLengthStatsKey struct {
	CgroupName string `json:"cgroupName"`
}

// TCPQueueLengthStatsValue is the type of the `TCPQueueLengthStats` map value: the maximum fill rate of busiest read and write buffers
type TCPQueueLengthStatsValue struct {
	ReadBufferMaxUsage  uint32 `json:"read_buffer_max_usage"`
	WriteBufferMaxUsage uint32 `json:"write_buffer_max_usage"`
}

// TCPQueueLengthStats is the map of the maximum fill rate of the read and write buffers per container
type TCPQueueLengthStats map[string]TCPQueueLengthStatsValue
