// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mallochook

// Stats contains statistics about allocations
type Stats struct {
	// Inuse is the number of bytes currently in use (allocated, but not freed)
	Inuse uint
	// Alloc is the total number of bytes allocated so far
	Alloc uint
}
