// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package symbol provides a way to symbolicate stack traces.
package symbol

import (
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
)

// StackFrame represents set of functions corresponding to a single pc on a stack.
// The first line represents innermost function, while each following line represents
// call in an outer function to the inner function that has been inlined.
type StackFrame struct {
	Lines []StackLine
}

// StackLine represents a single line of a stack trace.
type StackLine = gosym.GoLocation

// Symbolicator is a type that can symbolicate stack traces.
type Symbolicator interface {
	Symbolicate(stack []uint64, stackHash uint64) ([]StackFrame, error)
}

// GoSymbolicator is a Symbolicator that uses the Go symbol table to symbolicate stack traces.
// Thread-safe.
type GoSymbolicator struct {
	symtab *gosym.GoSymbolTable
}

// NewGoSymbolicator creates a new GoSymbolicator.
func NewGoSymbolicator(symtab *gosym.GoSymbolTable) *GoSymbolicator {
	return &GoSymbolicator{
		symtab: symtab,
	}
}

// Symbolicate implements the Symbolicator.
func (s *GoSymbolicator) Symbolicate(stack []uint64, _ uint64) ([]StackFrame, error) {
	stackFrames := make([]StackFrame, len(stack))
	for i, pc := range stack {
		if i != 0 {
			// Stack contains return addresses, while we want to show call locations.
			// While we do not now the width of the call instruction, any pc within
			// the instruction will do.
			pc--
		}
		stackFrames[i] = StackFrame{
			Lines: s.symtab.LocatePC(pc),
		}
	}
	return stackFrames, nil
}

// CachingSymbolicator is a Symbolicator that caches stack frames.
// Thread-safe.
type CachingSymbolicator struct {
	symbolicator Symbolicator
	cache        *lru.Cache[uint64, []StackFrame]
}

// NewCachingSymbolicator creates a new CachingSymbolicator.
func NewCachingSymbolicator(symbolicator Symbolicator, cacheSize int) (*CachingSymbolicator, error) {
	cache, err := lru.New[uint64, []StackFrame](cacheSize)
	if err != nil {
		return nil, err
	}
	return &CachingSymbolicator{
		symbolicator,
		cache,
	}, nil
}

// Symbolicate implements the Symbolicator.
func (c *CachingSymbolicator) Symbolicate(stack []uint64, stackHash uint64) ([]StackFrame, error) {
	stackFrames, ok := c.cache.Get(stackHash)
	if ok {
		return stackFrames, nil
	}
	// There may be a race here, with two threads adding the same stack to the cache.
	// It's not a problem, and expected to be rare behavior.
	stackFrames, err := c.symbolicator.Symbolicate(stack, stackHash)
	if err != nil {
		return nil, err
	}
	c.cache.Add(stackHash, stackFrames)
	return stackFrames, nil
}
