// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package btf

import (
	"fmt"

	"github.com/cilium/ebpf/btf"
)

// GetKernelFuncProto returns the BTF prototype of the named kernel function
// from the running kernel's BTF. Inspecting the parameters detects signature
// changes that were backported within a kernel version series, where the
// kernel version can't be relied upon.
//
// The returned prototype is shared with the cached kernel spec, so it must not
// be modified.
//
// Returns an error wrapping cilium/ebpf's btf.ErrNotFound if the function isn't
// in the kernel BTF, or btf.ErrMultipleMatches if several functions share the
// name.
func GetKernelFuncProto(funcName string) (*btf.FuncProto, error) {
	spec, err := GetKernelSpec()
	if err != nil {
		return nil, fmt.Errorf("error loading kernel BTF: %w", err)
	}

	var fn *btf.Func
	if err := spec.TypeByName(funcName, &fn); err != nil {
		return nil, fmt.Errorf("error finding BTF for %s: %w", funcName, err)
	}

	proto, ok := fn.Type.(*btf.FuncProto)
	if !ok {
		return nil, fmt.Errorf("unexpected BTF type %T for %s", fn.Type, funcName)
	}

	return proto, nil
}

// GetKernelFuncArgCount returns the number of arguments of the named kernel
// function according to the running kernel's BTF.
//
// Only vmlinux BTF is searched, not module BTF, so functions in kernel modules
// (e.g. udpv6_recvmsg with CONFIG_IPV6=m) return btf.ErrNotFound. The count is
// of BTF parameters, not fentry/fexit ctx slots: a struct passed by value that
// is larger than 8 bytes takes two slots, so for such functions the count is
// not the index of the fexit return value.
//
// Returns an error wrapping cilium/ebpf's btf.ErrNotFound if the function isn't
// in the kernel BTF, or btf.ErrMultipleMatches if several functions share the
// name.
func GetKernelFuncArgCount(funcName string) (int, error) {
	proto, err := GetKernelFuncProto(funcName)
	if err != nil {
		return 0, err
	}

	return len(proto.Params), nil
}
