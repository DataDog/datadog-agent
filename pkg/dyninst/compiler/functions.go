// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// FunctionID is the identifier of a logical function.
// Implementations must be usable as a hash map key.
type FunctionID interface {
	logicalFuncID() // marker

	String() string
}

// Implements interface marker.
type baseFunctionID struct{}

func (baseFunctionID) logicalFuncID() {}

// Global, unique functions.

// ChasePointers is a function for pointer chasing.
type ChasePointers struct {
	baseFunctionID
}

// String returns a human-readable identifier for the function.
func (ChasePointers) String() string {
	return "ChasePointers"
}

// ProcessEvent is a function for processing user function event (call, line, etc..),
// at given injection PC.
type ProcessEvent struct {
	baseFunctionID
	InjectionPC         uint64
	ThrottlerIdx        int
	PointerChasingLimit uint32
	Frameless           bool
	EventRootType       *ir.EventRootType
}

// String returns a human-readable identifier for the function.
func (e ProcessEvent) String() string {
	return fmt.Sprintf("ProcessEvent[%s@%x]", e.EventRootType.GetName(), e.InjectionPC)
}

// ProcessExpression is a function that runs expression evaluation from a context
// of event function frame, at given injection PC.
type ProcessExpression struct {
	baseFunctionID
	EventRootType *ir.EventRootType
	// The index of the expression in the event root type.
	ExprIdx     uint32
	InjectionPC uint64
}

// String returns a human-readable identifier for the function.
func (e ProcessExpression) String() string {
	return fmt.Sprintf("ProcessExpression[%s@0x%x.expr[%d]]", e.EventRootType.GetName(), e.InjectionPC, e.ExprIdx)
}

// ProcessType is a function that processes user data of a specific type, chasing
// pointers, resolving interfaces, etc (after the data was already read into ringbuf).
// The generated function expects output offset to be set to the beginning of the data.
type ProcessType struct {
	baseFunctionID
	Type ir.Type
}

// String returns a human-readable identifier for the function.
func (e ProcessType) String() string {
	return fmt.Sprintf("ProcessType[%s]", e.Type.GetName())
}
