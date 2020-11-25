// +build linux_bpf

package ebpf

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type CompiledOutput interface {
	io.Reader
	io.ReaderAt
	io.Closer
}

// RuntimeAsset represents an asset that needs it content integrity checked at runtime
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

// Verify reads the asset in the provided directory and verifies the content hash matches what is expected.
// On success, it returns the full path and content hash of the asset.
func (a *RuntimeAsset) Verify(dir string) (string, string, error) {
	p := filepath.Join(dir, "runtime", a.filename)
	f, err := os.Open(p)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", "", fmt.Errorf("error hashing file %s: %w", f.Name(), err)
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != a.hash {
		return "", "", fmt.Errorf("file content hash does not match expected value")
	}
	return p, a.hash, nil
}

// Compile compiles the runtime asset if necessary and returns the resulting file.
func (a *RuntimeAsset) Compile(config *Config, cflags []string) (CompiledOutput, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to get kernel version: %w", err)
	}

	inputFile, hash, err := a.Verify(config.BPFDir)
	if err != nil {
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	if err := os.MkdirAll(config.RuntimeCompilerOutputDir, 0755); err != nil {
		return nil, fmt.Errorf("unable to create compiler output directory %s: %w", config.RuntimeCompilerOutputDir, err)
	}
	// filename includes kernel version and input file hash
	// this ensures we re-compile when either of the input changes
	baseName := strings.TrimSuffix(a.filename, filepath.Ext(a.filename))
	outputFile := filepath.Join(config.RuntimeCompilerOutputDir, fmt.Sprintf("%s-%d-%s.o", baseName, kv, hash))
	if _, err := os.Stat(outputFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error stat-ing output file %s: %w", outputFile, err)
		}
		comp, err := compiler.NewEBPFCompiler(config.KernelHeadersDirs, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("failed to create compiler: %w", err)
		}

		if err := comp.CompileToObjectFile(inputFile, outputFile, cflags); err != nil {
			return nil, fmt.Errorf("failed to compile runtime version of %s: %s", a.filename, err)
		}
	}
	return os.Open(outputFile)
}
