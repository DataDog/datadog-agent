// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package testrtloader exercises the rtloader C API against Python stubs.
package testrtloader

/*
#include <stdlib.h>
#include "datadog_agent_rtloader.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

type remoteQueryCheck struct{ instance *C.rtloader_pyobject_t }

func newRemoteQueryCheck(moduleName string) (*remoteQueryCheck, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gil := C.ensure_gil(rtloader)
	defer C.release_gil(rtloader, gil)
	name := C.CString(moduleName)
	defer C.free(unsafe.Pointer(name))
	var module, class, instance *C.rtloader_pyobject_t
	if C.get_class(rtloader, name, &module, &class) == 0 {
		return nil, fetchError()
	}
	defer C.rtloader_decref(rtloader, module)
	defer C.rtloader_decref(rtloader, class)
	empty := C.CString("")
	defer C.free(unsafe.Pointer(empty))
	config := C.CString("{}")
	defer C.free(unsafe.Pointer(config))
	if C.get_check(rtloader, class, config, config, name, name, empty, &instance) == 0 {
		return nil, fetchError()
	}
	return &remoteQueryCheck{instance}, nil
}
func (c *remoteQueryCheck) close() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gil := C.ensure_gil(rtloader)
	defer C.release_gil(rtloader, gil)
	C.rtloader_decref(rtloader, c.instance)
}
func (c *remoteQueryCheck) supported() bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gil := C.ensure_gil(rtloader)
	defer C.release_gil(rtloader, gil)
	return C.has_remote_query(rtloader, c.instance) != 0
}
func (c *remoteQueryCheck) query(request string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gil := C.ensure_gil(rtloader)
	defer C.release_gil(rtloader, gil)
	input := C.CString(request)
	defer C.free(unsafe.Pointer(input))
	result := C.remote_query(rtloader, c.instance, input)
	if result == nil {
		err := fetchError()
		if err == nil {
			err = errors.New("no string outcome")
		}
		return "", err
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(result))
	return C.GoString(result), nil
}
func (c *remoteQueryCheck) cancel(runID string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gil := C.ensure_gil(rtloader)
	defer C.release_gil(rtloader, gil)
	id := C.CString(runID)
	defer C.free(unsafe.Pointer(id))
	if C.cancel_remote_query(rtloader, c.instance, id) == 0 {
		return fetchError()
	}
	return nil
}
