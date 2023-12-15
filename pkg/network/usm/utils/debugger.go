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

// TracedProgramsEndpoint generates a summary of all active uprobe-based
// programs along with their file paths and PIDs.
// This is used for debugging purposes only.
func TracedProgramsEndpoint(w http.ResponseWriter, _ *http.Request) {
	otherutils.WriteAsJSON(w, debugger.GetTracedPrograms())
}

var debugger *fileRegistryDebugger

type fileRegistryDebugger struct {
	mux       sync.Mutex
	instances []*FileRegistry
}

func (d *fileRegistryDebugger) Add(r *FileRegistry) {
	d.mux.Lock()
	defer d.mux.Unlock()

	d.instances = append(d.instances, r)
}

func (d *fileRegistryDebugger) GetTracedPrograms() []TracedProgram {
	d.mux.Lock()
	defer d.mux.Unlock()

	var all []TracedProgram

	// Iterate over each `FileRegistry` instance:
	// Examples of this would be: "shared_libraries", "istio", "goTLS" etc
	for _, registry := range d.instances {
		programType := registry.telemetry.programName
		tracedProgramsByID := make(map[PathIdentifier]*TracedProgram)

		registry.m.Lock()
		// First, "aggregate" PathIDs by PIDs
		for pid, pathSet := range registry.byPID {
			for pathID := range pathSet {
				tracedProgram, ok := tracedProgramsByID[pathID]
				if !ok {
					tracedProgram = new(TracedProgram)
					tracedProgramsByID[pathID] = tracedProgram
				}

				tracedProgram.PIDs = append(tracedProgram.PIDs, pid)
			}
		}

		// Then, enhance each PathID with a sample file path and the program type
		for pathID, program := range tracedProgramsByID {
			registration, ok := registry.byID[pathID]
			if !ok {
				continue
			}

			program.ProgramType = programType
			program.FilePath = registration.sampleFilePath
		}
		registry.m.Unlock()

		// Finally, add everything to a slice that is transformed in JSON
		// content by the endpoint handler
		for _, program := range tracedProgramsByID {
			all = append(all, *program)
		}
	}

	return all
}

func init() {
	debugger = new(fileRegistryDebugger)
}
