// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

// Dependencies is a collection of dependencies for the module.
// It is exported for testing purposes. Tests can interact with the
// through the WithDependencies option.
type dependencies struct {
	Actuator            Actuator
	Dispatcher          Dispatcher
	DecoderFactory      DecoderFactory
	IRGenerator         IRGenerator
	ProgramCompiler     ProgramCompiler
	KernelLoader        KernelLoader
	Attacher            Attacher
	LogsFactory         LogsUploaderFactory[LogsUploader]
	DiagnosticsUploader DiagnosticsUploader
	ProcessSubscriber   ProcessSubscriber
	symdbManager        *symdbManager
}

// ProcessSubscriber emits combined process lifecycle updates and configuration
// updates.
type ProcessSubscriber interface {
	Start()
	Subscribe(callback func(process.ProcessesUpdate))
}

// IRGenerator is used to generate IR from binary updates.
type IRGenerator interface {
	GenerateIR(
		_ ir.ProgramID, binaryPath string, _ []ir.ProbeDefinition,
		options ...irgen.Option,
	) (*ir.Program, error)
}

// ProgramCompiler turns IR into stack machine programs ready to be loaded.
type ProgramCompiler interface {
	GenerateProgram(*ir.Program) (compiler.Program, error)
}

// KernelLoader loads compiled programs into the kernel.
type KernelLoader interface {
	Load(compiler.Program) (*loader.Program, error)
}

// Attacher connects a loaded program to a target process.
type Attacher interface {
	Attach(
		*loader.Program, actuator.Executable, actuator.ProcessID,
	) (actuator.AttachedProgram, error)
}

// DecoderFactory is a factory for creating decoders.
type DecoderFactory interface {
	NewDecoder(*ir.Program, process.Executable) (Decoder, error)
}

// Decoder is a decoder for a program.
type Decoder interface {
	// Decode writes the decoded event to the output writer and returns the
	// relevant probe definition. If missingTypes is nil, missing types are
	// silently ignored.
	Decode(
		event decode.Event,
		symbolicator symbol.Symbolicator,
		missingTypes decode.MissingTypeCollector,
		out []byte,
	) ([]byte, ir.ProbeDefinition, error)

	// ReportStackPCs reports the program counters of the stack trace for a
	// given stack hash.
	ReportStackPCs(
		stackHash uint64,
		stackPCs []uint64,
	)
}

// decoderFactory is the default decoder factory.
type decoderFactory struct {
	approximateBootTime time.Time
}

// NewDecoder creates a new decoder using decode.NewDecoder.
func (f decoderFactory) NewDecoder(
	program *ir.Program,
	executable process.Executable,
) (_ Decoder, retErr error) {

	// It's a bit unfortunate that we have to open the file here, but it's
	// necessary to get the type information.
	//
	// TODO(ajwerner): This decoder construction shouldn't be here; we should
	// be constructing the decoder as we compile and load the program. Both to
	// avoid that reparsing of the elf headers but also because it's weird to
	// have an interface called a Reporter that fallibly constructs a decoder.
	mm, err := object.OpenMMappingElfFile(executable.Path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := mm.Close(); closeErr != nil {
			if retErr == nil {
				retErr = fmt.Errorf("failed to close file: %w", closeErr)
			} else {
				retErr = fmt.Errorf("%w: (failed to close file: %w)", retErr, closeErr)
			}
		}
	}()
	table, err := gotype.NewTable(mm)
	if err != nil {
		return nil, err
	}
	decoder, err := decode.NewDecoder(program, (*decode.GoTypeNameResolver)(table), f.approximateBootTime)
	if err != nil {
		return nil, err
	}
	return decoder, nil
}

// Actuator defines the interface for an actuator that can be injected.
// This allows tests to use fake actuators.
type Actuator interface {
	HandleUpdate(update actuator.ProcessesUpdate)
	SetRuntime(runtime actuator.Runtime)
	ReportMissingTypes(processID actuator.ProcessID, typeNames []string)
}

// Dispatcher coordinates with the output dispatcher runtime.
type Dispatcher interface {
	RegisterSink(progID ir.ProgramID, sink dispatcher.Sink)
	UnregisterSink(progID ir.ProgramID)
	Shutdown() error
}

// DiagnosticsUploader is an interface that enables the Controller to send
// diagnostics to the backend.
type DiagnosticsUploader interface {
	// Enqueue adds a message to the uploader's queue.
	Enqueue(diag *uploader.DiagnosticMessage) error
}

// LogsUploaderFactory is an interface that enables the Controller to create a
// new logs uploader.
type LogsUploaderFactory[LU LogsUploader] interface {
	// GetUploader returns a reference-counted uploader for the given tags and
	// entity/container IDs.
	GetUploader(metadata uploader.LogsUploaderMetadata) LU
}

// LogsUploader is an interface that enables the Controller to send logs to the
// backend.
type LogsUploader interface {
	// Enqueue adds a message to the uploader's queue.
	Enqueue(data json.RawMessage)
	// Close closes the uploader.
	Close()
}

type erasedLogsUploaderFactory LogsUploaderFactory[LogsUploader]

// logsUploaderFactoryImpl is an implementation of LogsUploaderFactory that
// wraps a typed LogsUploaderFactory and erases the type parameter.
type logsUploaderFactoryImpl[LU LogsUploader] struct {
	factory LogsUploaderFactory[LU]
}

// GetUploader implements erasedLogsUploaderFactory.
func (f logsUploaderFactoryImpl[LU]) GetUploader(
	metadata uploader.LogsUploaderMetadata,
) LogsUploader {
	return f.factory.GetUploader(metadata)
}
