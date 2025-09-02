// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package actuator

// Note that this separate file exists because generating the stringers
// to carry the build tags is a bit of a pain, so these files do not
// have the build tags.

//go:generate go run golang.org/x/tools/cmd/stringer -type=processState -trimprefix=processState

// processState represents the state of a process in the state machine.
type processState uint8

const (
	processStateInvalid processState = iota

	// The process is waiting for a program to be compiled and loaded.
	processStateWaitingForProgram

	// A program is currently in the process of being attached.
	processStateAttaching

	// A program is attached to the process.
	processStateAttached

	// A program is currently in the process of being detached.
	processStateDetaching

	// The program for this process failed to load.
	processStateLoadingFailed
)

//go:generate go run golang.org/x/tools/cmd/stringer -type=programState -trimprefix=programState

// programState represents the state of a program in the state machine.
type programState uint8

const (
	programStateInvalid programState = iota

	// The program is queued to be compiled.
	programStateQueued

	// The program is being loaded.
	programStateLoading

	// The program is loaded, and may be attached to processes.
	programStateLoaded

	// The program is being detached from all processes and unloaded
	// for removal.
	programStateDraining

	// The program has been detached from all processes and is now
	// being unloaded asynchronously (closing BPF objects, sinks, etc.).
	programStateUnloading

	// The program loading was aborted.
	programStateLoadingAborted
)
