// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var ConstantFetcher = newGeneratedAsset("constant_fetcher.c")

// generatedAsset represents an asset whose contents will be dynamically generated at runtime
type generatedAsset struct {
	filename string
	tm       CompilationTelemetry
}

func newGeneratedAsset(filename string) *generatedAsset {
	return &generatedAsset{
		filename: filename,
		tm:       newCompilationTelemetry(),
	}
}

// Compile compiles the provided c code to an object file, writes it to the configured output directory, and
// then opens and returns the compiled output
func (a *generatedAsset) Compile(config *ebpf.Config, inputCode string, additionalFlags []string, client statsd.ClientInterface) (CompiledOutput, error) {
	log.Debugf("starting runtime compilation of %s", a.filename)

	start := time.Now()
	a.tm.compilationEnabled = true
	defer func() {
		a.tm.compilationDuration = time.Since(start)
		if client != nil {
			a.tm.SubmitTelemetry(a.filename, client)
		}
	}()

	opts := kernel.KernelHeaderOptions{
		DownloadEnabled: config.EnableKernelHeaderDownload,
		Dirs:            config.KernelHeadersDirs,
		DownloadDir:     config.KernelHeadersDownloadDir,
		AptConfigDir:    config.AptConfigDir,
		YumReposDir:     config.YumReposDir,
		ZypperReposDir:  config.ZypperReposDir,
	}
	kernelHeaders := kernel.GetKernelHeaders(opts, client)
	if len(kernelHeaders) == 0 {
		a.tm.compilationResult = headerFetchErr
		return nil, fmt.Errorf("unable to find kernel headers")
	}

	outputDir := config.RuntimeCompilerOutputDir

	inputReader := strings.NewReader(inputCode)
	inputHash, err := sha256hex([]byte(inputCode))
	if err != nil {
		a.tm.compilationResult = inputHashError
		return nil, fmt.Errorf("error hashing input: %w", err)
	}

	out, result, err := compileToObjectFile(inputReader, outputDir, a.filename, inputHash, additionalFlags, kernelHeaders)
	a.tm.compilationResult = result

	return out, err
}

// GetTelemetry returns the compilation telemetry for this asset
func (a *generatedAsset) GetTelemetry() CompilationTelemetry {
	return a.tm
}
