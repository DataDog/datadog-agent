// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// event represents an event in the state machine.
type event interface {
	event() // marker
	fmt.Stringer
}

type baseEvent struct{}

func (baseEvent) event() {}

type eventProcessesUpdated struct {
	baseEvent
	updated []ProcessUpdate
	removed []ProcessID
}

func (e eventProcessesUpdated) String() string {
	return fmt.Sprintf("eventProcessesUpdated{updated: %d, removed: %d}", len(e.updated), len(e.removed))
}

type eventProgramLoaded struct {
	baseEvent
	programID ir.ProgramID
	loaded    *loadedProgram
}

func (e eventProgramLoaded) String() string {
	return fmt.Sprintf("eventProgramLoaded{programID: %v}", e.programID)
}

type eventProgramLoadingFailed struct {
	baseEvent
	programID ir.ProgramID
}

func (e eventProgramLoadingFailed) String() string {
	return fmt.Sprintf("eventProgramLoadingFailed{programID: %v}", e.programID)
}

type eventProgramAttached struct {
	baseEvent
	program *attachedProgram
}

func (e eventProgramAttached) String() string {
	if e.program == nil {
		return "eventProgramAttached{program: nil}"
	}
	return fmt.Sprintf(
		"eventProgramAttached{programID: %v, processID: %v}",
		e.program.programID, e.program.processID,
	)
}

type eventProgramAttachingFailed struct {
	baseEvent
	programID ir.ProgramID
	processID ProcessID
}

func (e eventProgramAttachingFailed) String() string {
	return fmt.Sprintf(
		"eventProgramAttachingFailed{programID: %v, processID: %v}",
		e.programID, e.processID,
	)
}

// Note that we'll send this even if the detachment fails.
type eventProgramDetached struct {
	baseEvent
	programID ir.ProgramID
	processID ProcessID
}

func (e eventProgramDetached) String() string {
	return fmt.Sprintf(
		"eventProgramDetached{programID: %v, processID: %v}",
		e.programID, e.processID,
	)
}

// eventProgramUnloaded is emitted once a program has been fully unloaded
// (BPF program closed, sink closed, and unregistered from the dispatcher).
// It signals to the state-machine that the resources can now be dropped.
type eventProgramUnloaded struct {
	baseEvent
	programID ir.ProgramID
}

func (e eventProgramUnloaded) String() string {
	return fmt.Sprintf("eventProgramUnloaded{programID: %v}", e.programID)
}

type eventMissingTypesReported struct {
	baseEvent
	processID ProcessID
	typeNames []string
}

func (e eventMissingTypesReported) String() string {
	return fmt.Sprintf(
		"eventMissingTypesReported{processID: %v, typeNames: %d}",
		e.processID, len(e.typeNames),
	)
}

type eventShutdown struct {
	baseEvent
}

func (e eventShutdown) String() string {
	return "eventShutdown{}"
}

type eventGetMetrics struct {
	baseEvent
	metricsChan chan<- Metrics
}

func (e eventGetMetrics) String() string {
	return "eventGetMetrics{}"
}

type eventHeartbeatCheck struct {
	baseEvent
}

func (e eventHeartbeatCheck) String() string {
	return "eventHeartbeatCheck{}"
}
