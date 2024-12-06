// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils groups multiple utils function that can be used by the secl package
package utils

import (
	"context"
	"runtime/pprof"
)

// LabelSet represents an abstracted set of labels that can be used to call PprofDoWithoutContext
type LabelSet struct {
	innerCtx context.Context
}

// NewLabelSet returns a new LabelSet based on the labels provided as a pair number of arguments (key, value, key value ....)
func NewLabelSet(labels ...string) (LabelSet, error) {
	labelSet := pprof.Labels(labels...)
	innerCtx := pprof.WithLabels(context.Background(), labelSet)

	return LabelSet{
		innerCtx: innerCtx,
	}, nil
}

// PprofDoWithoutContext does the same thing as https://pkg.go.dev/runtime/pprof#Do, but without the allocation resulting
// from the usage of context values. This function also directly takes a map of labels, instead of incuring allocations
// when converting from one format (LabelSet) to the other (map).
func PprofDoWithoutContext(labelSet LabelSet, f func()) {
	defer pprof.SetGoroutineLabels(context.Background())
	pprof.SetGoroutineLabels(labelSet.innerCtx)
	f()
}
