// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"errors"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

type noopSymbolicator struct{}

var unknownFrameData = []symbol.StackFrame{
	{
		Lines: []gosym.GoLocation{
			{
				Function: "unknown",
				File:     "unknown",
				Line:     0,
			},
		},
	},
}

func (n noopSymbolicator) Symbolicate(_ []uint64, _ uint64) ([]symbol.StackFrame, error) {
	return unknownFrameData, nil
}

var _ symbol.Symbolicator = noopSymbolicator{}

func newSymbolicator(executable actuator.Executable) (_ symbol.Symbolicator, _ io.Closer, retErr error) {
	var closer multiCloser
	defer func() {
		if retErr != nil {
			_ = closer.Close()
		}
	}()
	mef, err := object.OpenMMappingElfFile(executable.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating mmapping elf file: %w", err)
	}
	closer.closers = append(closer.closers, mef)
	// Do not close mef here, it must be kept open for the symbolicator to work.
	// It will be closed when the symbolicator is closed.

	moduledata, err := object.ParseModuleData(mef)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing module data: %w", err)
	}

	goVersion, err := object.ParseGoVersion(mef)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing go version: %w", err)
	}

	goDebugSections, err := moduledata.GoDebugSections(mef)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting go debug sections: %w", err)
	}
	closer.closers = append(closer.closers, goDebugSections)
	symbolTable, err := gosym.ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data,
		goDebugSections.GoFunc.Data,
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
		goVersion,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing go symbol table: %w", err)
	}
	symbolicator := symbol.NewGoSymbolicator(symbolTable)
	if symbolicator == nil {
		return nil, nil, fmt.Errorf("error creating go symbolicator")
	}

	// TODO: make this configurable
	cachingSymbolicator, err := symbol.NewCachingSymbolicator(symbolicator, 1000)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating caching symbolicator: %w", err)
	}
	return cachingSymbolicator, &closer, nil
}

type multiCloser struct {
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var errs []error
	for _, closer := range m.closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
