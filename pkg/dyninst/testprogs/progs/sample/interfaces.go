// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
	"unsafe"
)

type behavior interface {
	DoSomething() string
}

type firstBehavior struct {
	s string
}

type secondBehavior struct {
	i int
}

func (b firstBehavior) DoSomething() string {
	return fmt.Sprintln(b)
}

func (b secondBehavior) DoSomething() string {
	return fmt.Sprintf("%10d\n", b.i)
}

type iface struct {
	tab  *itab
	data unsafe.Pointer
}
type itab struct {
	inter uintptr
	_type uintptr
	hash  uint32
	_     [4]byte
	fun   [1]uintptr
}

//nolint:all
//go:noinline
func testInterface(b behavior) string {
	ptr := unsafe.Pointer(&b)
	iface := (*iface)(ptr)
	hash := fmt.Sprintf("iface.tab.hash = %#x", iface.tab.hash)
	inter := fmt.Sprintf("iface.tab.inter = %#x", iface.tab.inter)
	iType := fmt.Sprintf("iface.tab._type = %#x", iface.tab._type)
	iFun := fmt.Sprintf("iface.tab.fun = %#x", iface.tab.fun)
	return fmt.Sprintln(hash, inter, iType, iFun)
}

//nolint:all
//go:noinline
func testError(e error) {}

//nolint:all
func executeInterfaceFuncs() {
	testInterface(firstBehavior{"foo"})
	testInterface(secondBehavior{42})
	testError(errors.New("blah"))
}
