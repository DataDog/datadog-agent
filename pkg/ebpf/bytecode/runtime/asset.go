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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers"
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

// CompileOptions are options used to compile eBPF programs at runtime
type CompileOptions struct {
	// AdditionalFlags are extra flags passed to clang
	AdditionalFlags []string
	// ModifyCallback is a callback function that is allowed to modify the contents before compilation
	ModifyCallback func(in io.Reader, out io.Writer) error
	// UseKernelHeaders enables the inclusion of kernel headers from the host
	UseKernelHeaders bool
}

// Compile compiles the asset to an object file, writes it to the configured output directory, and
// then opens and returns the compiled output
func (a *asset) Compile(config *ebpf.Config, additionalFlags []string) (CompiledOutput, error) {
	return a.compile(config, CompileOptions{AdditionalFlags: additionalFlags, UseKernelHeaders: true})
}

// CompileWithOptions is the same as Compile, but takes an options struct with additional choices.
func (a *asset) CompileWithOptions(config *ebpf.Config, opts CompileOptions) (CompiledOutput, error) {
	return a.compile(config, opts)
}

func (a *asset) compile(config *ebpf.Config, opts CompileOptions) (CompiledOutput, error) {
	log.Debugf("starting runtime compilation of %s", a.filename)

	start := time.Now()
	a.tm.compilationEnabled = true
	defer func() {
		a.tm.compilationDuration = time.Since(start)
		a.tm.SubmitTelemetry(a.filename)
	}()

	var kernelHeaders []string
	if opts.UseKernelHeaders {
		headerOpts := headers.HeaderOptions{
			DownloadEnabled: config.EnableKernelHeaderDownload,
			Dirs:            config.KernelHeadersDirs,
			DownloadDir:     config.KernelHeadersDownloadDir,
			AptConfigDir:    config.AptConfigDir,
			YumReposDir:     config.YumReposDir,
			ZypperReposDir:  config.ZypperReposDir,
		}
		kernelHeaders = headers.GetKernelHeaders(headerOpts)
		if len(kernelHeaders) == 0 {
			a.tm.compilationResult = headerFetchErr
			return nil, errors.New("unable to find kernel headers")
		}
	}

	a.tm.compilationResult = verificationError
	outputDir := config.RuntimeCompilerOutputDir

	p := filepath.Join(config.BPFDir, "runtime", a.filename)
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", p, err)
	}
	defer f.Close()

	if err := secureRuntimeDir(outputDir); err != nil {
		return nil, err
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
	if opts.ModifyCallback != nil {
		outBuf := &bytes.Buffer{}
		// seek to the start and read all of protected file contents
		if _, err := diskProtectedFile.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek disk protected file: %w", err)
		}

		// run modify callback
		if err := opts.ModifyCallback(diskProtectedFile, outBuf); err != nil {
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

	out, result, err := compileToObjectFile(protectedFile.Name(), outputDir, a.filename, hash, opts.AdditionalFlags, kernelHeaders)
	a.tm.compilationResult = result

	return out, err
}

// secureRuntimeDir creates the runtime-compiler output directory (root-only,
// 0700) and verifies that it, and every ancestor up to the filesystem root, is a
// real directory owned by root and not writable by other users. The compiler
// writes object files here and system-probe later re-reads them to load into the
// kernel as root, so the whole path must be under root's control.
//
// It rejects the directory if any ancestor is a symlink, is not a directory, is
// not owned by root, or is writable by group or other without the sticky bit
// set. The sticky-bit exception permits the default location under /var/tmp
// (root-owned, mode 1777): the sticky bit keeps other users from renaming or
// deleting the datadog-agent directory that root creates beneath it.
func secureRuntimeDir(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return fmt.Errorf("unable to create compiler output directory %s: %w", outputDir, err)
	}

	abs, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("unable to resolve compiler output directory %s: %w", outputDir, err)
	}

	// Collect every path component from outputDir up to the filesystem root.
	var components []string
	for p := filepath.Clean(abs); ; {
		components = append(components, p)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}

	for _, p := range components {
		info, err := os.Lstat(p)
		if err != nil {
			return fmt.Errorf("unable to verify compiler output directory component %s: %w", p, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to use compiler output directory: %s is a symlink", p)
		}
		if !info.IsDir() {
			return fmt.Errorf("refusing to use compiler output directory: %s is not a directory", p)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("unable to read ownership of compiler output directory component %s", p)
		}
		if stat.Uid != 0 {
			return fmt.Errorf("refusing to use compiler output directory: %s is not owned by root (uid=%d)", p, stat.Uid)
		}
		if info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("refusing to use compiler output directory: %s is writable by non-root and not sticky (mode=%#o)", p, info.Mode().Perm())
		}
	}
	return nil
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
		return errors.New("file content hash does not match expected value")
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
