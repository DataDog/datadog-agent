// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package utils contains common code shared across the USM codebase
package utils

import (
	"net/http"
	"sync"

	otherutils "github.com/DataDog/datadog-agent/cmd/system-probe/utils"
)

// TracedProgram represents an active uprobe-based program and its used
// for the purposes of generating JSON content in our debugging endpoint
type TracedProgram struct {
	ProgramType string
	FilePath    string
	PIDs        []uint32
}

// BinaryTracer is an interface to be used by the debugger.
type BinaryTracer interface {
	GetTracedPrograms() []TracedProgram
}

// TracedProgramsEndpoint generates a summary of all active uprobe-based
// programs along with their file paths and PIDs.
// This is used for debugging purposes only.
func TracedProgramsEndpoint(w http.ResponseWriter, _ *http.Request) {
	otherutils.WriteAsJSON(w, Debugger.GetTracedPrograms())
}

// Debugger is a tool to expose at runtime all traced binary for USM.
var Debugger *binaryTracerDebugger

type binaryTracerDebugger struct {
	mux       sync.Mutex
	instances []BinaryTracer
}

// Add saves the given binary tracer in a list to later access for getting
// a list of its traced binaries.
func (d *binaryTracerDebugger) Add(b BinaryTracer) {
	d.mux.Lock()
	defer d.mux.Unlock()

	d.instances = append(d.instances, b)
}

// GetTracedPrograms returns all traced programs from all registered tracers.
func (d *binaryTracerDebugger) GetTracedPrograms() []TracedProgram {
	d.mux.Lock()
	defer d.mux.Unlock()

	var all []TracedProgram

	// Iterate over each `FileRegistry` instance:
	// Examples of this would be: "shared_libraries", "istio", "goTLS" etc
	for _, registry := range d.instances {
		all = append(all, registry.GetTracedPrograms()...)
	}

	return all
}

func init() {
	Debugger = new(binaryTracerDebugger)
}
