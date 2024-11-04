// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"github.com/shirou/gopsutil/v3/process"
)

// ignoreServices is a list of service names that should not be reported as a service.
var ignoreServices = map[string]struct{}{
	"datadog-agent": {},
}

// addIgnoredPid store excluded pid.
func (s *discovery) addIgnoredPid(pid int32) {
	s.mux.Lock()
	s.ignorePids[pid] = true
	s.mux.Unlock()
}

// shouldIgnorePid returns true if service should be excluded from handling.
func (s *discovery) shouldIgnorePid(pid int32) bool {
	s.mux.Lock()
	_, found := s.ignorePids[pid]
	s.mux.Unlock()

	return found
}

// shouldIgnoreService saves pid of the process if the service should be excluded from handling
// and returns true for such process.
func (s *discovery) shouldIgnoreService(name string, proc *process.Process) bool {
	s.mux.Lock()
	_, found := ignoreServices[name]
	if found {
		s.ignorePids[proc.Pid] = true
	}
	s.mux.Unlock()

	return found
}

// cleanIgnoredPids removes dead PIDs from the list of ignored processes.
func (s *discovery) cleanIgnoredPids(alivePids map[int32]struct{}) {
	s.mux.Lock()
	defer s.mux.Unlock()

	for pid := range s.ignorePids {
		if _, alive := alivePids[pid]; alive {
			continue
		}
		delete(s.ignorePids, pid)
	}
}
