// +build linux_bpf

package tracer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

//go:generate go run ../../ebpf/bytecode/include_headers.go ../ebpf/c/runtime/tracer.c ../../ebpf/bytecode/build/runtime/tracer.c ../ebpf/c ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/integrity.go ../../ebpf/bytecode/build/runtime/tracer.c ../../ebpf/bytecode/runtime/tracer.go runtime

func getRuntimeCompiledTracer(config *config.Config) (bytecode.CompiledOutput, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to get kernel version: %w", err)
	}
	pre410Kernel := kv < kernel.VersionCode(4, 1, 0)

	var cflags []string
	if config.CollectIPv6Conns {
		cflags = append(cflags, "-DFEATURE_IPV6_ENABLED")
	}
	if config.DNSInspection && !pre410Kernel && config.CollectDNSStats {
		cflags = append(cflags, "-DFEATURE_DNS_STATS_ENABLED")
	}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}

	return runtime.Tracer.Compile(&config.Config, cflags)
}
