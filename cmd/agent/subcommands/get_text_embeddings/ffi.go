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
	"unsafe"
)

func printWithRust(text string) error {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	C.dd_get_text_embeddings_print(cText)
	return nil
}
