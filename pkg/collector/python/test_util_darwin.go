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

func testGetSubprocessOutputEnv(t *testing.T) {
	var argv []*C.char = []*C.char{C.CString("bash"), C.CString("-c"), C.CString("echo $BAZ"), nil}
	var env []*C.char = []*C.char{C.CString("FOO=BAR"), C.CString("BAZ=QUX"), nil}
	var cStdout *C.char
	var cStderr *C.char
	var cRetCode C.int
	var exception *C.char

	GetSubprocessOutput(&argv[0], &env[0], &cStdout, &cStderr, &cRetCode, &exception)
	assert.Equal(t, "QUX\n", C.GoString(cStdout))
	assert.Equal(t, "", C.GoString(cStderr))
	assert.Equal(t, C.int(0), cRetCode)
	assert.Nil(t, exception)
}
