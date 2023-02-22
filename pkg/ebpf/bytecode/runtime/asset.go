// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package runtime

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/justincormack/go-memfd"

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

	inputFile, closeFn, err := createRamBackedFile(a.filename, a.hash, f, config.RuntimeCompilerOutputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create ram backed file from %s: %w", f.Name(), err)
	}
	defer closeFn()

	if err = a.verify(inputFile); err != nil {
		a.tm.compilationResult = verificationError
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	out, result, err := compileToObjectFile(inputFile, outputDir, a.filename, a.hash, additionalFlags, kernelHeaders)
	a.tm.compilationResult = result

	return out, err
}

func setupSourceInfoFile(source io.Reader, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, source); err != nil {
		return err
	}

	return nil
}

// creates a ram backed file from the given reader. The file is made immutable
func createRamBackedFile(name, hash string, source io.Reader, runtimeDir string) (string, func(), error) {
	var err error

	memfdFile, err := memfd.Create()
	if err != nil {
		return "", nil, err
	}
	defer func() {
		if err != nil {
			memfdFile.Close()
		}
	}()

	if _, err = io.Copy(memfdFile, source); err != nil {
		return "", nil, fmt.Errorf("error copying bytes to memfd file: %w", err)
	}

	// seal the memfd file, making it immutable.
	if err = memfdFile.SetSeals(memfd.SealAll); err != nil {
		return "", nil, fmt.Errorf("failed to seal memfd file: %w", err)
	}

	target := fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), memfdFile.Fd())
	tmpFile := filepath.Join(runtimeDir, fmt.Sprintf("%s-%s", name, hash))

	os.Remove(tmpFile)
	if err := os.Symlink(target, tmpFile); err != nil {
		return "", nil, fmt.Errorf("failed to create symlink with target file %s: %w", target, err)
	}

	return tmpFile, func() {
		os.Remove(tmpFile)
		if _, err := memfdFile.Seek(0, os.SEEK_SET); err != nil {
			log.Debug(err)
		}
		if err := setupSourceInfoFile(memfdFile, tmpFile); err != nil {
			log.Debug("failed to setup source file: ", err)
		}
		memfdFile.Close()
	}, nil
}

// verify reads the asset in the provided directory and verifies the content hash matches what is expected.
// On success, it returns an io.Reader for the contents
func (a *asset) verify(verifyFile string) error {
	f, err := os.Open(verifyFile)
	if err != nil {
		return fmt.Errorf("error opening file %s: %w", verifyFile, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("error hashing file: %w", err)
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
