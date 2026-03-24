// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
)

// LoadOptions carries optional parameters for Runtime.Load.
type LoadOptions struct {
	// AdditionalTypes is a sorted, deduplicated list of Go type names
	// discovered at runtime (e.g. from interface decoding) that should be
	// included in the IR program's type registry.
	AdditionalTypes []string
}

// Runtime abstracts the creation, attachment, and cleanup of a program.
type Runtime interface {
	// Load loads a program into the runtime.
	//
	// If loading fails, the process will enter a failed state until new
	// probes are added for it or the process is removed.
	Load(
		ir.ProgramID, Executable, ProcessID, []ir.ProbeDefinition, LoadOptions,
	) (LoadedProgram, error)
}

// LoadedProgram represents a program prepared for attachment.
type LoadedProgram interface {
	// Attach attaches the program to a process.
	Attach(ProcessID, Executable) (AttachedProgram, error)

	// RuntimeStats returns the per-core runtime stats of the program.
	RuntimeStats() []loader.RuntimeStats

	// Close closes the loaded program. It will only be called after any
	// Attach() call have returned and any AttachedProgram.Detach() call have
	// returned.
	Close() error
}

// AttachedProgram represents a program attached to a process.
type AttachedProgram interface {
	// Detach detaches the program from the process.
	Detach(reason error) error
}
