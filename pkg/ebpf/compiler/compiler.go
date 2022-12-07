// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

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
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	datadogAgentEmbeddedPath = "/opt/datadog-agent/embedded"
	clangBinPath             = filepath.Join(datadogAgentEmbeddedPath, "bin/clang-bpf")
	llcBinPath               = filepath.Join(datadogAgentEmbeddedPath, "bin/llc-bpf")

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

func kernelHeaderPaths(headerDirs []string) ([]string, error) {
	arch := kernel.Arch()
	if arch == "" {
		return nil, fmt.Errorf("unable to get kernel arch for %s", runtime.GOARCH)
	}

	var paths []string
	for _, d := range headerDirs {
		paths = append(paths,
			fmt.Sprintf("%s/arch/%s/include", d, arch),
			fmt.Sprintf("%s/arch/%s/include/generated", d, arch),
			fmt.Sprintf("%s/include", d),
			fmt.Sprintf("%s/arch/%s/include/uapi", d, arch),
			fmt.Sprintf("%s/arch/%s/include/generated/uapi", d, arch),
			fmt.Sprintf("%s/include/uapi", d),
			fmt.Sprintf("%s/include/generated/uapi", d),
		)
	}
	return paths, nil
}

// CompileToObjectFile compiles an eBPF program
func CompileToObjectFile(in io.Reader, outputFile string, cflags []string, headerDirs []string) error {
	if len(headerDirs) == 0 {
		return fmt.Errorf("unable to find kernel headers")
	}

	tmpIncludeDir, err := writeStdarg()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpIncludeDir)
	cflags = append(cflags, fmt.Sprintf("-isystem%s", tmpIncludeDir))

	kps, err := kernelHeaderPaths(headerDirs)
	if err != nil {
		return err
	}
	for _, p := range kps {
		cflags = append(cflags, fmt.Sprintf("-isystem%s", p))
	}

	cflags = append(cflags, "-c", "-x", "c", "-o", "-", "-")
	clangOut := &bytes.Buffer{}
	if err := clang(in, clangOut, cflags); err != nil {
		return err
	}
	return llc(clangOut, outputFile)
}

func clang(in io.Reader, out io.Writer, cflags []string) error {
	var clangErr bytes.Buffer

	clangCtx, clangCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer clangCancel()

	compileToBC := exec.CommandContext(clangCtx, clangBinPath, cflags...)
	compileToBC.Stdin = in
	compileToBC.Stdout = out
	compileToBC.Stderr = &clangErr

	log.Debugf("compiling asset to bytecode: %v", compileToBC.Args)

	err := compileToBC.Run()
	if err != nil {
		var errMsg string
		if clangCtx.Err() == context.DeadlineExceeded {
			errMsg = "operation timed out"
		} else if len(clangErr.String()) > 0 {
			errMsg = clangErr.String()
		} else {
			errMsg = err.Error()
		}
		return fmt.Errorf("error compiling asset to bytecode: %s", errMsg)
	}

	if len(clangErr.String()) > 0 {
		log.Debugf("%s", clangErr.String())
	}
	return nil
}

func llc(in io.Reader, outputFile string) error {
	var llcErr bytes.Buffer
	llcCtx, llcCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer llcCancel()

	bcToObj := exec.CommandContext(llcCtx, llcBinPath, "-march=bpf", "-filetype=obj", "-o", outputFile, "-")
	bcToObj.Stdin = in
	bcToObj.Stdout = nil
	bcToObj.Stderr = &llcErr

	log.Debugf("compiling bytecode to object file: %v", bcToObj.Args)

	err := bcToObj.Run()
	if err != nil {
		var errMsg string
		if llcCtx.Err() == context.DeadlineExceeded {
			errMsg = "operation timed out"
		} else if len(llcErr.String()) > 0 {
			errMsg = llcErr.String()
		} else {
			errMsg = err.Error()
		}
		return fmt.Errorf("error compiling bytecode to object file: %s", errMsg)
	}

	if len(llcErr.String()) > 0 {
		log.Debugf("%s", llcErr.String())
	}
	return nil
}

func Preprocess(in io.Reader, out io.Writer, cflags []string, headerDirs []string) error {
	if len(headerDirs) == 0 {
		return fmt.Errorf("unable to find kernel headers")
	}
	tmpIncludeDir, err := writeStdarg()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpIncludeDir)
	cflags = append(cflags, fmt.Sprintf("-isystem%s", tmpIncludeDir))

	kps, err := kernelHeaderPaths(headerDirs)
	if err != nil {
		return err
	}
	for _, p := range kps {
		cflags = append(cflags, fmt.Sprintf("-isystem%s", p))
	}

	cflags = append(cflags, "-E", "-x", "c", "-o", "-", "-")
	return clang(in, out, cflags)
}
