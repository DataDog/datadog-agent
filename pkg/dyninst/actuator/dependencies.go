// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// Sink is an interface that abstracts the sink for the Actuator.
type Sink interface {
	// HandleEvent is called when an event is received from the kernel.
	//
	// Note that the event must not be referenced after this call returns;
	// the underlying memory is reused. If any of the data is needed after
	// this call, it must be copied.
	HandleEvent(output.Event) error

	// Close will be called when the sink is no longer needed.
	Close()
}

// Reporter is an interface for reporting events related to attachment and
// detachment of programs to processes.
type Reporter interface {
	// ReportAttaching is called when a program is about to be attached to a
	// process. This is strictly before the program has been attached.
	ReportAttaching(ProcessID, Executable, *ir.Program)

	// ReportAttachingFailed is called when a program fails to attach to a
	// process.
	ReportAttachingFailed(ProcessID, *ir.Program, error)

	// ReportAttached is called when a program is attached to a process. This
	// is after the program has been attached.
	ReportAttached(ProcessID, *ir.Program)

	// ReportDetached is called when a program is detached from a process.
	ReportDetached(ProcessID, *ir.Program)

	// ReportLoaded is called after a program has been loaded. It is used
	// by the Reporter to initialize the Sink for this program.
	ReportLoaded(*ir.Program) (Sink, error)

	// ReportIRGenFailed is called when generating the IR for the binary fails.
	ReportIRGenFailed(ir.ProgramID, error, []ir.ProbeDefinition)

	// ReportLoadingFailed is called when a program fails to load.
	ReportLoadingFailed(*ir.Program, error)
}

// Loader is an interface that abstracts ebpf program loader.
type Loader interface {
	Load(program compiler.Program) (*loader.Program, error)
	OutputReader() *ringbuf.Reader
	Close() error
}
