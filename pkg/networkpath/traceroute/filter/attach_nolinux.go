// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package filter

import (
	"syscall"

	"golang.org/x/net/bpf"
)

// SetBPF is a no-op on this platform
func SetBPF(_c syscall.RawConn, _filter []bpf.RawInstruction) error {
	return nil
}

// SetBPF is a no-op on this platform
func SetBPFAndDrain(_c syscall.RawConn, _filter []bpf.RawInstruction) error {
	return nil
}
