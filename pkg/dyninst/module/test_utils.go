// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
)

// SetScraperUpdatesCallback installs a callback that will be called when the
// module processes updates from the rcscrape.Scraper.
func (m *Module) SetScraperUpdatesCallback(
	callback func(updates []rcscrape.ProcessUpdate),
) {
	m.testingKnobs.scraperUpdatesCallback = callback
}

// DiagnosticsStates returns the diagnostics states for the controller.
func (m *Module) DiagnosticsStates() map[string]map[string][]string {
	var states = make(map[string]map[string][]string)
	for _, t := range []*diagnosticTracker{
		m.diagnostics.received,
		m.diagnostics.installed,
		m.diagnostics.emitted,
		m.diagnostics.errors,
	} {
		t.byRuntimeID.Range(func(runtimeIDAny, probesAny interface{}) bool {
			runtimeID := runtimeIDAny.(string)
			m, ok := states[runtimeID]
			if !ok {
				m = make(map[string][]string)
				states[runtimeID] = m
			}
			probes := probesAny.(*sync.Map)
			probes.Range(func(probeIDAny, _ interface{}) bool {
				probeID := probeIDAny.(string)
				m[probeID] = append(m[probeID], t.name)
				return true
			})
			return true
		})
	}
	return states
}
