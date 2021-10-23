// +build linux_bpf

package runtime

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	defaultFlags = []string{
		"-DCONFIG_64BIT",
		"-D__BPF_TRACING__",
		`-DKBUILD_MODNAME="ddsysprobe"`,
		"-Wno-unused-value",
		"-Wno-pointer-sign",
		"-Wno-compare-distinct-pointer-types",
		"-Wunused",
		"-Wall",
		"-Werror",
	}
)

func ComputeFlagsAndHash(additionalFlags []string) ([]string, string) {
	flags := make([]string, len(defaultFlags)+len(additionalFlags))
	copy(flags, defaultFlags)
	copy(flags[len(defaultFlags):], additionalFlags)

	flagHash := hashFlags(flags)
	return flags, flagHash
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

type RuntimeCompilationFileProvider interface {
	GetInputFilename() string
	GetInputReader(config *ebpf.Config, tm *RuntimeCompilationTelemetry) (io.Reader, error)
	GetOutputFilePath(config *ebpf.Config, kernelVersion kernel.Version, flagHash string, tm *RuntimeCompilationTelemetry) (string, error)
}

func RuntimeCompileObjectFile(config *ebpf.Config, cflags []string, provider RuntimeCompilationFileProvider, tm *RuntimeCompilationTelemetry) (CompiledOutput, error) {
	start := time.Now()
	defer func() {
		tm.compilationDuration = time.Since(start)
		tm.compilationEnabled = true
	}()

	kv, err := kernel.HostVersion()
	if err != nil {
		tm.compilationResult = kernelVersionErr
		return nil, fmt.Errorf("unable to get kernel version: %w", err)
	}

	inputReader, err := provider.GetInputReader(config, tm)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(config.RuntimeCompilerOutputDir, 0755); err != nil {
		tm.compilationResult = outputDirErr
		return nil, fmt.Errorf("unable to create compiler output directory %s: %w", config.RuntimeCompilerOutputDir, err)
	}

	flags, flagHash := ComputeFlagsAndHash(cflags)

	outputFile, err := provider.GetOutputFilePath(config, kv, flagHash, tm)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			tm.compilationResult = outputFileErr
			return nil, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}
		dirs, res, err := kernel.GetKernelHeaders(config.KernelHeadersDirs, config.KernelHeadersDownloadDir, config.AptConfigDir, config.YumReposDir, config.ZypperReposDir)
		tm.headerFetchResult = res
		if err != nil {
			tm.compilationResult = headerFetchErr
			return nil, fmt.Errorf("unable to find kernel headers: %w", err)
		}
		comp, err := compiler.NewEBPFCompiler(dirs, config.BPFDebug)
		if err != nil {
			tm.compilationResult = newCompilerErr
			return nil, fmt.Errorf("failed to create compiler: %w", err)
		}
		defer comp.Close()

		if err := comp.CompileToObjectFile(inputReader, outputFile, flags); err != nil {
			tm.compilationResult = compilationErr
			return nil, fmt.Errorf("failed to compile runtime version of %s: %s", provider.GetInputFilename(), err)
		}
		tm.compilationResult = compilationSuccess
	} else {
		tm.compilationResult = compiledOutputFound
	}

	out, err := os.Open(outputFile)
	if err != nil {
		tm.compilationResult = resultReadErr
	}
	return out, err
}
