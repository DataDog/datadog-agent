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
// from spec. Inspecting the parameters detects signature changes that were
// backported within a kernel version series, where the kernel version can't be
// relied upon.
//
// Which spec to pass depends on how the result is used. Programs that the
// kernel attaches and verifies against its own BTF (fentry/fexit/fmod_ret,
// tp_btf, LSM, iterators, struct_ops) must use the running kernel's BTF from
// GetKernelSpec(). Programs that don't need kernel BTF, such as kprobes, can
// use the CO-RE loader's KernelTypes, which on kernels without native BTF may
// come from embedded or remote-config BTF. GetKernelSpec() holds vmlinux types
// only, so functions in kernel modules (e.g. udpv6_recvmsg with
// CONFIG_IPV6=m) return btf.ErrNotFound.
//
// If spec is shared, as GetKernelSpec()'s is, the returned prototype is shared
// too and must not be modified.
//
// Returns an error wrapping cilium/ebpf's btf.ErrNotFound if the function isn't
// in spec, or btf.ErrMultipleMatches if several functions share the name.
func GetKernelFuncProto(spec *btf.Spec, funcName string) (*btf.FuncProto, error) {
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
// function according to spec. See GetKernelFuncProto for which spec to pass.
//
// The count is of BTF parameters, not fentry/fexit ctx slots: a struct passed
// by value that is larger than 8 bytes takes two slots, so for such functions
// the count is not the index of the fexit return value.
//
// Returns an error wrapping cilium/ebpf's btf.ErrNotFound if the function isn't
// in spec, or btf.ErrMultipleMatches if several functions share the name.
func GetKernelFuncArgCount(spec *btf.Spec, funcName string) (int, error) {
	proto, err := GetKernelFuncProto(spec, funcName)
	if err != nil {
		return 0, err
	}

	return len(proto.Params), nil
}
