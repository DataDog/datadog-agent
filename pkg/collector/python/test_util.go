// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

import "C"

func testGetSubprocessOutputEmptyArgs(t *testing.T) {
	var argv **C.char
	var env **C.char
	var cStdout *C.char
	var cStderr *C.char
	var cRetCode C.int
	var exception *C.char

	GetSubprocessOutput(argv, env, &cStdout, &cStderr, &cRetCode, &exception)
	assert.Nil(t, cStdout)
	assert.Nil(t, cStderr)
	assert.Equal(t, C.int(0), cRetCode)
	assert.Nil(t, exception)
}

func testGetSubprocessOutput(t *testing.T) {
	var argv []*C.char = []*C.char{C.CString("echo"), C.CString("hello world"), nil}
	var env **C.char
	var cStdout *C.char
	var cStderr *C.char
	var cRetCode C.int
	var exception *C.char

	GetSubprocessOutput(&argv[0], env, &cStdout, &cStderr, &cRetCode, &exception)
	assert.Equal(t, "hello world\n", C.GoString(cStdout))
	assert.Equal(t, "", C.GoString(cStderr))
	assert.Equal(t, C.int(0), cRetCode)
	assert.Nil(t, exception)
}

func testGetSubprocessOutputUnknownBin(t *testing.T) {
	// go will not start the command since 'unknown_command' bin does not
	// exists. This will result in 0 error code and empty output
	var argv []*C.char = []*C.char{C.CString("unknown_command"), nil}
	var env **C.char
	var cStdout *C.char
	var cStderr *C.char
	var cRetCode C.int
	var exception *C.char

	GetSubprocessOutput(&argv[0], env, &cStdout, &cStderr, &cRetCode, &exception)
	assert.Equal(t, "", C.GoString(cStdout))
	assert.Equal(t, "", C.GoString(cStderr))
	assert.Equal(t, C.int(0), cRetCode)
	assert.Nil(t, exception)
}

func testGetSubprocessOutputError(t *testing.T) {
	var argv []*C.char = []*C.char{C.CString("ls"), C.CString("does not exists"), nil}
	var env **C.char
	var cStdout *C.char
	var cStderr *C.char
	var cRetCode C.int
	var exception *C.char

	GetSubprocessOutput(&argv[0], env, &cStdout, &cStderr, &cRetCode, &exception)
	assert.Equal(t, "", C.GoString(cStdout))
	assert.NotEqual(t, "", C.GoString(cStderr))
	assert.NotEqual(t, C.int(0), cRetCode)
	assert.Nil(t, exception)
}
