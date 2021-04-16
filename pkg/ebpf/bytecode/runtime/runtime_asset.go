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

// RuntimeCompilationEnabled indicates whether or not runtime compilation is enabled
var RuntimeCompilationEnabled = false

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

	// Telemetry
	compilationResult   CompilationResult
	compilationDuration time.Duration
}

func NewRuntimeAsset(filename, hash string) *RuntimeAsset {
	return &RuntimeAsset{
		filename:          filename,
		hash:              hash,
		compilationResult: notAttempted,
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
func (a *RuntimeAsset) Compile(config *ebpf.Config, cflags []string) (CompiledOutput, error) {
	start := time.Now()
	defer func() {
		a.compilationDuration = time.Since(start)
	}()

	kv, err := kernel.HostVersion()
	if err != nil {
		a.compilationResult = kernelVersionErr
		return nil, fmt.Errorf("unable to get kernel version: %w", err)
	}

	inputReader, hash, err := a.Verify(config.BPFDir)
	if err != nil {
		a.compilationResult = verificationError
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	if err := os.MkdirAll(config.RuntimeCompilerOutputDir, 0755); err != nil {
		a.compilationResult = outputDirErr
		return nil, fmt.Errorf("unable to create compiler output directory %s: %w", config.RuntimeCompilerOutputDir, err)
	}

	flags := make([]string, len(defaultFlags)+len(cflags))
	copy(flags, defaultFlags)
	copy(flags[len(defaultFlags):], cflags)
	flagHash := hashFlags(flags)

	// filename includes kernel version, input file hash, and cflags hash
	// this ensures we re-compile when either of the input changes
	baseName := strings.TrimSuffix(a.filename, filepath.Ext(a.filename))
	outputFile := filepath.Join(config.RuntimeCompilerOutputDir, fmt.Sprintf("%s-%d-%s-%s.o", baseName, kv, hash, flagHash))
	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			a.compilationResult = outputFileErr
			return nil, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}
		comp, err := compiler.NewEBPFCompiler(config.KernelHeadersDirs, config.BPFDebug)
		if err != nil {
			a.compilationResult = newCompilerErr
			return nil, fmt.Errorf("failed to create compiler: %w", err)
		}
		defer comp.Close()

		if err := comp.CompileToObjectFile(inputReader, outputFile, flags); err != nil {
			a.compilationResult = compilationErr
			return nil, fmt.Errorf("failed to compile runtime version of %s: %s", a.filename, err)
		}
	}

	out, err := os.Open(outputFile)
	if err == nil {
		a.compilationResult = compilationSuccess
	} else {
		a.compilationResult = resultReadErr
	}
	return out, err
}

func (a *RuntimeAsset) GetTelemetry() map[string]int64 {
	stats := make(map[string]int64)
	if RuntimeCompilationEnabled {
		stats["runtime_compilation_enabled"] = 1
		stats["runtime_compilation_result"] = int64(a.compilationResult)
		stats["runtime_compilation_duration"] = a.compilationDuration.Nanoseconds()
	} else {
		stats["runtime_compilation_enabled"] = 0
	}
	return stats
}

func hashFlags(flags []string) string {
	h := sha256.New()
	for _, f := range flags {
		h.Write([]byte(f))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
