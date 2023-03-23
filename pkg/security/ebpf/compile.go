// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && !ebpf_bindata
// +build linux_bpf,!ebpf_bindata

package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// TODO change probe.c path to runtime-compilation specific version
//go:generate go run ../../ebpf/include_headers.go ./c/prebuilt/probe.c ../../ebpf/bytecode/build/runtime/runtime-security.c ./c ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/runtime/integrity.go ../../ebpf/bytecode/build/runtime/runtime-security.c ../../ebpf/bytecode/runtime/runtime-security.go runtime

func getRuntimeCompiledPrograms(config *config.Config, useSyscallWrapper, useRingBuffer bool, client statsd.ClientInterface) (bytecode.AssetReader, error) {
	var cflags []string

	if useSyscallWrapper {
		cflags = append(cflags, "-DUSE_SYSCALL_WRAPPER=1")
	} else {
		cflags = append(cflags, "-DUSE_SYSCALL_WRAPPER=0")
	}

	if !config.NetworkEnabled {
		cflags = append(cflags, "-DDO_NOT_USE_TC")
	}

	if useRingBuffer {
		cflags = append(cflags, "-DUSE_RING_BUFFER=1")
	}

	return runtime.RuntimeSecurity.Compile(&config.Config, cflags, client)
}
