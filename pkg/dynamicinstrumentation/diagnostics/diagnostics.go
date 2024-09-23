// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package diagnostics provides a facility for dynamic instrumentation to upload diagnostic information
package diagnostics

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

func newDIDiagnostic(service, runtimeID, probeID string, status ditypes.Status) *ditypes.DiagnosticUpload {
	return &ditypes.DiagnosticUpload{
		Service:  service,
		DDSource: "dd_debugger",
		Debugger: struct {
			ditypes.Diagnostic `json:"diagnostics"`
		}{
			Diagnostic: ditypes.Diagnostic{
				RuntimeID: runtimeID,
				ProbeID:   probeID,
				Status:    status,
			},
		},
	}
}

type probeInstanceID struct {
	service   string
	runtimeID string
	probeID   string
}

// DiagnosticManager is used to keep track and upload diagnostic information
type DiagnosticManager struct {
	state   map[probeInstanceID]*ditypes.DiagnosticUpload
	Updates chan *ditypes.DiagnosticUpload

	mu sync.Mutex
}

// NewDiagnosticManager creates a new DiagnosticManager
func NewDiagnosticManager() *DiagnosticManager {
	return &DiagnosticManager{
		state:   make(map[probeInstanceID]*ditypes.DiagnosticUpload),
		Updates: make(chan *ditypes.DiagnosticUpload),
	}
}

// SetStatus associates the status with the specified service/probe
func (m *DiagnosticManager) SetStatus(service, runtimeID, probeID string, status ditypes.Status) {
	id := probeInstanceID{service, probeID, runtimeID}
	d := newDIDiagnostic(service, runtimeID, probeID, status)
	m.update(id, d)
}

// SetError associates the error with the specified service/probe
func (m *DiagnosticManager) SetError(service, runtimeID, probeID, errorType, errorMessage string) {
	id := probeInstanceID{service, probeID, runtimeID}
	d := newDIDiagnostic(service, runtimeID, probeID, ditypes.StatusError)
	d.SetError(errorType, errorMessage)
	m.update(id, d)
}

func (m *DiagnosticManager) update(id probeInstanceID, d *ditypes.DiagnosticUpload) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state[id] != d {
		m.state[id] = d
		// TODO: if there is no consumer reading updates, this blocks the calling goroutine
		m.Updates <- d
	}
}

// Diagnostics is a global instance of a diagnostic manager
var Diagnostics = NewDiagnosticManager()
