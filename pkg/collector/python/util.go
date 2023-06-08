// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

/*
#include <datadog_agent_rtloader.h>
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static
*/
import "C"

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
)

// GetSubprocessOutput runs the subprocess and returns the output
// Indirectly used by the C function `get_subprocess_output` that's mapped to `_util.get_subprocess_output`.
//
//export GetSubprocessOutput
func GetSubprocessOutput(argv **C.char, env **C.char, cStdout **C.char, cStderr **C.char, cRetCode *C.int, exception **C.char) {
	subprocessArgs := cStringArrayToSlice(argv)
	// this should never happen as this case is filtered by rtloader
	if len(subprocessArgs) == 0 {
		return
	}

	ctx, _ := GetSubprocessContextCancel()
	cmd := exec.CommandContext(ctx, subprocessArgs[0], subprocessArgs[1:]...)

	subprocessEnv := cStringArrayToSlice(env)
	if len(subprocessEnv) != 0 {
		cmd.Env = subprocessEnv
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		*exception = TrackedCString(fmt.Sprintf("internal error creating stdout pipe: %v", err))
		return
	}

	var wg sync.WaitGroup
	var output []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		output, _ = io.ReadAll(stdout)
	}()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		*exception = TrackedCString(fmt.Sprintf("internal error creating stderr pipe: %v", err))
		return
	}

	var outputErr []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		outputErr, _ = io.ReadAll(stderr)
	}()

	cmd.Start() //nolint:errcheck

	// Wait for the pipes to be closed *before* waiting for the cmd to exit, as per os.exec docs
	wg.Wait()

	retCode := 0
	err = cmd.Wait()
	if exiterr, ok := err.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			retCode = status.ExitStatus()
		}
	}

	*cStdout = TrackedCString(string(output))
	*cStderr = TrackedCString(string(outputErr))
	*cRetCode = C.int(retCode)
}
