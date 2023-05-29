// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// asset represents an asset that needs its content integrity checked at runtime
type asset struct {
	filename string
	hash     string
	tm       CompilationTelemetry
}

func newAsset(filename, hash string) *asset {
	return &asset{
		filename: filename,
		hash:     hash,
		tm:       newCompilationTelemetry(),
	}
}

// Compile compiles the asset to an object file, writes it to the configured output directory, and
// then opens and returns the compiled output
func (a *asset) Compile(config *ebpf.Config, additionalFlags []string, client statsd.ClientInterface) (CompiledOutput, error) {
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

	p := filepath.Join(config.BPFDir, "runtime", a.filename)
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", p, err)
	}
	defer f.Close()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("unable to create compiler output directory %s: %w", outputDir, err)
	}

	protectedFile, err := createProtectedFile(fmt.Sprintf("%s-%s", a.filename, a.hash), outputDir, f)
	if err != nil {
		return nil, fmt.Errorf("failed to create ram backed file from %s: %w", f.Name(), err)
	}
	defer func() {
		if err := protectedFile.Close(); err != nil {
			log.Debugf("error closing protected file %s: %s", protectedFile.Name(), err)
		}
	}()

	if err = a.verify(protectedFile); err != nil {
		a.tm.compilationResult = verificationError
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	out, result, err := compileToObjectFile(protectedFile.Name(), outputDir, a.filename, a.hash, additionalFlags, kernelHeaders)
	a.tm.compilationResult = result

	return out, err
}

// creates a ram backed file from the given reader. The file is made immutable
func createProtectedFile(name, runtimeDir string, source io.Reader) (ProtectedFile, error) {
	protectedFile, err := NewProtectedFile(name, runtimeDir, source)
	if err != nil {
		return nil, fmt.Errorf("failed to create protected file: %w", err)
	}

	return protectedFile, err
}

// verify reads the asset from the reader and verifies the content hash matches what is expected.
func (a *asset) verify(source ProtectedFile) error {
	h := sha256.New()
	if _, err := io.Copy(h, source.Reader()); err != nil {
		return fmt.Errorf("error hashing file %s: %w", source.Name(), err)
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != a.hash {
		return fmt.Errorf("file content hash does not match expected value")
	}

	return nil
}

// GetTelemetry returns the compilation telemetry for this asset
func (a *asset) GetTelemetry() CompilationTelemetry {
	return a.tm
}
