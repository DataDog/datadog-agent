// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model is the types for the Seccomp Tracer check
package model

// SeccompStatsKey is the type of the `SeccompStats` map key
type SeccompStatsEntry struct {
	CgroupName    string `json:"cgroupName"`
	SyscallNr     uint32 `json:"syscallNr"`
	SeccompAction uint32 `json:"seccompAction"`
	Count         uint64 `json:"count"`
}

// SeccompStats is the map of seccomp denials per container, syscall, and action
type SeccompStats []SeccompStatsEntry
