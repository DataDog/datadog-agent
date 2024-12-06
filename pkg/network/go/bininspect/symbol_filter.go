// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// symbolFilter is an interface for filtering symbols read from ELF files.
type symbolFilter interface {
	// getNumWanted returns the number of symbols wanted by the filter
	getNumWanted() int
	// getMinMaxLength returns the minimum and maximum name lengths of the symbols wanted by the filter.
	getMinMaxLength() (int, int)
	// want returns true if the filter want the symbol.
	want(symbol string) bool
	// findMissing returns the list of symbol names which the filter wanted but were not found in the
	// symbol map. This is only used for error messages.
	findMissing(map[string]safeelf.Symbol) []string
}

// stringSetSymbolFilter is a symbol filter which finds all the symbols in a
// string set.
type stringSetSymbolFilter struct {
	symbolSet common.StringSet
	min       int
	max       int
}

func newStringSetSymbolFilter(symbolSet common.StringSet) stringSetSymbolFilter {
	min, max := getSymbolLengthBoundaries(symbolSet)
	return stringSetSymbolFilter{
		symbolSet: symbolSet,
		min:       min,
		max:       max,
	}
}

func (f stringSetSymbolFilter) getMinMaxLength() (int, int) {
	return f.min, f.max
}

func (f stringSetSymbolFilter) getNumWanted() int {
	return len(f.symbolSet)
}

func (f stringSetSymbolFilter) want(symbol string) bool {
	_, ok := f.symbolSet[symbol]
	return ok
}

// findMissing gets the list of symbols which were missing. Only used for error prints.
func (f stringSetSymbolFilter) findMissing(symbolByName map[string]safeelf.Symbol) []string {
	missingSymbols := make([]string, 0, max(0, len(f.symbolSet)-len(symbolByName)))
	for symbolName := range f.symbolSet {
		if _, ok := symbolByName[symbolName]; !ok {
			missingSymbols = append(missingSymbols, symbolName)
		}
	}

	return missingSymbols
}

// infixSymbolFilter is a symbol filter which gets any symbol which has the
// specified infix.
type infixSymbolFilter struct {
	infix     string
	minLength int
	maxLength int
}

func newInfixSymbolFilter(infix string, minLength int, maxLength int) infixSymbolFilter {
	return infixSymbolFilter{
		infix:     infix,
		minLength: minLength,
		maxLength: maxLength,
	}
}

func (f infixSymbolFilter) getMinMaxLength() (int, int) {
	return f.minLength, f.maxLength
}

func (f infixSymbolFilter) getNumWanted() int {
	return 1
}

func (f infixSymbolFilter) want(symbol string) bool {
	return strings.Contains(symbol, f.infix)
}

// findMissing gets the list of symbols which were missing. Only used for error
// prints. Since we only know we were looking for an infix, return that.
func (f infixSymbolFilter) findMissing(_ map[string]safeelf.Symbol) []string {
	return []string{f.infix}
}
