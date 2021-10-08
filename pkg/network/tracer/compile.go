// +build linux_bpf

package tracer

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

//go:generate go run ../../../pkg/ebpf/include_headers.go ../../../pkg/network/ebpf/c/runtime/conntrack.c ../../../pkg/ebpf/bytecode/build/runtime/conntrack.c ../../../pkg/ebpf/c ../../../pkg/network/ebpf/c/runtime ../../../pkg/network/ebpf/c
//go:generate go run ../../../pkg/ebpf/bytecode/runtime/integrity.go ../../../pkg/ebpf/bytecode/build/runtime/conntrack.c ../../../pkg/ebpf/bytecode/runtime/conntrack.go runtime

func getRuntimeCompiledConntracker(config *config.Config) (runtime.CompiledOutput, error) {
	return runtime.Conntrack.Compile(&config.Config, getCFlags(config))
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
