// +build linux_bpf

package tracer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

//go:generate go run ../../ebpf/include_headers.go ../ebpf/c/runtime/tracer.c ../../ebpf/bytecode/build/runtime/tracer.c ../ebpf/c ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/runtime/integrity.go ../../ebpf/bytecode/build/runtime/tracer.c ../../ebpf/bytecode/runtime/tracer.go runtime

//go:generate go run ../../ebpf/include_headers.go ../ebpf/c/runtime/conntrack.c ../../ebpf/bytecode/build/runtime/conntrack.c ../ebpf/c ../ebpf/c/runtime ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/runtime/integrity.go ../../ebpf/bytecode/build/runtime/conntrack.c ../../ebpf/bytecode/runtime/conntrack.go runtime

func getRuntimeCompiledTracer(config *config.Config) (runtime.CompiledOutput, error) {
	cflags, err := getCFlags(config)
	if err != nil {
		return nil, err
	}
	return runtime.Tracer.Compile(&config.Config, cflags)
}

func getRuntimeCompiledConntracker(config *config.Config) (runtime.CompiledOutput, error) {
	cflags, err := getCFlags(config)
	if err != nil {
		return nil, err
	}
	return runtime.Conntrack.Compile(&config.Config, cflags)
}

func getCFlags(config *config.Config) ([]string, error) {
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
	return cflags, nil
}
