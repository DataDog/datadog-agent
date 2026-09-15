// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

// Package gpu contains implementation for the gpu-monitoring module
package gpu

import (
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
)

//go:generate $GOPATH/bin/include_headers pkg/gpu/ebpf/c/runtime/gpu.c pkg/ebpf/bytecode/build/runtime/gpu.c pkg/ebpf/c pkg/gpu/ebpf/c/runtime pkg/gpu/ebpf/c pkg/network/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/gpu.c pkg/ebpf/bytecode/runtime/gpu.go runtime

func getRuntimeCompiledGPUMonitoring(ebpfConfig *ddebpf.Config) (runtime.CompiledOutput, error) {
	return runtime.Gpu.Compile(ebpfConfig, getCFlags(ebpfConfig))
}

func getCFlags(ebpfConfig *ddebpf.Config) []string {
	cflags := []string{"-g"}

	if ebpfConfig.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}
