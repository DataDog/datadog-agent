//go:build linux && linux_bpf
// +build linux,linux_bpf

package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
)

func main() {
	cfg := ebpf.Config{}

	outputDir, err := os.MkdirTemp("", "rctester")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(outputDir)

	cfg.RuntimeCompilerOutputDir = outputDir

	kv, err := kernel.NewKernelVersion()
	if err != nil {
		panic(err)
	}

	rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&cfg, nil)
	rcConstants, err := probe.GetOffsetConstantsFromFetcher(rcFetcher, kv)
	if err != nil {
		panic(err)
	}

	fmt.Printf("constants: %+v\n", rcConstants)
}
