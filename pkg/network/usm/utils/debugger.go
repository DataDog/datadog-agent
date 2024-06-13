// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package utils contains common code shared across the USM codebase
package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	otherutils "github.com/DataDog/datadog-agent/cmd/system-probe/utils"
)

// Attacher is the interface that represents a PID attacher/detacher.
// It is used to attach/detach a PID to/from an eBPF program.
type Attacher interface {
	// AttachPID attaches the provided PID to the eBPF program.
	AttachPID(pid uint32) error
	// DetachPID detaches the provided PID from the eBPF program.
	DetachPID(pid uint32) error
}

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

var debugger *tlsDebugger

type tlsDebugger struct {
	mux        sync.Mutex
	registries []*FileRegistry
	attachers  map[string]Attacher
}

func (d *tlsDebugger) AddRegistry(r *FileRegistry) {
	d.mux.Lock()
	defer d.mux.Unlock()

	d.registries = append(d.registries, r)
}

func (d *tlsDebugger) GetTracedPrograms() []TracedProgram {
	d.mux.Lock()
	defer d.mux.Unlock()

	var all []TracedProgram

	// Iterate over each `FileRegistry` instance:
	// Examples of this would be: "shared_libraries", "istio", "goTLS" etc
	for _, registry := range d.registries {
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

// GetBlockedPathIDs returns a list of PathIdentifiers blocked in the
// registry for the specified program type.
func (d *tlsDebugger) GetBlockedPathIDs(programType string) []PathIdentifier {
	d.mux.Lock()
	defer d.mux.Unlock()

	for _, registry := range d.registries {
		if registry.telemetry.programName != programType {
			continue
		}

		registry.m.Lock()
		defer registry.m.Unlock()

		return registry.blocklistByID.Keys()
	}

	return nil
}

// AddAttacher adds an attacher to the debugger.
func (d *tlsDebugger) AddAttacher(name string, a Attacher) {
	d.mux.Lock()
	defer d.mux.Unlock()

	d.attachers[name] = a
}

// AddAttacher adds an attacher to the debugger.
// Used to wrap the internal debugger instance.
func AddAttacher(name string, a Attacher) {
	debugger.AddAttacher(name, a)
}

// attachRequestBody represents the request body for the attach/detach PID endpoint.
type attachRequestBody struct {
	PID  uint32 `json:"pid"`
	Type string `json:"type"`
}

// callbackType represents the type of callback to run.
type callbackType uint8

const (
	attach callbackType = iota
	detach
)

// String returns a string representation of the callback type.
func (m callbackType) String() string {
	switch m {
	case attach:
		return "attach"
	case detach:
		return "detach"
	default:
		return "unknown"
	}
}

// runAttacherCallback runs the attacher callback for the given request.
func (d *tlsDebugger) runAttacherCallback(w http.ResponseWriter, r *http.Request, mode callbackType) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Only POST requests are allowed")
		return
	}

	var reqBody attachRequestBody
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error decoding request body: %v", err)
		return
	}

	d.mux.Lock()
	attacher, ok := d.attachers[reqBody.Type]
	d.mux.Unlock()
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Module %q is not enabled", reqBody.Type)
		return
	}
	cb := attacher.AttachPID
	if mode == detach {
		cb = attacher.DetachPID
	}
	if err := cb(reqBody.PID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error %sing PID: %v", mode.String(), err)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s successfully %sed PID %d", reqBody.Type, mode.String(), reqBody.PID)
}

// AttachPIDEndpoint attaches a PID to an eBPF program.
func AttachPIDEndpoint(w http.ResponseWriter, r *http.Request) {
	debugger.runAttacherCallback(w, r, attach)
}

// DetachPIDEndpoint detaches a PID from an eBPF program.
func DetachPIDEndpoint(w http.ResponseWriter, r *http.Request) {
	debugger.runAttacherCallback(w, r, detach)
}

func init() {
	debugger = &tlsDebugger{
		attachers: make(map[string]Attacher),
	}
}

// GetTracedProgramList returns a list of traced programs.
func GetTracedProgramList() []TracedProgram {
	if debugger == nil {
		return nil
	}
	return debugger.GetTracedPrograms()
}
