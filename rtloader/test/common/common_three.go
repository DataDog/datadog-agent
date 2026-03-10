// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testcommon

// #include <datadog_agent_rtloader.h>
//
import "C"

import (
	"os"
	"path/filepath"
	"unsafe"
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

	return C.make3(pythonHome, executablePath, &err)
}
