// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package libproc provides bounded Darwin socket ownership snapshots.
package libproc

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// Limits bounds one host-wide libproc scan.
type Limits struct {
	MaxPIDs         int
	MaxFDsPerPID    int
	MaxObservations int
}

// DefaultLimits limits worst-case process and descriptor work while retaining
// enough capacity for ordinary hosts.
var DefaultLimits = Limits{
	MaxPIDs:         4096,
	MaxFDsPerPID:    4096,
	MaxObservations: 65536,
}

func (l Limits) validate() error {
	if l.MaxPIDs <= 0 || l.MaxFDsPerPID <= 0 || l.MaxObservations <= 0 {
		return errors.New("libproc scan limits must be positive")
	}
	return nil
}

// Observation is one direct process-to-socket binding.
type Observation struct {
	Tuple            network.ConnectionTuple
	PID              uint32
	ProcessStartTime uint64
}

// Snapshot is one bounded point-in-time scan.
type Snapshot struct {
	Observations []Observation
	Truncated    bool
}

// Scanner produces point-in-time socket ownership snapshots.
type Scanner interface {
	Scan() (Snapshot, error)
}
