// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

// Package btf provides utilities for dealing with BPF Type Format (BTF) data
package btf

import (
	"sync/atomic"

	"github.com/cilium/ebpf/btf"
)

var kernelCache atomic.Pointer[btf.Cache]

func init() {
	Flush()
}

// Flush deletes any cache of loaded BTF data
func Flush() {
	kernelCache.Store(btf.NewCache())
}

// Cache returns the BTF cache for use with the cilium/ebpf library
func Cache() *btf.Cache {
	return kernelCache.Load()
}

// GetKernelSpec returns a possibly cached version of the running kernel BTF spec.
// It is very important that the caller of this function does not modify the returned value
func GetKernelSpec() (*btf.Spec, error) {
	return kernelCache.Load().Kernel()
}
