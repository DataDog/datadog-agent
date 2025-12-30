// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck && test

package ffi

/*
#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
#include "ffi.h"
*/
import "C"

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test null Library struct pointer
func testNilLibraryStruct(t *testing.T) {
	loader := NewSharedLibraryLoader("")

	err := loader.Run(nil, "", "", "")
	assert.EqualError(t, err, "pointer to 'Library' struct is null")

	_, err = loader.Version(nil)
	assert.EqualError(t, err, "pointer to 'Library' struct is null")

	err = loader.Close(nil)
	assert.EqualError(t, err, "pointer to 'Library' struct is null")
}

// Test null symbol pointers cases
func testLibraryStructWithNilSymbols(t *testing.T) {
	lib := NewNullSymbolLibrary()
	loader := NewSharedLibraryLoader("")

	err := loader.Run(lib, "", "", "")
	assert.EqualError(t, err, "Failed to run check: pointer to 'Run' symbol of the shared library is NULL")

	_, err = loader.Version(lib)
	assert.EqualError(t, err, "Failed to get version: pointer to 'Version' symbol of the shared library is NULL")
}
