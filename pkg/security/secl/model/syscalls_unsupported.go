// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package model

import "errors"

// UnsupportedSyscall defines a syscall on a non supported platform
type UnsupportedSyscall int

// MarshalText maps the syscall identifier to UTF-8-encoded text and returns the result
func (s UnsupportedSyscall) MarshalText() ([]byte, error) {
	return nil, errors.New("unsupported platform")
}

// ToInt returns the syscall number
func (s UnsupportedSyscall) ToInt() int {
	return 0
}

// String returns a string representation of the syscall
func (s UnsupportedSyscall) String() string {
	return "unsupported platform"
}

// NewSyscall returns a new syscall
func NewSyscall(num int) Syscall {
	return UnsupportedSyscall(0)
}

// NewSyscallByArch returns a new syscall for the given arch
func NewSyscallByArch(num int, arch string) Syscall {
	return UnsupportedSyscall(0)
}
