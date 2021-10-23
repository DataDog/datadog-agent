// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && !ebpf_bindata
// +build linux_bpf,!ebpf_bindata

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

// TODO change probe.c path to runtime-compilation specific version
//go:generate go run ../../ebpf/include_headers.go ../ebpf/c/prebuilt/probe.c ../../ebpf/bytecode/build/runtime/runtime-security.c ../ebpf/c ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/runtime/integrity.go ../../ebpf/bytecode/build/runtime/runtime-security.c ../../ebpf/bytecode/runtime/runtime-security.go runtime

func getRuntimeCompiledProbe(config *config.Config, useSyscallWrapper bool) (bytecode.AssetReader, error) {
	var cflags []string

	if useSyscallWrapper {
		cflags = append(cflags, "-DUSE_SYSCALL_WRAPPER=1")
	} else {
		cflags = append(cflags, "-DUSE_SYSCALL_WRAPPER=0")
	}

	return runtime.RuntimeSecurity.Compile(&config.Config, cflags)
}

func getRuntimeCompiledConstants(config *config.Config) (map[string]uint64, error) {
	constantFetcher := NewRuntimeCompilationConstantFetcher(&config.Config)
	constantFetcher.AppendSizeofRequest("sizeof_inode", "struct inode", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest("sb_magic_offset", "struct super_block", "s_magic", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest("tty_offset", "struct signal_struct", "tty", "linux/sched/signal.h")
	constantFetcher.AppendOffsetofRequest("tty_name_offset", "struct tty_struct", "name", "linux/tty.h")
	return constantFetcher.FinishAndGetResults()
}
