// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package runtime

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/runtimecompiler/config"
	"github.com/DataDog/datadog-agent/pkg/security/log"
)

// RuntimeAsset represents an asset that needs its content integrity checked at runtime
type RuntimeAsset struct {
	filename string
	hash     string
}

func NewRuntimeAsset(filename, hash string) *RuntimeAsset {
	return &RuntimeAsset{
		filename: filename,
		hash:     hash,
	}
}

// Compile compiles the runtime asset to an object file and writes it to the configured output directory
func (a *RuntimeAsset) Compile(config *config.Config, additionalFlags, kernelHeaders []string) (tm RuntimeCompilationTelemetry, err error) {
	log.Debugf("starting runtime compilation of %s", a.filename)

	tm = newRuntimeCompilationTelemetry()

	start := time.Now()
	tm.compilationEnabled = true
	defer func() {
		tm.compilationDuration = time.Since(start)
	}()

	inputReader, err := a.verify(config.BPFDir)
	if err != nil {
		tm.compilationResult = verificationError
		err = fmt.Errorf("error reading input file: %s", err)
		return
	}

	err = compileToObjectFile(inputReader, config.RuntimeCompilerOutputDir, a.filename, a.hash, additionalFlags, kernelHeaders, &tm)
	return
}

// GetCompiledOutputs finds, opens and returns the compiled version of this runtime asset
func (a *RuntimeAsset) GetCompiledOutput(cFlags []string, outputDir string) (CompiledOutput, error) {
	_, flagHash := computeFlagsAndHash(cFlags)

	outputFile, err := getOutputFilePath(outputDir, a.filename, a.hash, flagHash)
	if err != nil {
		return nil, fmt.Errorf("error getting output file path: %w", err)
	}

	return openCompiledOutput(a.filename, outputFile)
}

// verify reads the asset in the provided directory and verifies the content hash matches what is expected.
// On success, it returns an io.Reader for the contents
func (a *RuntimeAsset) verify(dir string) (io.Reader, error) {
	p := filepath.Join(dir, "runtime", a.filename)
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	h := sha256.New()

	w := io.MultiWriter(&buf, h)
	if _, err := io.Copy(w, f); err != nil {
		return nil, fmt.Errorf("error hashing file %s: %w", f.Name(), err)
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != a.hash {
		return nil, fmt.Errorf("file content hash does not match expected value")
	}
	return &buf, nil
}
