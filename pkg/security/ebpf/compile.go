// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && !ebpf_bindata && !btfhubsync

// Package ebpf holds ebpf related files
package ebpf

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
)

// TODO change probe.c path to runtime-compilation specific version
//go:generate $GOPATH/bin/include_headers pkg/security/ebpf/c/prebuilt/probe.c pkg/ebpf/bytecode/build/runtime/runtime-security.c pkg/security/ebpf/c/include pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/runtime-security.c pkg/ebpf/bytecode/runtime/runtime-security.go runtime
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/model/bpf_maps_generator -runtime-path ../../ebpf/bytecode/build/runtime/runtime-security.c -output ../../security/secl/model/consts_map_names.go -pkg-name model

func getRuntimeCompiledPrograms(config *config.Config, useSyscallWrapper, useFentry, useRingBuffer bool, client statsd.ClientInterface) (bytecode.AssetReader, error) {
	var cflags []string

	if useFentry {
		cflags = append(cflags, "-DUSE_FENTRY=1")
	}

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

	cflags = append(cflags, "-g")

	return runtime.RuntimeSecurity.Compile(&config.Config, cflags, client)
}
