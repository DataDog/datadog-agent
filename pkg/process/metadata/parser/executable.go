// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"path/filepath"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

var _ metadata.Extractor = &ProcessNameExtractor{}

// ProcessNameExtractor extracts process names from processes for use in
// network connection enrichment.
type ProcessNameExtractor struct {
	mu    sync.RWMutex
	names map[int32]string
}

// NewProcessNameExtractor creates a new ProcessNameExtractor.
func NewProcessNameExtractor() *ProcessNameExtractor {
	return &ProcessNameExtractor{}
}

// Extract populates the internal map with the executable name for each process.
func (e *ProcessNameExtractor) Extract(processes map[int32]*procutil.Process) {
	names := make(map[int32]string, len(processes))
	for pid, p := range processes {
		if p.Comm != "" {
			names[pid] = p.Comm
		} else if p.Exe != "" {
			names[pid] = filepath.Base(p.Exe)
		}
	}
	e.mu.Lock()
	e.names = names
	e.mu.Unlock()
}

// GetProcessName returns the process name for the given pid, or an empty
// string if the process is not known.
func (e *ProcessNameExtractor) GetProcessName(pid int32) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.names[pid]
}
