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
	"io/ioutil"
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

// CompileToObjectFile compiles an eBPF program
func CompileToObjectFile(in io.Reader, outputFile string, cflags []string, headerDirs []string) error {
	if len(headerDirs) == 0 {
		return fmt.Errorf("unable to find kernel headers")
	}

	arch := kernel.Arch()
	if arch == "" {
		return fmt.Errorf("unable to get kernel arch for %s", runtime.GOARCH)
	}

	tmpIncludeDir, err := ioutil.TempDir(os.TempDir(), "include-")
	if err != nil {
		return fmt.Errorf("error creating temporary include directory: %s", err.Error())
	}
	defer os.RemoveAll(tmpIncludeDir)

	if err = os.WriteFile(filepath.Join(tmpIncludeDir, "stdarg.h"), stdargHData, 0644); err != nil {
		return fmt.Errorf("error writing data to stdarg.h: %s", err.Error())
	}

	for _, d := range headerDirs {
		cflags = append(cflags,
			fmt.Sprintf("-isystem%s/arch/%s/include", d, arch),
			fmt.Sprintf("-isystem%s/arch/%s/include/generated", d, arch),
			fmt.Sprintf("-isystem%s/include", d),
			fmt.Sprintf("-isystem%s/arch/%s/include/uapi", d, arch),
			fmt.Sprintf("-isystem%s/arch/%s/include/generated/uapi", d, arch),
			fmt.Sprintf("-isystem%s/include/uapi", d),
			fmt.Sprintf("-isystem%s/include/generated/uapi", d),
		)
	}
	cflags = append(cflags, fmt.Sprintf("-isystem%s", tmpIncludeDir))
	cflags = append(cflags, "-c", "-x", "c", "-o", "-", "-")

	var clangOut, clangErr, llcErr bytes.Buffer

	clangCtx, clangCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer clangCancel()

	compileToBC := exec.CommandContext(clangCtx, clangBinPath, cflags...)
	compileToBC.Stdin = in
	compileToBC.Stdout = &clangOut
	compileToBC.Stderr = &clangErr

	log.Debugf("compiling asset to bytecode: %v", compileToBC.Args)

	err = compileToBC.Run()

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

	llcCtx, llcCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer llcCancel()

	bcToObj := exec.CommandContext(llcCtx, llcBinPath, "-march=bpf", "-filetype=obj", "-o", outputFile, "-")
	bcToObj.Stdin = &clangOut
	bcToObj.Stdout = nil
	bcToObj.Stderr = &llcErr

	log.Debugf("compiling bytecode to object file: %v", bcToObj.Args)

	err = bcToObj.Run()
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
		log.Debugf("%s", clangErr.String())
	}

	return nil
}
