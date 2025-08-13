// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file contains the unique.Handle-based string interner implementation.

package server

import (
	"fmt"
	"unique"
)

// uniqueStringInterner uses Go's unique.Handle for string interning.
// It provides automatic deduplication and memory management via garbage collection.
type uniqueStringInterner struct {
	id string
}

func newUniqueStringInterner(_ int, internerID int, _ *stringInternerTelemetry) *uniqueStringInterner {
	id := fmt.Sprintf("interner_%d", internerID)
	i := &uniqueStringInterner{
		id: id,
	}

	return i
}

// LoadOrStore always returns the interned string. With unique.Handle, the cache 
// is managed automatically by the Go runtime and will be garbage collected when 
// no longer referenced.
func (i *uniqueStringInterner) LoadOrStore(key []byte) string {
	// Create a handle for the string. unique.Make will automatically
	// deduplicate and intern the string.
	handle := unique.Make(string(key))

	// Return the canonical string value
	return handle.Value()
}

// cacheSize returns 0 since the actual cache size is managed by the Go runtime
// and is not accessible. This is primarily for testing purposes.
func (i *uniqueStringInterner) cacheSize() int {
	return 0 // Cannot track size with unique.Handle
}