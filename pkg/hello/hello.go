// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package hello provides a C++ function that prints hello world.
package hello

/*
#include "hello.h"
*/
import "C"
import "errors"

func Benchmark() error {
	var error *C.char
	C.benchmark(&error)
	if error != nil {
		return errors.New(C.GoString(error))
	}
	return nil
}
