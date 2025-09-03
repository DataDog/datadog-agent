// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
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

func newSymbolicator(executable actuator.Executable) (_ symbol.Symbolicator, c io.Closer, retErr error) {
	defer func() {
		if retErr != nil && c != nil {
			_ = c.Close()
		}
	}()
	symbolTable, err := object.OpenGoSymbolTable(executable.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening go symbol table: %w", err)
	}
	c = symbolTable
	symbolicator := symbol.NewGoSymbolicator(&symbolTable.GoSymbolTable)
	if symbolicator == nil {
		return nil, nil, fmt.Errorf("error creating go symbolicator")
	}

	// TODO: make this configurable
	cachingSymbolicator, err := symbol.NewCachingSymbolicator(symbolicator, 1000)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating caching symbolicator: %w", err)
	}
	return cachingSymbolicator, symbolTable, nil
}
