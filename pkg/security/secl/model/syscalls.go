// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/model/syscall_table_generator -table-url https://raw.githubusercontent.com/torvalds/linux/v5.19/arch/x86/entry/syscalls/syscall_64.tbl -output syscalls_linux_amd64.go -output-string syscalls_string_linux_amd64.go -abis common,64
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/model/syscall_table_generator -table-url https://raw.githubusercontent.com/torvalds/linux/v5.19/include/uapi/asm-generic/unistd.h -output syscalls_linux_arm64.go -output-string syscalls_string_linux_arm64.go

package model

import (
	"strings"
)

// MarshalText maps the syscall identifier to UTF-8-encoded text and returns the result
func (s Syscall) MarshalText() ([]byte, error) {
	return []byte(strings.ToLower(strings.TrimPrefix(s.String(), "Sys"))), nil
}
