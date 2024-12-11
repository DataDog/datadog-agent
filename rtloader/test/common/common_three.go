// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build three

package testcommon

// #include <datadog_agent_rtloader.h>
//
import "C"

import "unsafe"

// UsingTwo states whether we're using Two as backend
const UsingTwo bool = false

// GetRtLoader returns a RtLoader instance using Three
func GetRtLoader() *C.rtloader_t {
	var err *C.char = nil

	executablePath := C.CString("/folder/mock_python_interpeter_bin_path")
	defer C.free(unsafe.Pointer(executablePath))

	return C.make3(nil, executablePath, &err)
}
