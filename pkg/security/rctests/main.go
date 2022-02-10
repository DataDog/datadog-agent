package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
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

	rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&cfg, nil)
	rcConstants, err := probe.GetOffsetConstantsFromFetcher(rcFetcher)
	if err != nil {
		panic(err)
	}

	fmt.Printf("constants: %+v\n", rcConstants)
}
