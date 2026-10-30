// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

// Package compiler is the runtime compiler for eBPF
package compiler

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	datadogAgentEmbeddedPath = filepath.Join(defaultpaths.GetInstallPath(), "embedded")
	clangBinPath             = filepath.Join(datadogAgentEmbeddedPath, "bin/clang-bpf")

	//go:embed stdarg.h
	stdargHData []byte
)

const compilationStepTimeout = 60 * time.Second

func writeStdarg() (string, error) {
	tmpIncludeDir, err := os.MkdirTemp(os.TempDir(), "include-")
	if err != nil {
		return "", fmt.Errorf("error creating temporary include directory: %s", err.Error())
	}

	if err = os.WriteFile(filepath.Join(tmpIncludeDir, "stdarg.h"), stdargHData, 0644); err != nil {
		return "", fmt.Errorf("error writing data to stdarg.h: %s", err.Error())
	}
	return tmpIncludeDir, nil
}

func kernelHeaderPaths(headerDirs []string) []string {
	arch := kernel.Arch()
	var paths []string
	for _, d := range headerDirs {
		paths = append(paths,
			d+"/arch/"+arch+"/include",
			d+"/arch/"+arch+"/include/generated",
			d+"/include",
			d+"/arch/"+arch+"/include/uapi",
			d+"/arch/"+arch+"/include/generated/uapi",
			d+"/include/uapi",
			d+"/include/generated/uapi",
		)
	}
	return paths
}

// CompileToObjectFile compiles an eBPF program
func CompileToObjectFile(inFile, outputFile string, cflags []string, headerDirs []string) error {
	tmpIncludeDir, err := writeStdarg()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpIncludeDir)
	cflags = append(cflags, "-isystem"+tmpIncludeDir)

	kps := kernelHeaderPaths(headerDirs)
	for _, p := range kps {
		cflags = append(cflags, "-isystem"+p)
	}

	cflags = append(cflags, "-c", "-x", "c", "-o", "-", inFile)
	clangOut := &bytes.Buffer{}
	if err := clang(cflags, WithStdout(clangOut)); err != nil {
		return fmt.Errorf("compiling asset to bytecode: %w", err)
	}
	if err := llc(clangOut, outputFile); err != nil {
		return fmt.Errorf("error compiling bytecode to object file: %w", err)
	}
	return nil
}

// WithStdin assigns the provided io.Reader as Stdin for the command
func WithStdin(in io.Reader) func(*exec.Cmd) {
	return func(c *exec.Cmd) {
		c.Stdin = in
	}
}

// WithStdout assigns the provided io.Writer as Stdout for the command
func WithStdout(out io.Writer) func(*exec.Cmd) {
	return func(c *exec.Cmd) {
		c.Stdout = out
	}
}

func clang(cflags []string, options ...func(*exec.Cmd)) error {
	var clangErr bytes.Buffer

	// We execute the verified descriptor rather than the path, so the binary that
	// runs as root is exactly the one that was verified.
	clangBin, err := openRootOwnedExecutable(clangBinPath)
	if err != nil {
		return fmt.Errorf("refusing to execute clang: %w", err)
	}
	defer clangBin.Close()

	clangCtx, clangCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer clangCancel()

	// The descriptor is O_CLOEXEC, but it stays open in the forked child until
	// execve succeeds, so /proc/self/fd/N resolves to the verified file there.
	compileToBC := exec.CommandContext(clangCtx, fmt.Sprintf("/proc/self/fd/%d", clangBin.Fd()), cflags...)
	compileToBC.Args[0] = clangBinPath
	for _, opt := range options {
		opt(compileToBC)
	}
	compileToBC.Stderr = &clangErr

	log.Debugf("running clang: %v", compileToBC.Args)

	if err := compileToBC.Run(); err != nil {
		var errMsg string
		if clangCtx.Err() == context.DeadlineExceeded {
			errMsg = "operation timed out"
		} else if len(clangErr.String()) > 0 {
			errMsg = clangErr.String()
		} else {
			errMsg = err.Error()
		}
		return fmt.Errorf("clang: %s", errMsg)
	}

	if len(clangErr.String()) > 0 {
		log.Debugf("%s", clangErr.String())
	}
	return nil
}

// openRootOwnedExecutable opens path with O_NOFOLLOW and verifies, on the opened
// descriptor, that it is a regular file owned by root:root and not writable by
// group or other. system-probe runs as root, so anything it executes must not be
// modifiable by a less privileged user. The caller must close the returned file.
func openRootOwnedExecutable(path string) (*os.File, error) {
	// O_NONBLOCK keeps a FIFO planted at path from blocking the open forever; it
	// has no effect on regular files, and anything else is rejected below.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("error opening %s: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("error stat-ing %s: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		f.Close()
		return nil, fmt.Errorf("error getting ownership of %s", path)
	}
	if !info.Mode().IsRegular() || stat.Uid != 0 || stat.Gid != 0 || info.Mode().Perm()&0022 != 0 {
		f.Close()
		return nil, fmt.Errorf("%s has incorrect permissions: user=%v, group=%v, mode=%v", path, stat.Uid, stat.Gid, info.Mode())
	}
	return f, nil
}

// llc reproduces `llc -march=bpf -filetype=obj` output verbatim.
func llc(in io.Reader, outputFile string) error {
	return clang([]string{
		"-O2",
		"-Wno-override-module",
		"-Xclang", "-disable-llvm-optzns",
		"-c",
		"-fno-addrsig",
		"-o", outputFile,
		"-target", "bpf",
		"-x", "ir", "-",
	}, WithStdin(in))
}

// Preprocess runs the clang preprocessor on `in` and writes the output to `out`
func Preprocess(in io.Reader, out io.Writer, cflags []string, headerDirs []string) error {
	tmpIncludeDir, err := writeStdarg()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpIncludeDir)
	cflags = append(cflags, "-isystem"+tmpIncludeDir)

	kps := kernelHeaderPaths(headerDirs)
	for _, p := range kps {
		cflags = append(cflags, "-isystem"+p)
	}

	cflags = append(cflags, "-E", "-x", "c", "-o", "-", "-")
	return clang(cflags, WithStdin(in), WithStdout(out))
}
