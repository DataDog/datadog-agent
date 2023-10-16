// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpfless

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Monitor regroups all the work we want to do to monitor the probes we pushed in the kernel
type Monitor struct {
	probe *Probe
}

// NewMonitor returns a new instance of a ProbeMonitor
func NewMonitor(p *Probe) *Monitor {
	return &Monitor{
		probe: p,
	}
}

// Init initializes the monitor
func (m *Monitor) Init() error {
	return nil
}

// GetEventStreamMonitor returns the perf buffer monitor
/*func (m *Monitor) GetEventStreamMonitor() *eventstream.Monitor {
	return nil
}*/

// SendStats sends statistics about the probe to Datadog
func (m *Monitor) SendStats() error {
	return nil
}

// ProcessEvent processes an event through the various monitors and controllers of the probe
func (m *Monitor) ProcessEvent(event *model.Event) {}
