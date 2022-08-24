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
	"strings"

	"golang.org/x/sys/unix"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/nikos/types"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// CompilationResult enumerates runtime compilation success & failure modes
type CompilationResult int

const (
	notAttempted CompilationResult = iota
	compilationSuccess
	kernelVersionErr
	verificationError
	outputDirErr
	outputFileErr
	newCompilerErr
	compilationErr
	resultReadErr
	headerFetchErr
	compiledOutputFound
)

type CompiledOutput interface {
	io.Reader
	io.ReaderAt
	io.Closer
}

// RuntimeAsset represents an asset that needs its content integrity checked at runtime
type RuntimeAsset struct {
	filename string
	hash     string

	runtimeCompiler *RuntimeCompiler
}

func NewRuntimeAsset(filename, hash string) *RuntimeAsset {
	return &RuntimeAsset{
		filename: filename,
		hash:     hash,

		runtimeCompiler: NewRuntimeCompiler(),
	}
}

// Verify reads the asset in the provided directory and verifies the content hash matches what is expected.
// On success, it returns an io.Reader for the contents and the content hash of the asset.
func (a *RuntimeAsset) Verify(dir string) (io.Reader, string, error) {
	p := filepath.Join(dir, "runtime", a.filename)
	f, err := os.Open(p)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	h := sha256.New()

	w := io.MultiWriter(&buf, h)
	if _, err := io.Copy(w, f); err != nil {
		return nil, "", fmt.Errorf("error hashing file %s: %w", f.Name(), err)
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != a.hash {
		return nil, "", fmt.Errorf("file content hash does not match expected value")
	}
	return &buf, a.hash, nil
}

// Compile compiles the runtime asset if necessary and returns the resulting file.
func (a *RuntimeAsset) Compile(config *ebpf.Config, cflags []string, client statsd.ClientInterface) (CompiledOutput, error) {
	output, err := a.runtimeCompiler.CompileObjectFile(config, cflags, a.filename, a)
	a.SubmitTelemetry(client)
	return output, err
}

func (a *RuntimeAsset) GetInputReader(config *ebpf.Config, tm *RuntimeCompilationTelemetry) (io.Reader, error) {
	inputReader, _, err := a.Verify(config.BPFDir)
	if err != nil {
		tm.compilationResult = verificationError
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	return inputReader, nil
}

func (a *RuntimeAsset) GetOutputFilePath(config *ebpf.Config, uname *unix.Utsname, flagHash string, tm *RuntimeCompilationTelemetry) (string, error) {
	// filename includes kernel version, input file hash, and cflags hash
	// this ensures we re-compile when either of the input changes
	baseName := strings.TrimSuffix(a.filename, filepath.Ext(a.filename))

	unameHash, err := UnameHash(uname)
	if err != nil {
		return "", err
	}

	outputFile := filepath.Join(config.RuntimeCompilerOutputDir, fmt.Sprintf("%s-%s-%s-%s.o", baseName, unameHash, a.hash, flagHash))
	return outputFile, nil
}

func (a *RuntimeAsset) GetTelemetry() map[string]int64 {
	telemetry := a.runtimeCompiler.GetRCTelemetry()
	return telemetry.GetTelemetry()
}

func (a *RuntimeAsset) SubmitTelemetry(statsdClient statsd.ClientInterface) {
	tm := a.runtimeCompiler.GetRCTelemetry()

	if !tm.compilationEnabled {
		return
	}

	var platform string
	if target, err := types.NewTarget(); err == nil {
		// Prefer platform information from nikos over platform info from the host package, since this
		// is what kernel header downloading uses
		platform = strings.ToLower(target.Distro.Display)
	} else {
		log.Warnf("failed to retrieve host platform information from nikos: %s", err)
		platform = host.GetStatusInformation().Platform
	}

	tags := []string{
		fmt.Sprintf("asset:%s", a.filename),
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
		fmt.Sprintf("platform:%s", platform),
	}

	if tm.compilationResult != notAttempted {
		var resultTag string
		if tm.compilationResult == compilationSuccess || tm.compilationResult == compiledOutputFound {
			resultTag = "success"
		} else {
			resultTag = "failure"
		}

		rcTags := append(tags,
			fmt.Sprintf("result:%s", resultTag),
			fmt.Sprintf("reason:%s", model.RuntimeCompilationResult(tm.compilationResult).String()),
		)

		if err := statsdClient.Count("datadog.system_probe.runtime_compilation.attempted", 1.0, rcTags, 1.0); err != nil {
			log.Warnf("error submitting runtime compilation metric to statsd: %s", err)
		}
	}

	if tm.headerFetchResult != kernel.NotAttempted {
		var resultTag string
		if tm.headerFetchResult <= kernel.DownloadSuccess {
			resultTag = "success"
		} else {
			resultTag = "failure"
		}

		khdTags := append(tags,
			fmt.Sprintf("result:%s", resultTag),
			fmt.Sprintf("reason:%s", model.KernelHeaderFetchResult(tm.headerFetchResult).String()),
		)

		if err := statsdClient.Count("datadog.system_probe.kernel_header_fetch.attempted", 1.0, khdTags, 1); err != nil {
			log.Warnf("error submitting kernel header downloading metric to statsd: %s", err)
		}
	}
}
