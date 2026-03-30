// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

// SocketContentionStats contains aggregate lock-contention timings for stage 2.
type SocketContentionStats struct {
	TotalTimeNS uint64 `json:"total_time_ns"`
	MinTimeNS   uint64 `json:"min_time_ns"`
	MaxTimeNS   uint64 `json:"max_time_ns"`
	Count       uint32 `json:"count"`
	Flags       uint32 `json:"flags"`
}
