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

type eventProgramCompiled struct {
	baseEvent
	programID       ir.ProgramID
	compiledProgram *CompiledProgram
}

func (e eventProgramCompiled) String() string {
	return fmt.Sprintf("eventProgramCompiled{programID: %v}", e.programID)
}

type eventProgramCompilationFailed struct {
	baseEvent
	programID ir.ProgramID
	err       error
}

func (e eventProgramCompilationFailed) String() string {
	return fmt.Sprintf("eventProgramCompilationFailed{programID: %v, err: %v}", e.programID, e.err)
}

type eventProgramLoaded struct {
	baseEvent
	programID     ir.ProgramID
	loadedProgram *loadedProgram
}

func (e eventProgramLoaded) String() string {
	return fmt.Sprintf("eventProgramLoaded{programID: %v}", e.programID)
}

type eventProgramLoadingFailed struct {
	baseEvent
	programID ir.ProgramID
	err       error
}

func (e eventProgramLoadingFailed) String() string {
	return fmt.Sprintf("eventProgramLoadingFailed{programID: %v, err: %v}", e.programID, e.err)
}

type eventProgramAttached struct {
	baseEvent
	program *attachedProgram
}

func (e eventProgramAttached) String() string {
	if e.program == nil {
		return "eventProgramAttached{program: nil}"
	}
	return fmt.Sprintf("eventProgramAttached{programID: %v, processID: %v}", e.program.progID, e.program.procID)
}

type eventProgramAttachingFailed struct {
	baseEvent
	programID ir.ProgramID
	processID ProcessID
	err       error
}

func (e eventProgramAttachingFailed) String() string {
	return fmt.Sprintf("eventProgramAttachingFailed{programID: %v, processID: %v, err: %v}", e.programID, e.processID, e.err)
}

// Note that we'll send this even if the detachment fails.
type eventProgramDetached struct {
	baseEvent
	programID ir.ProgramID
	processID ProcessID
}

func (e eventProgramDetached) String() string {
	return fmt.Sprintf("eventProgramDetached{programID: %v, processID: %v}", e.programID, e.processID)
}

type eventShutdown struct {
	baseEvent
}

func (e eventShutdown) String() string {
	return "eventShutdown{}"
}
