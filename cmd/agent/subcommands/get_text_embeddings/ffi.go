// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux || darwin) && cgo

package get_text_embeddings

/*
#cgo CFLAGS: -I${SRCDIR}/../../../../pkg/get_text_embeddings/rust/include
#cgo LDFLAGS: -L${SRCDIR}/../../../../pkg/get_text_embeddings/rust/target/release -lget_text_embeddings -Wl,-rpath,${SRCDIR}/../../../../pkg/get_text_embeddings/rust/target/release
#include <stdlib.h>
#include "get_text_embeddings.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func Init() error {
	var err *C.char
	C.dd_get_text_embeddings_init(&err)
	if err != nil {
		return fmt.Errorf("failed to initialize get_text_embeddings: %s", C.GoString(err))
	}
	return nil
}

func GetEmbeddingsSize() (int, error) {
	size := C.dd_get_text_embeddings_get_embeddings_size()
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
	C.dd_get_text_embeddings_get_embeddings(cText, (*C.float)(unsafe.Pointer(&buffer[0])), &errPtr)
	if errPtr != nil {
		return nil, fmt.Errorf("failed to get embeddings: %s", C.GoString(errPtr))
	}

	return buffer, nil
}
