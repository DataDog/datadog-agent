// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package sharedlibraries

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
)

func getRuntimeCompiledSharedLibraries(config *ebpf.Config) (runtime.CompiledOutput, error) {
	return runtime.SharedLibraries.Compile(config, getCFlags(config))
}

func getCFlags(config *ebpf.Config) []string {
	cflags := []string{"-g"}

	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}
