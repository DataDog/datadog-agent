// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import "runtime"

// Syscall defines a syscall
type Syscall interface {
	MarshalText() ([]byte, error)
	String() string
	ToInt() int
}

// NewSyscall returns a new syscall
func NewSyscall(num int) Syscall {
	if runtime.GOARCH == "arm64" {
		return Arm64Syscall(num)
	}
	return Amd64Syscall(num)
}

// NewSyscallByArch returns a new syscall for the given arch
func NewSyscallByArch(num int, arch string) Syscall {
	if arch == "arm64" {
		return Arm64Syscall(num)
	}
	return Amd64Syscall(num)
}
