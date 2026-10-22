// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"unsafe"
)

/*
#include <stdlib.h>
*/
import "C"

// CStringArrayToSlice converts an array of C strings to a slice of Go strings.
// The function will not free the memory of the C strings.
func CStringArrayToSlice(arrayPtr unsafe.Pointer) []string {
	if arrayPtr == nil {
		return nil
	}

	a := (**C.char)(arrayPtr)

	var length int
	forEachCString(a, func(_ *C.char) {
		length++
	})
	res := make([]string, 0, length)
	si, release := acquireInterner()
	defer release()
	forEachCString(a, func(s *C.char) {
		res = append(res, internCString(si, s))
	})
	return res
}

// internCStrings converts a metric name and hostname to interned Go strings.
// Both have low cardinality, but every new context retains them, so a fresh
// copy per submission costs memory proportional to the number of contexts.
func internCStrings(name, hostname *C.char) (string, string) {
	si, release := acquireInterner()
	defer release()
	return internCString(si, name), internCString(si, hostname)
}

func internCString(si *stringInterner, s *C.char) string {
	if s == nil {
		return ""
	}
	return si.intern(unsafe.Slice((*byte)(unsafe.Pointer(s)), cstrlen(s)))
}

// cstrlen returns the length of a null-terminated C string. It's an alternative
// to calling C.strlen, which avoids the overhead of doing a cgo call.
func cstrlen(s *C.char) (len int) {
	// TODO: This is ~13% of the CPU time of Benchmark_cStringArrayToSlice.
	// Optimize using SWAR or similar vector techniques?
	for ; *s != 0; s = (*C.char)(unsafe.Add(unsafe.Pointer(s), 1)) {
		len++
	}
	return
}

// forEachCString iterates over a null-terminated array of C strings and calls
// the given function for each string.
func forEachCString(a **C.char, f func(*C.char)) {
	for ; a != nil && *a != nil; a = (**C.char)(unsafe.Add(unsafe.Pointer(a), unsafe.Sizeof(a))) {
		f(*a)
	}
}

// testHelperSliceToCStringArray converts a slice of Go strings to an array of C strings.
// It's a test helper, but it can't be declared in a _test.go file because cgo
// is not allowed there.
func testHelperSliceToCStringArray(s []string) **C.char {
	cArray := (**C.char)(C.malloc(C.size_t(len(s)+1) * C.size_t(unsafe.Sizeof((*C.char)(nil)))))
	for i, str := range s {
		*(**C.char)(unsafe.Add(unsafe.Pointer(cArray), uintptr(i)*unsafe.Sizeof(cArray))) = C.CString(str)
	}
	*(**C.char)(unsafe.Add(unsafe.Pointer(cArray), uintptr(len(s))*unsafe.Sizeof(cArray))) = nil
	return cArray
}
