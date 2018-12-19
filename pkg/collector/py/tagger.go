// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

// #cgo pkg-config: python-3.7
// #cgo linux CFLAGS: -std=gnu99
// #include "tagger.h"
import "C"
import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

// GetTags queries the agent6 tagger and returns a string array containing
// tags for the entity. If entity not found, or tagging error, the returned
// array is empty but valid.
//export GetTags
// FIXME: replace highCard with a TagCardinality
func GetTags(id *C.char, highCard int) *C.PyObject {
	goID := C.GoString(id)
	var highCardBool bool
	var tags []string
	if highCard > 0 {
		highCardBool = true
	}

	if highCardBool == true {
		tags, _ = tagger.Tag(goID, collectors.HighCardinality)
	} else {
		tags, _ = tagger.Tag(goID, collectors.LowCardinality)
	}
	output := C.PyList_New(0)

	for _, t := range tags {
		cTag := C.CString(t)
		pyTag := C.PyUnicode_FromString(cTag)
		defer C.Py_DecRef(pyTag)
		C.free(unsafe.Pointer(cTag))
		C.PyList_Append(output, pyTag)
	}

	return output
}

func initTagger() {
	C.register_tagger_module()
}
