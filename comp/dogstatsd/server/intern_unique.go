// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build go1.23

package server

import (
	"fmt"
	"unique"
)

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
//
// This implementation uses Go's unique.Handle for automatic string interning,
// which provides:
// - Automatic deduplication
// - Fast pointer-based comparisons
// - Automatic memory management by the garbage collector
// - No manual cache size management needed
type stringInterner struct {
	id        string
	telemetry *stringInternerInstanceTelemetry
}

func newStringInterner(_ int, internerID int, siTelemetry *stringInternerTelemetry) *stringInterner {
	id := fmt.Sprintf("interner_%d", internerID)
	i := &stringInterner{
		id:        id,
		telemetry: siTelemetry.PrepareForID(id),
	}

	return i
}

// LoadOrStore always returns the string from the cache, adding it into the
// cache if needed. With unique.Handle, the cache is managed automatically
// by the Go runtime and will be garbage collected when no longer referenced.
func (i *stringInterner) LoadOrStore(key []byte) string {
	// Create a handle for the string. unique.Make will automatically
	// deduplicate and intern the string.
	handle := unique.Make(string(key))

	// Return the canonical string value
	return handle.Value()
}
