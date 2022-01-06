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
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/statsd"
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

func (tm *RuntimeCompilationTelemetry) SendMetrics(client *statsd.Client) error {
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
	GetOutputFilePath(config *ebpf.Config, kernelVersion kernel.Version, flagHash string, tm *RuntimeCompilationTelemetry) (string, error)
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

	kv, err := kernel.HostVersion()
	if err != nil {
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

	flags, flagHash := ComputeFlagsAndHash(cflags)

	outputFile, err := provider.GetOutputFilePath(config, kv, flagHash, &rc.telemetry)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			rc.telemetry.compilationResult = outputFileErr
			return nil, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}
		dirs, res, err := kernel.GetKernelHeaders(config.KernelHeadersDirs, config.KernelHeadersDownloadDir, config.AptConfigDir, config.YumReposDir, config.ZypperReposDir)
		rc.telemetry.headerFetchResult = res
		if err != nil {
			rc.telemetry.compilationResult = headerFetchErr
			return nil, fmt.Errorf("unable to find kernel headers: %w", err)
		}
		comp, err := compiler.NewEBPFCompiler(dirs, config.BPFDebug)
		if err != nil {
			rc.telemetry.compilationResult = newCompilerErr
			return nil, fmt.Errorf("failed to create compiler: %w", err)
		}
		defer comp.Close()

		if err := comp.CompileToObjectFile(inputReader, outputFile, flags); err != nil {
			rc.telemetry.compilationResult = compilationErr
			return nil, fmt.Errorf("failed to compile runtime version of %s: %s", inputFileName, err)
		}
		rc.telemetry.compilationResult = compilationSuccess
	} else {
		rc.telemetry.compilationResult = compiledOutputFound
	}

	out, err := os.Open(outputFile)
	if err != nil {
		rc.telemetry.compilationResult = resultReadErr
	}
	return out, err
}
