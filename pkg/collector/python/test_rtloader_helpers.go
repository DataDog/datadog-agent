// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"unsafe"
)

// #include <datadog_agent_rtloader.h>
//
import "C"

var (
	// Real object addresses of size 0 that we can use as mock pointers
	mockRtLoaderPtr         = [0]byte{}
	mockRtLoaderPyObjectPtr = [0]byte{}
)

// Since Golang 1.15, we are no longer able to instantiate an opaque pointer to a
// struct since it was an incomplete type. To ensure that we can unit test our
// rtloader supporting logic that does a lot of sanity checks like `somePtr != nil`
// we have to have a non-nil pointer that we can pass around (but not directly use).
// By casting the location of our 0-size arrays to the proper type, we are able to
// create such mock objects for the sole purpose of testing while also making CGO
// compilation happy.

func newMockRtLoaderPtr() *C.rtloader_t {
	return (*C.rtloader_t)(unsafe.Pointer(&mockRtLoaderPtr))
}

func newMockPyObjectPtr() *C.rtloader_pyobject_t {
	return (*C.rtloader_pyobject_t)(unsafe.Pointer(&mockRtLoaderPyObjectPtr))
}
