// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package core

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=network_mock_linux.go

// NetworkCollector defines the interface for collecting network statistics.
type NetworkCollector interface {
	Close()
	GetStats(pids PidSet) (map[uint32]NetworkStats, error)
}
