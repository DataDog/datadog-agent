// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
)

// GoSymbolTable is a wrapper around the Go symbol table.
//
// Note that it is not safe to use the SymbolTable field after calling Close.
type GoSymbolTable struct {
	gosym.GoSymbolTable
	GoDebugSections
	ModuleData *ModuleData

	// The GoDebugSections and GoDebugSections are both part of the struct
	// so that by holding any references to the GoSymbolTable alive will
	// prevent the GoDebugSections from being garbage collected, and thus
	// will prevent the finalizers from running. This is all just a
	// precaution to help prevent misuse, but it is not enforced by the type
	// system.
	_ struct{}
}

// OpenGoSymbolTable opens the Go symbol table from an object file.
func OpenGoSymbolTable(path string) (_ *GoSymbolTable, retErr error) {
	mef, err := OpenMMappingElfFile(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		closeErr := mef.Close()
		if retErr == nil {
			retErr = closeErr
		} else if closeErr != nil {
			retErr = errors.Join(retErr, closeErr)
		}
	}()
	return ParseGoSymbolTable(mef)
}

// ParseGoSymbolTable parses the Go symbol table from an object file.
func ParseGoSymbolTable(mef File) (_ *GoSymbolTable, retErr error) {
	moduledata, err := ParseModuleData(mef)
	if err != nil {
		return nil, err
	}

	goDebugSections, err := moduledata.GoDebugSections(mef)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr == nil {
			return
		}
		if err := goDebugSections.Close(); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}()

	symbolTable, err := gosym.ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data(),
		goDebugSections.GoFunc.Data(),
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
	)
	if err != nil {
		return nil, err
	}

	return &GoSymbolTable{
		GoSymbolTable:   *symbolTable,
		GoDebugSections: *goDebugSections,
		ModuleData:      moduledata,
	}, nil
}
