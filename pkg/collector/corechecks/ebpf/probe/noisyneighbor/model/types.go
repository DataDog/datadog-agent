// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package model

// NoisyNeighborStats contains the statistics from the noisy neighbor check
type NoisyNeighborStats struct {
	PrevCgroupID   uint64
	CgroupID       uint64
	RunqLatencyNs  uint64
	TimestampNs    uint64
	PrevCgroupName string
	CgroupName     string
	Pid            uint64
	PrevPid        uint64
}
