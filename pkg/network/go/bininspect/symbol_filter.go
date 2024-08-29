// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"debug/elf"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

// SymbolFilter is an interface for filtering symbols read from ELF files.
type SymbolFilter interface {
	// GetNumWanted returns the number of symbols wanted by the filter
	GetNumWanted() int
	// GetMinMaxLength returns the minimum and maximum name lengths of the symbols wanted by the filter.
	GetMinMaxLength() (int, int)
	// Want returns true if the filter want the symbol.
	Want(symbol string) bool
	// FindMissing returns the list of symbol names which the filter wanted but were not found in the
	// symbol map. This is only used for error messages.
	FindMissing(map[string]elf.Symbol) []string
}

// StringSetSymbolFilter is a symbol filter which finds all the symbols in a
// string set.
type StringSetSymbolFilter struct {
	SymbolFilter
	symbolSet common.StringSet
	min       int
	max       int
}

// NewStringSetSymbolFilter creates a new StringSetSymbolFilter
func NewStringSetSymbolFilter(symbolSet common.StringSet) StringSetSymbolFilter {
	min, max := getSymbolLengthBoundaries(symbolSet)
	return StringSetSymbolFilter{
		symbolSet: symbolSet,
		min:       min,
		max:       max,
	}
}

// GetMinMaxLength implements GetMinMaxLength
func (f StringSetSymbolFilter) GetMinMaxLength() (int, int) {
	return f.min, f.max
}

// GetNumWanted implements GetNumWanted
func (f StringSetSymbolFilter) GetNumWanted() int {
	return len(f.symbolSet)
}

// Want implements Want
func (f StringSetSymbolFilter) Want(symbol string) bool {
	_, ok := f.symbolSet[symbol]
	return ok
}

// FindMissing implements FindMissing
func (f StringSetSymbolFilter) FindMissing(symbolByName map[string]elf.Symbol) []string {
	missingSymbols := make([]string, 0, len(f.symbolSet)-len(symbolByName))
	for symbolName := range f.symbolSet {
		if _, ok := symbolByName[symbolName]; !ok {
			missingSymbols = append(missingSymbols, symbolName)
		}

	}

	return missingSymbols
}

// PrefixSymbolFilter is a symbol filter which gets any one symbol which has the
// specified prefix.
type PrefixSymbolFilter struct {
	SymbolFilter
	prefix    string
	maxLength int
}

// NewPrefixSymbolFilter creates a new prefix symbol filter.
func NewPrefixSymbolFilter(prefix string, maxLength int) PrefixSymbolFilter {
	return PrefixSymbolFilter{
		prefix:    prefix,
		maxLength: maxLength,
	}
}

// GetMinMaxLength implements GetMinMaxLength
func (f PrefixSymbolFilter) GetMinMaxLength() (int, int) {
	return len(f.prefix), f.maxLength
}

// GetNumWanted implements GetNumWanted
func (f PrefixSymbolFilter) GetNumWanted() int {
	return 1
}

// Want implements Want
func (f PrefixSymbolFilter) Want(symbol string) bool {
	return strings.HasPrefix(symbol, f.prefix)
}

// FindMissing implements FindMissing
func (f PrefixSymbolFilter) FindMissing(_ map[string]elf.Symbol) []string {
	return []string{f.prefix + "..."}
}
