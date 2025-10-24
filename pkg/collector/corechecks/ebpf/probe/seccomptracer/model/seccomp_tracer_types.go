// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model is the types for the Seccomp Tracer check
package model

// StackTraceInfo represents a single stack trace with its count
type StackTraceInfo struct {
	StackID   int32    `json:"stackId"`
	Count     uint64   `json:"count"`
	Addresses []uint64 `json:"addresses"`
	Symbols   []string `json:"symbols,omitempty"`
}

// SeccompStatsEntry represents a single seccomp denial event with the count of the times it occurred
type SeccompStatsEntry struct {
	CgroupName    string           `json:"cgroupName"`
	SyscallNr     uint32           `json:"syscallNr"`
	SeccompAction uint32           `json:"seccompAction"`
	Pid           uint32           `json:"pid,omitempty"`
	Comm          string           `json:"comm,omitempty"`
	Count         uint64           `json:"count"`
	StackTraces   []StackTraceInfo `json:"stackTraces,omitempty"`
	DroppedStacks uint64           `json:"droppedStacks,omitempty"`
}

// SeccompStats is a slice of SeccompStatsEntry objects
type SeccompStats []SeccompStatsEntry
