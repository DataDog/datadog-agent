// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import "github.com/DataDog/datadog-agent/pkg/dyninst/procmon"

// GetTrackedProcesses returns the set of processes that the scraper is
// tracking. This is a utility function for testing.
func (s *Scraper) GetTrackedProcesses() []procmon.ProcessID {
	s.mu.Lock()
	defer s.mu.Unlock()
	var processes []procmon.ProcessID
	for pid := range s.mu.processes {
		processes = append(processes, pid)
	}
	return processes
}

// NewScraperWithIRGenerator creates a new Scraper with a custom IR generator
// for testing.
func NewScraperWithIRGenerator[A Actuator[AT], AT ActuatorTenant](
	a A, d Dispatcher, loader Loader, irGenerator IRGenerator,
) *Scraper {
	return newScraper(a, d, loader, irGenerator)
}

type IRGeneratorImpl = irGenerator
