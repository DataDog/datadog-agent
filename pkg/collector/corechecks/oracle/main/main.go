// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is the entry point for the Oracle check.
package main

// #cgo CFLAGS: -I../../../cshared/include
// #include "check_wrapper.h"
import "C"

import (
	"runtime"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle"
	"github.com/DataDog/datadog-agent/pkg/collector/cshared/pinner"
)

var checkPinner runtime.Pinner

func main() {}

// oracleLoadCheck exports an Oracle check instance
//
//export oracleLoadCheck
func oracleLoadCheck() *C.c_check_wrapper_t {
	factory := oracle.Factory()
	if checkFunc, ok := factory.Get(); ok {
		c := checkFunc()
		pinner.Pin(checkPinner, c)
		ptr := unsafe.Pointer(&c)
		pinner.Pin(checkPinner, ptr)
		return C.newCheckWrapper(ptr)
	}
	return nil
}

// go build -tags oracle -o libcheckoracle.so -buildmode=c-shared ./pkg/collector/corechecks/oracle/main/main.go
