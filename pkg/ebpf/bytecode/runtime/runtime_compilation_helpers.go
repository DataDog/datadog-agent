// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
)

var defaultFlags = []string{
	"-D__KERNEL__",
	"-DCONFIG_64BIT",
	"-D__BPF_TRACING__",
	`-DKBUILD_MODNAME="ddsysprobe"`,
	"-Wno-unused-value",
	"-Wno-pointer-sign",
	"-Wno-compare-distinct-pointer-types",
	"-Wunused",
	"-Wall",
	"-Werror",
	"-emit-llvm",
	"-O2",
	"-fno-stack-protector",
	"-fno-color-diagnostics",
	"-fno-unwind-tables",
	"-fno-asynchronous-unwind-tables",
	"-fno-jump-tables",
	"-nostdinc",
}

func hashFlags(flags []string) string {
	h := sha256.New()
	for _, f := range flags {
		h.Write([]byte(f))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

type RuntimeCompilationTelemetry struct {
	compilationEnabled  bool
	compilationResult   CompilationResult
	compilationDuration time.Duration
	headerFetchResult   kernel.HeaderFetchResult
}

func NewRuntimeCompilationTelemetry() RuntimeCompilationTelemetry {
	return RuntimeCompilationTelemetry{
		compilationEnabled: false,
		compilationResult:  notAttempted,
		headerFetchResult:  kernel.NotAttempted,
	}
}

func (tm *RuntimeCompilationTelemetry) GetTelemetry() map[string]int64 {
	stats := make(map[string]int64)
	if tm.compilationEnabled {
		stats["runtime_compilation_enabled"] = 1
		stats["runtime_compilation_result"] = int64(tm.compilationResult)
		stats["kernel_header_fetch_result"] = int64(tm.headerFetchResult)
		stats["runtime_compilation_duration"] = tm.compilationDuration.Nanoseconds()
	} else {
		stats["runtime_compilation_enabled"] = 0
	}
	return stats
}

func (tm *RuntimeCompilationTelemetry) SendMetrics(client statsd.ClientInterface) error {
	tags := []string{fmt.Sprintf("version:%s", version.AgentVersion)}

	var enabled float64 = 0
	if tm.compilationEnabled {
		enabled = 1
	}
	if err := client.Gauge(metrics.MetricRuntimeCompiledConstantsEnabled, enabled, tags, 1); err != nil {
		return err
	}

	// if the runtime compilation is not enabled we return directly
	if !tm.compilationEnabled {
		return nil
	}

	if err := client.Gauge(metrics.MetricRuntimeCompiledConstantsCompilationResult, float64(tm.compilationResult), tags, 1); err != nil {
		return err
	}
	if err := client.Gauge(metrics.MetricRuntimeCompiledConstantsCompilationDuration, float64(tm.compilationDuration), tags, 1); err != nil {
		return err
	}
	return client.Gauge(metrics.MetricRuntimeCompiledConstantsHeaderFetchResult, float64(tm.headerFetchResult), tags, 1)
}

type RuntimeCompilationFileProvider interface {
	GetInputReader(config *ebpf.Config, tm *RuntimeCompilationTelemetry) (io.Reader, error)
	GetOutputFilePath(config *ebpf.Config, uname *unix.Utsname, flagHash string, tm *RuntimeCompilationTelemetry) (string, error)
}

type RuntimeCompiler struct {
	telemetry RuntimeCompilationTelemetry
}

func NewRuntimeCompiler() *RuntimeCompiler {
	return &RuntimeCompiler{
		telemetry: NewRuntimeCompilationTelemetry(),
	}
}

func (rc *RuntimeCompiler) GetRCTelemetry() RuntimeCompilationTelemetry {
	return rc.telemetry
}

func (rc *RuntimeCompiler) CompileObjectFile(config *ebpf.Config, cflags []string, inputFileName string, provider RuntimeCompilationFileProvider) (CompiledOutput, error) {
	start := time.Now()
	defer func() {
		rc.telemetry.compilationDuration = time.Since(start)
		rc.telemetry.compilationEnabled = true
	}()

	// we use the raw uname instead of the kernel version, because some kernel versions
	// can be clamped to 255 thus causing collisions
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		rc.telemetry.compilationResult = kernelVersionErr
		return nil, fmt.Errorf("unable to get kernel version: %w", err)
	}

	inputReader, err := provider.GetInputReader(config, &rc.telemetry)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(config.RuntimeCompilerOutputDir, 0755); err != nil {
		rc.telemetry.compilationResult = outputDirErr
		return nil, fmt.Errorf("unable to create compiler output directory %s: %w", config.RuntimeCompilerOutputDir, err)
	}

	flags := append(defaultFlags, cflags...)
	outputFile, err := provider.GetOutputFilePath(config, &uname, hashFlags(flags), &rc.telemetry)

	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			rc.telemetry.compilationResult = outputFileErr
			return nil, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}
		dirs, res, err := kernel.GetKernelHeaders(config.EnableKernelHeaderDownload, config.KernelHeadersDirs, config.KernelHeadersDownloadDir, config.AptConfigDir, config.YumReposDir, config.ZypperReposDir)
		rc.telemetry.headerFetchResult = res
		if err != nil {
			rc.telemetry.compilationResult = headerFetchErr
			return nil, fmt.Errorf("unable to find kernel headers: %w", err)
		}
		if err := compiler.CompileToObjectFile(inputReader, outputFile, flags, dirs); err != nil {
			rc.telemetry.compilationResult = compilationErr
			return nil, fmt.Errorf("failed to compile runtime version of %s: %s", inputFileName, err)
		}
		rc.telemetry.compilationResult = compilationSuccess
		log.Infof("successfully compiled runtime version of %s", inputFileName)
	} else {
		rc.telemetry.compilationResult = compiledOutputFound
	}

	err = bytecode.VerifyAssetPermissions(outputFile)
	if err != nil {
		rc.telemetry.compilationResult = outputFileErr
		return nil, err
	}

	out, err := os.Open(outputFile)
	if err != nil {
		rc.telemetry.compilationResult = resultReadErr
	}
	return out, err
}

// Sha256hex returns the hex string of the sha256 of the provided buffer
func Sha256hex(buf []byte) (string, error) {
	hasher := sha256.New()
	if _, err := hasher.Write(buf); err != nil {
		return "", err
	}
	cCodeHash := hasher.Sum(nil)
	return hex.EncodeToString(cCodeHash), nil
}

// UnameHash returns a sha256 hash of the uname release and version
func UnameHash(uname *unix.Utsname) (string, error) {
	var rv string
	rv += unix.ByteSliceToString(uname.Release[:])
	rv += unix.ByteSliceToString(uname.Version[:])
	return Sha256hex([]byte(rv))
}
