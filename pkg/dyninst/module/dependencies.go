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
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

// ProcessSubscriber is an interface that can be used to subscribe to process
// events.
type ProcessSubscriber interface {
	SubscribeExec(func(pid uint32)) (cleanup func())
	SubscribeExit(func(pid uint32)) (cleanup func())
}

// Scraper is an interface that enables the Controller to get updates from the
// scraper and to set the probe status to emitting.
type Scraper interface {
	// GetUpdates returns the current set of updates.
	GetUpdates() []rcscrape.ProcessUpdate
}

// DecoderFactory is a factory for creating decoders.
type DecoderFactory interface {
	NewDecoder(*ir.Program, procmon.Executable) (Decoder, error)
}

// Decoder is a decoder for a program.
type Decoder interface {
	// Decode writes the decoded event to the output writer and returns the
	// relevant probe definition.
	Decode(
		event decode.Event,
		symbolicator symbol.Symbolicator,
		out []byte,
	) ([]byte, ir.ProbeDefinition, error)
}

// DefaultDecoderFactory is the default decoder factory.
type DefaultDecoderFactory struct {
	approximateBootTime time.Time
}

// NewDecoder creates a new decoder using decode.NewDecoder.
func (f DefaultDecoderFactory) NewDecoder(
	program *ir.Program,
	executable procmon.Executable,
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

// Actuator is an interface that enables the Controller to create a new tenant.
type Actuator[T ActuatorTenant] interface {
	// NewTenant creates a new tenant.
	NewTenant(
		name string,
		reporter actuator.Reporter,
		irGenerator actuator.IRGenerator,
	) T

	Shutdown() error
}

// ActuatorTenant is an interface that enables the Controller to handle updates
// from the actuator.
type ActuatorTenant interface {
	// HandleUpdate handles an update from the actuator.
	HandleUpdate(actuator.ProcessesUpdate)
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
