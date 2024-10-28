// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils groups multiple utils function that can be used by the secl package
package utils

import (
	"errors"
	"unsafe"
)

// LabelSet represents an abstracted set of labels that can be used to call PprofDoWithoutContext
type LabelSet struct {
	inner *map[string]string
}

// NewLabelSet returns a new LabelSet based on the labels provided as a pair number of arguments (key, value, key value ....)
func NewLabelSet(labels ...string) (LabelSet, error) {
	if len(labels)%2 != 0 {
		return LabelSet{}, errors.New("non-pair label set values")
	}

	set := make(map[string]string)
	for i := 0; i+1 < len(labels); i += 2 {
		set[labels[i]] = labels[i+1]
	}

	return LabelSet{
		inner: &set,
	}, nil
}

type labelMap map[string]string

//go:linkname runtimeSetProfLabel runtime/pprof.runtime_setProfLabel
func runtimeSetProfLabel(labels unsafe.Pointer)

//go:linkname runtimeGetProfLabel runtime/pprof.runtime_getProfLabel
func runtimeGetProfLabel() unsafe.Pointer

func setGoroutineLabels(labels *labelMap) {
	runtimeSetProfLabel(unsafe.Pointer(labels))
}

func getGoroutineLabels() *labelMap {
	return (*labelMap)(runtimeGetProfLabel())
}

// PprofDoWithoutContext does the same thing as https://pkg.go.dev/runtime/pprof#Do, but without the allocation resulting
// from the usage of context values. This function also directly takes a map of labels, instead of incuring allocations
// when converting from one format (LabelSet) to the other (map).
func PprofDoWithoutContext(labelSet LabelSet, f func()) {
	previousLabels := getGoroutineLabels()
	defer setGoroutineLabels(previousLabels)

	setGoroutineLabels((*labelMap)(labelSet.inner))
	f()
}
