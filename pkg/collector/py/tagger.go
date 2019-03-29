// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include "tagger.h"
import "C"
import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

// Tag bridges towards tagger.Tag to retrieve container tags
//export Tag
func Tag(id *C.char, card C.TaggerCardinality) *C.PyObject {
	goID := C.GoString(id)
	var tags []string
	var cardinality collectors.TagCardinality

	switch card {
	case C.LOW_CARD:
		cardinality = collectors.LowCardinality
	case C.ORCHESTRATOR_CARD:
		cardinality = collectors.OrchestratorCardinality
	case C.HIGH_CARD:
		cardinality = collectors.HighCardinality
	}

	tags, _ = tagger.Tag(goID, cardinality)
	output := C.PyList_New(0)

	for _, t := range tags {
		cTag := C.CString(t)
		pyTag := C.PyString_FromString(cTag)
		defer C.Py_DecRef(pyTag)
		C.free(unsafe.Pointer(cTag))
		C.PyList_Append(output, pyTag)
	}

	return output
}

// GetTags is deprecated, Tag should be used now instead
//export GetTags
func GetTags(id *C.char, highCard int) *C.PyObject {
	if highCard > 0 {
		return Tag(id, C.HIGH_CARD)
	} else {
		return Tag(id, C.LOW_CARD)
	}
}

func initTagger() {
	C.inittagger()
}
