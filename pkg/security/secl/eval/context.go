// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package eval

import (
	"unsafe"
)

// RegisterID identify a register ID
type RegisterID = string

// Register describes a register that can be used by a set
type Register struct {
	ID    string
	Value unsafe.Pointer
}

// Registers defines all available registers
type Registers map[RegisterID]*Register

// Clone returns a copy of the registers
func (r Registers) Clone() Registers {
	regs := make(Registers)

	for k, v := range r {
		regs[k] = v
	}

	return regs
}

// Context describes the context used during a rule evaluation
type Context struct {
	Object unsafe.Pointer

	registers Registers
}

// SetObject set the given object to the context
func (c *Context) SetObject(obj unsafe.Pointer) {
	c.Object = obj
}
