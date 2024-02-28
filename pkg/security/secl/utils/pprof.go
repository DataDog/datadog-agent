// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils groups multiple utils function that can be used by the secl package
package utils

import (
	"unsafe"
)

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
func PprofDoWithoutContext(labels map[string]string, f func()) {
	previousLabels := getGoroutineLabels()
	defer setGoroutineLabels(previousLabels)

	labels2 := (labelMap)(labels)

	setGoroutineLabels(&labels2)
	f()
}
