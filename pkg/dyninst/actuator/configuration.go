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
)

type configuration struct {
	sink     MessageSink
	reporter Reporter
	loader   Loader
}

func makeConfiguration(opts ...Option) (configuration, error) {
	cfg := configuration{
		sink:     noopDispatcher{},
		reporter: noopReporter{},
		loader:   nil, // set after options are applied
	}
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	if cfg.loader == nil {
		var err error
		cfg.loader, err = loader.NewLoader()
		if err != nil {
			return configuration{}, err
		}
	}
	return cfg, nil
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
	ReportAttached(ProcessID, *ir.Program)

	// ReportDetached is called when a program is detached from a process.
	ReportDetached(ProcessID, *ir.Program)

	//ReportIRGenFailed is called when generating the IR for the binary fails.
	ReportIRGenFailed(ir.ProgramID, error, []ir.ProbeDefinition)

	// ReportLoadingFailed is called when a program fails to load.
	ReportLoadingFailed(*ir.Program, error)

	// ReportAttachingFailed is called when a program fails to attach to a process.
	ReportAttachingFailed(ProcessID, *ir.Program, error)
}

type noopReporter struct{}

func (noopReporter) ReportAttached(ProcessID, *ir.Program)                       {}
func (noopReporter) ReportDetached(ProcessID, *ir.Program)                       {}
func (noopReporter) ReportIRGenFailed(ir.ProgramID, error, []ir.ProbeDefinition) {}
func (noopReporter) ReportLoadingFailed(*ir.Program, error)                      {}
func (noopReporter) ReportAttachingFailed(ProcessID, *ir.Program, error)         {}

// Loader is an interface that abstracts ebpf program loader.
type Loader interface {
	Load(program compiler.Program) (*loader.Program, error)
	OutputReader() *ringbuf.Reader
	Close() error
}

// WithLoader sets the loader for the Actuator.
func WithLoader(l Loader) Option {
	return optionFunc(func(s *configuration) {
		s.loader = l
	})
}
