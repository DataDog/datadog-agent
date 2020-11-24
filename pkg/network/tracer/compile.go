// +build linux_bpf

package tracer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/build/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type CompiledOutput interface {
	io.Reader
	io.ReaderAt
	io.Closer
}

//go:generate go run ../../ebpf/bytecode/include_headers.go ../ebpf/c/runtime/tracer.c ../../ebpf/bytecode/build/runtime/tracer.c ../ebpf/c ../../ebpf/c
//go:generate go run ../../ebpf/bytecode/integrity.go ../../ebpf/bytecode/build/runtime/tracer.c ../../ebpf/bytecode/build/runtime/tracer.go runtime

func getRuntimeCompiledTracer(config *config.Config) (CompiledOutput, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to get kernel version: %w", err)
	}
	pre410Kernel := kv < kernel.VersionCode(4, 1, 0)

	inputFile, hash, err := runtime.Tracer.Verify(config.BPFDir)
	if err != nil {
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	if err := os.MkdirAll(config.RuntimeCompilerOutputDir, 0755); err != nil {
		return nil, fmt.Errorf("unable to create compiler output directory %s: %w", config.RuntimeCompilerOutputDir, err)
	}
	// filename includes kernel version and input file hash
	// this ensures we re-compile when either of the input changes
	outputFile := filepath.Join(config.RuntimeCompilerOutputDir, fmt.Sprintf("tracer-%d-%s.o", kv, hash))
	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}
		comp, err := compiler.NewEBPFCompiler(config.KernelHeadersDirs, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("failed to create compiler: %w", err)
		}

		var cflags []string
		if config.CollectIPv6Conns {
			cflags = append(cflags, "-DFEATURE_IPV6_ENABLED")
		}
		if config.DNSInspection && !pre410Kernel && config.CollectDNSStats {
			cflags = append(cflags, "-DFEATURE_DNS_STATS_ENABLED")
		}

		if err := comp.CompileToObjectFile(inputFile, outputFile, cflags); err != nil {
			return nil, fmt.Errorf("failed to compile runtime version of tracer: %s", err)
		}
	}
	return os.Open(outputFile)
}
