// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
)

type configuration struct {
	ringBufSize      int
	sink             MessageSink
	reporter         Reporter
	codegenWriter    CodegenWriterFactory
	compiledCallback CompiledCallback
}

const defaultRingbufSize = 1 << 20 // 1 MiB

var defaultSettings = configuration{
	ringBufSize: defaultRingbufSize,
	sink:        noopDispatcher{},
	reporter:    noopReporter{},
	codegenWriter: func(*ir.Program) io.Writer {
		return nil
	},
	compiledCallback: func(*CompiledProgram) {},
}

// Option is a function that can be used to configure the Actuator.
type Option interface {
	apply(*configuration)
}

type optionFunc func(*configuration)

func (f optionFunc) apply(s *configuration) {
	f(s)
}

// WithMessageSink sets the dispatcher for the Actuator.
func WithMessageSink(dispatcher MessageSink) Option {
	return optionFunc(func(s *configuration) {
		s.sink = dispatcher
	})
}

// MessageSink deals with processing messages produced by programs.
//
// Note that the dispatcher should be thread-safe.
type MessageSink interface {

	// RegisterProgram registers a program with the MessageSink.
	// It will be called before any messages are sent to the program.
	RegisterProgram(program *ir.Program)

	// UnregisterProgram unregisters a program from the MessageSink.
	// It will be called after all messages have been sent to the program.
	UnregisterProgram(program ir.ProgramID)

	// HandleMessage handles a message from a program.
	//
	// The Message buffers are pooled -- the caller should call Release
	// when it is done with the message. Note that at that point, the
	// caller should not access the message or its data anymore.
	HandleMessage(message Message) error
}

type noopDispatcher struct{}

func (noopDispatcher) HandleMessage(Message) error    { return nil }
func (noopDispatcher) RegisterProgram(*ir.Program)    {}
func (noopDispatcher) UnregisterProgram(ir.ProgramID) {}

// WithReporter sets the reporter for the Actuator.
func WithReporter(reporter Reporter) Option {
	return optionFunc(func(s *configuration) {
		s.reporter = reporter
	})
}

// Reporter is an ad-hoc interface for reporting events related to
// attachment and detachment of programs to processes.
//
// TODO: This is not sufficient for what we'll need for driving the
// diagnostics output, but it's a start to drive testing.
type Reporter interface {

	// ReportAttached is called when a program is attached to a process.
	ReportAttached(ProcessID, []irgen.ProbeDefinition)

	// ReportDetached is called when a program is detached from a process.
	ReportDetached(ProcessID, []irgen.ProbeDefinition)

	// ReportCompilationFailed is called when a program fails to compile.
	ReportCompilationFailed(ir.ProgramID, error)

	// ReportLoadingFailed is called when a program fails to load.
	ReportLoadingFailed(ir.ProgramID, error)

	// ReportAttachingFailed is called when a program fails to attach to a process.
	ReportAttachingFailed(ir.ProgramID, ProcessID, error)
}

type noopReporter struct{}

func (noopReporter) ReportCompilationFailed(ir.ProgramID, error)          {}
func (noopReporter) ReportLoadingFailed(ir.ProgramID, error)              {}
func (noopReporter) ReportAttachingFailed(ir.ProgramID, ProcessID, error) {}
func (noopReporter) ReportAttached(ProcessID, []irgen.ProbeDefinition)    {}
func (noopReporter) ReportDetached(ProcessID, []irgen.ProbeDefinition)    {}

// WithRingBufSize sets the size of the ring buffer for the Actuator.
func WithRingBufSize(size int) Option {
	return optionFunc(func(s *configuration) {
		s.ringBufSize = size
	})
}

// CodegenWriterFactory is a function that optionally creates a writer
// to which the generated code will be written.
type CodegenWriterFactory func(*ir.Program) io.Writer

// WithCodegenWriter allows the client to inject a writer to be used
// for writing out the generated code.
func WithCodegenWriter(f CodegenWriterFactory) Option {
	return optionFunc(func(s *configuration) {
		s.codegenWriter = f
	})
}

// CompiledCallback is a function that is called when the eBPF program
// has been compiled.
type CompiledCallback func(*CompiledProgram)

// WithCompiledCallback allows the client to inject a callback to be used
// for writing out the compiled code.
func WithCompiledCallback(f CompiledCallback) Option {
	return optionFunc(func(s *configuration) {
		s.compiledCallback = f
	})
}
