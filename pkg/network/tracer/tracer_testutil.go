// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux && bpf

package tracer

import (
	"fmt"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
)

// RemoveClient stops tracking stateful data for a given client
func (t *Tracer) RemoveClient(clientID string) {
	t.state.RemoveClient(clientID)
}

// USMMonitor returns the USM monitor field
func (t *Tracer) USMMonitor() *usm.Monitor {
	return t.usmMonitor
}

// GetMap returns the map with the given name
func (t *Tracer) GetMap(name string) (*ebpf.Map, error) {
	return t.ebpfTracer.GetMap(name)
}

// ProbeRunning reports whether the connection tracer's manager has a running probe for the given eBPF function.
func (t *Tracer) ProbeRunning(name probes.ProbeFuncName) (bool, error) {
	pr, ok := t.ebpfTracer.(interface {
		ProbeRunning(probes.ProbeFuncName) bool
	})
	if !ok {
		return false, fmt.Errorf("%T does not expose its probes", t.ebpfTracer)
	}
	return pr.ProbeRunning(name), nil
}

// setMax sets how many closed NAT translations can be deferred; 0 deletes each one when its connection closes.
func (d *deferredTranslationDeletes) setMax(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.max = n
}
