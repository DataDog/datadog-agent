// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testcommon

// #include <datadog_agent_rtloader.h>
//
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

// GetRtLoader returns a RtLoader instance
func GetRtLoader() *C.rtloader_t {
	var err *C.char = nil

	executablePath := C.CString("/folder/mock_python_interpeter_bin_path")
	defer C.free(unsafe.Pointer(executablePath))

	var pythonHome *C.char

	// Specific setup for when we're running the tests with bazel
	if isBazel() {
		if runtime.GOOS == "windows" {
			// Temporarily add the path to where the "three" dll is available to PATH
			// so that it can be found by `LoadLibrary`
			threeDirPath := rlocationPathFromEnv("THREE_PATH")
			oldPath := os.Getenv("PATH")
			defer os.Setenv("PATH", oldPath)
			os.Setenv("PATH", threeDirPath+";"+os.Getenv("PATH"))
			// On Windows, python_home is the directory produced by dir_with_python_home
			// (copy_to_directory of @cpython//:python_win), placed under _main/ in the
			// runfiles so it is reachable via the standard junction chain.
			pythonHomeStr := rlocationPathFromEnv("PYTHON_HOME")
			pythonHome = C.CString(pythonHomeStr)
			defer C.free(unsafe.Pointer(pythonHome))

			debugWindowsSetup(threeDirPath, pythonHomeStr)
		} else {
			// Python Home is a level up from the path to the binary
			pythonHome = C.CString(filepath.Dir(filepath.Dir(rlocationPathFromEnv("PYTHON_BIN"))))
			defer C.free(unsafe.Pointer(pythonHome))
		}
	}

	rtloader := C.make3(pythonHome, executablePath, &err)
	if err != nil {
		fmt.Printf("Error: %s\n", C.GoString(err))
		return rtloader
	}
	addStubsToPythonPath(rtloader)
	return rtloader
}

// addStubsToPythonPath puts Python stubs to the PYTHONPATH, based on whether tests
// are running from bazel and OS
func addStubsToPythonPath(rtloader *C.rtloader_t) {
	if isBazel() {
		C.add_python_path(rtloader, C.CString(rlocationPathFromEnv("STUBS_LOCATION")))
	} else {
		// Pre-bazel default
		C.add_python_path(rtloader, C.CString("../python"))
	}
}

func isBazel() bool {
	return os.Getenv("BAZEL_TEST") == "1"
}

func rlocationPathFromEnv(envvar string) string {
	resolved, err := runfiles.Rlocation(os.Getenv(envvar))
	if err != nil {
		panic(fmt.Sprintf("error: failed to get location for `three` library: %s", err))
	}
	return resolved
}

func debugWindowsSetup(threeDirPath, pythonHomeStr string) {
	dllPath := filepath.Join(threeDirPath, "libdatadog-agent-three.dll")
	python313Path := filepath.Join(pythonHomeStr, "python313.dll")

	if out, _ := exec.Command("whoami").CombinedOutput(); len(out) > 0 {
		fmt.Fprintf(os.Stderr, "[rtloader debug] current user: %s", out)
	}
	fmt.Fprintf(os.Stderr, "[rtloader debug] THREE_PATH (raw): %s\n", os.Getenv("THREE_PATH"))
	fmt.Fprintf(os.Stderr, "[rtloader debug] THREE_PATH (resolved): %s\n", threeDirPath)
	fmt.Fprintf(os.Stderr, "[rtloader debug] PYTHON_HOME (raw): %s\n", os.Getenv("PYTHON_HOME"))
	fmt.Fprintf(os.Stderr, "[rtloader debug] python_home (passed to SetDllDirectory): %s\n", pythonHomeStr)

	for _, path := range []string{dllPath, python313Path} {
		if info, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "[rtloader debug] stat %s: error: %v\n", path, err)
		} else {
			fmt.Fprintf(os.Stderr, "[rtloader debug] stat %s: size=%d\n", path, info.Size())
		}
		if out, _ := exec.Command("icacls", path).CombinedOutput(); len(out) > 0 {
			fmt.Fprintf(os.Stderr, "[rtloader debug] icacls %s:\n%s", path, out)
		}
	}

	tryLoadSequence(dllPath, pythonHomeStr)
}
