// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf && test

package connection

import "github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"

// ProbeRunning reports whether this tracer's manager has a running probe for the given eBPF function.
func (t *ebpfTracer) ProbeRunning(name probes.ProbeFuncName) bool {
	for _, p := range t.m.Manager.Probes {
		if p.EBPFFuncName == name && p.IsRunning() {
			return true
		}
	}
	return false
}
