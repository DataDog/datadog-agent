// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpfruntime "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
)

//go:generate $GOPATH/bin/include_headers pkg/dyninst/ebpf/event.c pkg/ebpf/bytecode/build/runtime/dyninst_event.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/dyninst_event.c pkg/ebpf/bytecode/runtime/dyninst_event.go runtime

func getCFlags(config *ddebpf.Config) []string {
	cflags := []string{
		"-g",
		"-Wno-unused-variable",
		"-Wno-unused-function",
	}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

// CompileBPFProgram compiles the eBPF program.
func CompileBPFProgram() error {
	// TODO: actually include generated code
	cfg := ddebpf.NewConfig()
	opts := ebpfruntime.CompileOptions{
		AdditionalFlags:  getCFlags(cfg),
		ModifyCallback:   nil,
		UseKernelHeaders: true,
	}
	_, err := ebpfruntime.Dyninstevent.CompileWithOptions(cfg, opts)
	return err
}
