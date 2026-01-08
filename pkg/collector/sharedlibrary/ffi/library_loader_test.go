// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck && test

// We cannot use Cgo in Go test files, so the actual tests are located in a different file.
// This file is basically a redirection to `test_library_loader.go`

package ffi

import (
	"testing"
)

func TestNullLibraryPointer(t *testing.T) {
	testNullLibraryPointer(t)
}

func TestLibraryWithNullSymbols(t *testing.T) {
	testLibraryWithNullSymbols(t)
}
