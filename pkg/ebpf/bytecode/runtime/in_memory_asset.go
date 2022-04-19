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

	"github.com/DataDog/datadog-agent/pkg/runtimecompiler/config"
	"github.com/DataDog/datadog-agent/pkg/security/log"
)

var ConstantFetcher = newInMemoryAsset("constant_fetcher.c")

// inMemoryAsset represents an asset whose content will be generated and provided at runtime
type inMemoryAsset struct {
	filename string
}

func newInMemoryAsset(filename string) *inMemoryAsset {
	return &inMemoryAsset{
		filename: filename,
	}
}

// Compile compiles the provided c code to an object file and writes it to the configured output directory
func (a *inMemoryAsset) Compile(config *config.Config, inputCode string, additionalFlags, kernelHeaders []string) (tm RuntimeCompilationTelemetry, err error) {
	log.Debugf("starting runtime compilation of %s", a.filename)

	tm = newRuntimeCompilationTelemetry()

	start := time.Now()
	tm.compilationEnabled = true
	defer func() {
		tm.compilationDuration = time.Since(start)
	}()

	inputReader := strings.NewReader(inputCode)

	inputHash, err := sha256hex([]byte(inputCode))
	if err != nil {
		tm.compilationResult = inputHashError
		err = fmt.Errorf("error hashing input: %w", err)
		return
	}

	err = compileToObjectFile(inputReader, config.RuntimeCompilerOutputDir, a.filename, inputHash, additionalFlags, kernelHeaders, &tm)
	return
}

// GetCompiledOutputs finds, opens and returns the compiled version of the provided c code
func (a *inMemoryAsset) GetCompiledOutput(cFlags []string, outputDir, inputCode string) (CompiledOutput, error) {
	_, flagHash := computeFlagsAndHash(cFlags)

	inputHash, err := sha256hex([]byte(inputCode))
	if err != nil {
		return nil, fmt.Errorf("error hashing input: %w", err)
	}

	outputFile, err := getOutputFilePath(outputDir, a.filename, inputHash, flagHash)
	if err != nil {
		return nil, fmt.Errorf("error getting output file path: %w", err)
	}

	return openCompiledOutput(a.filename, outputFile)
}
