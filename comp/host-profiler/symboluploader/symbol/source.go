// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symbol

import "fmt"

type Source int64

const (
	SourceNone Source = iota
	SourceDynamicSymbolTable
	SourceSymbolTable
	SourceGoPCLnTab
	SourceDebugInfo
)

func NewSource(s string) (Source, error) {
	switch s {
	case "none":
		return SourceNone, nil
	case "dynamic_symbol_table":
		return SourceDynamicSymbolTable, nil
	case "symbol_table":
		return SourceSymbolTable, nil
	case "debug_info":
		return SourceDebugInfo, nil
	case "gopclntab":
		return SourceGoPCLnTab, nil
	}
	return SourceNone, fmt.Errorf("unknown symbol source: %s", s)
}

func (s Source) String() string {
	switch s {
	case SourceNone:
		return "none"
	case SourceDynamicSymbolTable:
		return "dynamic_symbol_table"
	case SourceSymbolTable:
		return "symbol_table"
	case SourceGoPCLnTab:
		return "gopclntab"
	case SourceDebugInfo:
		return "debug_info"
	}

	return "unknown"
}
