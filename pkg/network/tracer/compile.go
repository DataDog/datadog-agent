// +build linux_bpf

package tracer

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

//go:generate go run ../../ebpf/include_headers.go ../ebpf/c/runtime/tracer.c ../../ebpf/bytecode/build/runtime/tracer.c ../ebpf/c ../ebpf/c/runtime ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/runtime/integrity.go ../../ebpf/bytecode/build/runtime/tracer.c ../../ebpf/bytecode/runtime/tracer.go runtime

//go:generate go run ../../ebpf/include_headers.go ../ebpf/c/runtime/conntrack.c ../../ebpf/bytecode/build/runtime/conntrack.c ../ebpf/c ../ebpf/c/runtime ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/runtime/integrity.go ../../ebpf/bytecode/build/runtime/conntrack.c ../../ebpf/bytecode/runtime/conntrack.go runtime

func getRuntimeCompiledTracer(config *config.Config) (runtime.CompiledOutput, error) {
	return runtime.Tracer.Compile(&config.Config, getCFlags(config))
}

func getRuntimeCompiledConntracker(config *config.Config) (runtime.CompiledOutput, error) {
	return runtime.Conntrack.Compile(&config.Config, getCFlags(config))
}

func getCFlags(config *config.Config) []string {
	kv, _ := kernel.HostVersion() // ignore any error here since we check again for this error in runtimeAsset.Compile
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
	return cflags
}
