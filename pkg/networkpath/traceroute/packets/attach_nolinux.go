// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package packets

import (
	"syscall"

	"golang.org/x/net/bpf"
)

// SetBPF is not supported
func SetBPF(_c syscall.RawConn, _filter []bpf.RawInstruction) error {
	return ErrAttachBPFNotSupported
}

// SetBPFAndDrain is not supported
func SetBPFAndDrain(_c syscall.RawConn, _filter []bpf.RawInstruction) error {
	return ErrAttachBPFNotSupported
}
