// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux || darwin) && cgo

// DeepInference is a library for doing machine learning inference on CPU relying on Rust / Candle.
package deepinference

/*
#cgo CFLAGS: -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust/target/release -ldeepinference -Wl,-rpath,${SRCDIR}/rust/target/release
#include <stdlib.h>
#include "deepinference.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func Init() error {
	var err *C.char
	C.dd_deepinference_init(&err)
	if err != nil {
		return fmt.Errorf("failed to initialize deepinference: %s", C.GoString(err))
	}
	return nil
}

func GetEmbeddingsSize() (int, error) {
	size := C.dd_deepinference_get_embeddings_size()
	return int(size), nil
}

func GetEmbeddings(text string) ([]float32, error) {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	size, err := GetEmbeddingsSize()
	if err != nil {
		return nil, fmt.Errorf("failed to get embeddings size: %s", err)
	}

	buffer := make([]float32, size)
	var errPtr *C.char
	C.dd_deepinference_get_embeddings(cText, (*C.float)(unsafe.Pointer(&buffer[0])), &errPtr)
	if errPtr != nil {
		return nil, fmt.Errorf("failed to get embeddings: %s", C.GoString(errPtr))
	}

	return buffer, nil
}

func Benchmark() error {
	var errPtr *C.char
	C.dd_deepinference_benchmark(&errPtr)
	if errPtr != nil {
		return fmt.Errorf("failed to benchmark deepinference: %s", C.GoString(errPtr))
	}
	return nil
}
