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
	// Use an explicit "Python Home" (the root directory of the Python installation)
	// Intended for driving the tests from Bazel
	if pythonBin := os.Getenv("PYTHON_BIN"); pythonBin != "" {
		// PYTHON_BIN points to the python executable under bin
		// Get the full path based on Bazel-provided variables
		absPath := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("TEST_WORKSPACE"), pythonBin)
		// And then walk back to get the "Python Home"
		pythonHome = C.CString(filepath.Dir(filepath.Dir(absPath)))
		defer C.free(unsafe.Pointer(pythonHome))
	}

	if pythonLib := os.Getenv("PYTHON_LIB"); pythonLib != "" {
		absPath, err := runfiles.Rlocation(pythonLib)
		if err != nil {
			panic(fmt.Sprintf("error: failed to get location for `python lib`: %s", err))
		}
		pythonHome = C.CString(filepath.Dir(absPath))
	}

	if stubsLocation := os.Getenv("STUBS_LOCATION"); stubsLocation != "" {
		absPath, err := runfiles.Rlocation(stubsLocation)
		if err != nil {
			panic(fmt.Sprintf("error: failed to get location for `python stubs`: %s", err))
		}
		os.Setenv("PYTHONPATH", absPath)
		fmt.Printf("stubs are in: %s\n", absPath)
	}

	if runtime.GOOS == "windows" {
		// Add the full path to where the "three" dll is available to PATH
		// THREE_PATH is given relative to the bazel execroot, and tests on windows
		// run from the execroot.
		// On windows, the ways to control the search path for dll's is limited,
		// modifying PATH being the most practical in this setting.
		if threePath := os.Getenv("THREE_PATH"); threePath != "" {
			absThreePath, err := runfiles.Rlocation(threePath)
			if err != nil {
				panic("error: failed to get location for `three` library")
			}
			os.Setenv("PATH", absThreePath + ";" + os.Getenv("PATH"))
		}
		fmt.Printf("PATH: %s\n", os.Getenv("PATH"))
	}

	rv := C.make3(pythonHome, executablePath, &err)
	if err != nil {
		fmt.Printf("Error: %s", C.GoString(err))
	}
	return rv
}
