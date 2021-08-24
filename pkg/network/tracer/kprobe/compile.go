// +build linux_bpf

package kprobe

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

//go:generate go run ../../../ebpf/include_headers.go ../../ebpf/c/runtime/tracer.c ../../../ebpf/bytecode/build/runtime/tracer.c ../../ebpf/c ../../ebpf/c/runtime ../../../ebpf/c
//go:generate go run ../../../ebpf/bytecode/runtime/integrity.go ../../../ebpf/bytecode/build/runtime/tracer.c ../../../ebpf/bytecode/runtime/tracer.go runtime

func getRuntimeCompiledTracer(config *config.Config) (runtime.CompiledOutput, error) {
	return runtime.Tracer.Compile(&config.Config, getCFlags(config))
}

func getCFlags(config *config.Config) []string {
	var cflags []string
	if config.CollectIPv6Conns {
		cflags = append(cflags, "-DFEATURE_IPV6_ENABLED")
	}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}
