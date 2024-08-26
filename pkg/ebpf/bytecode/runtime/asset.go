// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	return a.compile(config, additionalFlags, client, nil)
}

// CompileWithCallback is the same as Compile, but takes a callback function that is allowed to modify the contents before compilation.
func (a *asset) CompileWithCallback(config *ebpf.Config, additionalFlags []string, client statsd.ClientInterface, modifyCB func(in io.Reader, out io.Writer) error) (CompiledOutput, error) {
	return a.compile(config, additionalFlags, client, modifyCB)
}

func (a *asset) compile(config *ebpf.Config, additionalFlags []string, client statsd.ClientInterface, modifyCB func(in io.Reader, out io.Writer) error) (CompiledOutput, error) {
	log.Debugf("starting runtime compilation of %s", a.filename)

	start := time.Now()
	a.tm.compilationEnabled = true
	defer func() {
		a.tm.compilationDuration = time.Since(start)
		if client != nil {
			a.tm.SubmitTelemetry(a.filename, client)
		}
	}()

	opts := kernel.HeaderOptions{
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

	a.tm.compilationResult = verificationError
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

	diskProtectedFile, err := createProtectedFile(fmt.Sprintf("%s-%s", a.filename, a.hash), outputDir, f)
	if err != nil {
		return nil, fmt.Errorf("failed to create ram backed file from %s: %w", f.Name(), err)
	}
	defer func() {
		if err := diskProtectedFile.Close(); err != nil {
			log.Debugf("error closing protected file %s: %s", diskProtectedFile.Name(), err)
		}
	}()
	protectedFile := diskProtectedFile
	hash := a.hash

	if err = a.verify(diskProtectedFile); err != nil {
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	a.tm.compilationResult = compilationErr
	if modifyCB != nil {
		outBuf := &bytes.Buffer{}
		// seek to the start and read all of protected file contents
		if _, err := diskProtectedFile.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek disk protected file: %w", err)
		}

		// run modify callback
		if err := modifyCB(diskProtectedFile, outBuf); err != nil {
			return nil, fmt.Errorf("modify callback: %w", err)
		}
		outReader := bytes.NewReader(outBuf.Bytes())

		// update hash
		hash, err = sha256Reader(outReader)
		if err != nil {
			return nil, fmt.Errorf("hash post-modification protected file: %w", err)
		}
		if _, err := outReader.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek post-modification contents: %w", err)
		}

		// create new protected file with the post-modification contents
		postModifyProtectedFile, err := createProtectedFile(fmt.Sprintf("%s-%s", a.filename, hash), outputDir, outReader)
		if err != nil {
			return nil, fmt.Errorf("create post-modification protected file: %w", err)
		}
		defer func() {
			if err := postModifyProtectedFile.Close(); err != nil {
				log.Debugf("close post-modification protected file %s: %s", postModifyProtectedFile.Name(), err)
			}
		}()

		// set compilation to use post-modification contents
		protectedFile = postModifyProtectedFile
	}

	out, result, err := compileToObjectFile(protectedFile.Name(), outputDir, a.filename, hash, additionalFlags, kernelHeaders)
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
	sum, err := sha256Reader(source)
	if err != nil {
		return fmt.Errorf("hash file %s: %w", source.Name(), err)
	}
	if sum != a.hash {
		return fmt.Errorf("file content hash does not match expected value")
	}
	return nil
}

func sha256Reader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GetTelemetry returns the compilation telemetry for this asset
func (a *asset) GetTelemetry() CompilationTelemetry {
	return a.tm
}
