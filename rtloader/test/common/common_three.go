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
			oldPath := os.Getenv("PATH")
			defer os.Setenv("PATH", oldPath)
			os.Setenv("PATH", rlocationPathFromEnv("THREE_PATH")+";"+os.Getenv("PATH"))
			// On Windows, the python library file sits at the root of the Python Home
			pythonHome = C.CString(filepath.Dir(rlocationPathFromEnv("PYTHON_LIB")))
			defer C.free(unsafe.Pointer(pythonHome))
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
