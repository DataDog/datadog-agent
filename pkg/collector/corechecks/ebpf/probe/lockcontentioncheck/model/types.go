// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model contains the model for the lock contention check
package model

// LockContentionStats holds per-lock-type contention statistics
type LockContentionStats struct {
	LockType    string `json:"lock_type"`
	TotalTimeNs uint64 `json:"total_time_ns"`
	Count       uint64 `json:"count"`
	MaxTimeNs   uint64 `json:"max_time_ns"`
}
