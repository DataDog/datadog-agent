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
	for _, p := range s.mu.debouncer.processes {
		processes = append(processes, p.ProcessID)
	}
	return processes
}
