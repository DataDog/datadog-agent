// +build linux_bpf

package tracer

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type CompiledOutput interface {
	io.Reader
	io.ReaderAt
	io.Closer
}

func getRuntimeCompiledTracer(config *config.Config) (CompiledOutput, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to get kernel version: %w", err)
	}
	pre410Kernel := kv < kernel.VersionCode(4, 1, 0)

	// TODO support input file from bundled files?
	// TODO fix default config.BPFDir to be an absolute path, so this isn't required
	runtimeDir, _ := filepath.Abs(filepath.Join("../../ebpf/bytecode", config.BPFDir, "runtime"))
	inputFile := filepath.Join(runtimeDir, "tracer.c")
	hash, err := hashInput(inputFile)
	if err != nil {
		return nil, fmt.Errorf("error hashing input file: %w", err)
	}
	// TODO do we need to pre-process input to include all `#include`s in hash?
	// filename includes kernel version and input file hash
	// this ensures we re-compile when either of the input changes
	if err := os.MkdirAll(config.RuntimeCompilerOutputDir, 0755); err != nil {
		return nil, fmt.Errorf("unable to create compiler output directory %s: %w", config.RuntimeCompilerOutputDir, err)
	}

	outputFile := filepath.Join(config.RuntimeCompilerOutputDir, fmt.Sprintf("tracer-%d-%s.o", kv, hash))
	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}
		comp, err := compiler.NewEBPFCompiler(config.KernelHeadersDirs, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("failed to create compiler: %w", err)
		}

		cflags := []string{"-I" + runtimeDir}
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

func hashInput(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("unable to read input file: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("error hashing input file: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
