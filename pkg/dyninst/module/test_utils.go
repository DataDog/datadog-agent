// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

// DiagnosticsStates returns the diagnostics states for the controller.
func (m *Module) DiagnosticsStates() map[string]map[string][]string {
	var states = make(map[string]map[string][]string)
	for _, t := range []*diagnosticTracker{
		m.diagnostics.received,
		m.diagnostics.installed,
		m.diagnostics.emitted,
		m.diagnostics.errors,
	} {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.mu.btree.Ascend(func(item diagnosticItem) bool {
			m, ok := states[item.key.runtimeID]
			if !ok {
				m = make(map[string][]string)
				states[item.key.runtimeID] = m
			}
			m[item.key.probeID] = append(m[item.key.probeID], t.name)
			return true
		})
	}
	return states
}
