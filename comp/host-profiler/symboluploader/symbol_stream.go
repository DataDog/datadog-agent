// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbolcopier"
	"github.com/DataDog/datadog-agent/pkg/util/elfstream"
)

// streamSymbolFile plans e's symbol ELF with elfstream rather than objcopy, and
// returns a function that writes it compressed. With one endpoint, the ELF is
// compressed while it is sent, without files. With more, it is compressed once
// into a temporary file that each endpoint reads at its own pace, so a slow
// endpoint doesn't hold back the others. release removes that file.
func (d *DatadogSymbolUploader) streamSymbolFile(ctx context.Context, e *symbol.Elf, endpoints int) (writeSymbolFile func(io.Writer) error, release func(), err error) {
	stream, err := d.planSymbolStream(ctx, e)
	if err != nil {
		return nil, nil, err
	}
	write := func(w io.Writer) (err error) {
		// The ELF is written in-process: a writer bug must fail the upload,
		// not crash the profiler.
		defer recoverStreamPanic(&err)
		return compressSymbolFile(w, stream.Size(), func(w io.Writer) error { return stream.Write(ctx, w) })
	}
	if endpoints <= 1 {
		return write, func() {}, nil
	}
	spool, err := os.CreateTemp("", "symbol-zstd")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create compressed symbol file: %w", err)
	}
	release = func() { os.Remove(spool.Name()) }
	if err := errors.Join(write(spool), spool.Close()); err != nil {
		release()
		return nil, nil, err
	}
	return copySymbolFile(spool.Name()), release, nil
}

// planSymbolStream plans e's symbol ELF. Like objcopy, planning gets
// symbolCopyTimeout.
func (d *DatadogSymbolUploader) planSymbolStream(ctx context.Context, e *symbol.Elf) (stream *elfstream.ELFStream, err error) {
	// The ELF is parsed in-process: a parser bug on one input must fail that
	// upload, not crash the profiler.
	defer recoverStreamPanic(&err)
	ctx, cancel := context.WithTimeout(ctx, symbolCopyTimeout)
	defer cancel()
	// Read the already-open file (or in-memory VDSO): reopening the path could
	// pick up a replaced file.
	input, size, err := e.SymbolReader()
	if err != nil {
		return nil, err
	}
	symbolSource, goPCLnTabInfo, _ := d.getSymbolSourceWithGoPCLnTab(e)
	var sectionsToKeep []symbol.SectionInfo
	if symbolSource == symbol.SourceDynamicSymbolTable {
		sectionsToKeep = e.GetSectionsRequiredForDynamicSymbols()
	}
	stream, err = symbolcopier.NewSymbolStream(ctx, input, size, goPCLnTabInfo, sectionsToKeep)
	if err != nil {
		return nil, fmt.Errorf("failed to plan symbol file: %w", err)
	}
	return stream, nil
}

func recoverStreamPanic(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("panic while streaming symbols: %v\n%s", r, debug.Stack())
	}
}
