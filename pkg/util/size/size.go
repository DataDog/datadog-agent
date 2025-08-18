// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package size provides functions to compute the size of some complex types
package size

import (
	"unique"
	"unsafe"
)

// HasSizeInBytes is an interface that can be implemented by any object that has a size in bytes
type HasSizeInBytes interface {
	// SizeInBytes Return the size of the object in bytes (not including the size of its content)
	SizeInBytes() int

	// DataSizeInBytes Return the size of content of the object in bytes
	DataSizeInBytes() int
}

const (
	// stringSize is the size of a string structure in bytes.
	stringSize = unsafe.Sizeof(unique.Make(""))
	// stringSliceSize is the size of the string slice in bytes (not counting the size of the strings themselves).
	stringSliceSize = unsafe.Sizeof([]unique.Handle[string]{})
)

// SizeOfStringSlice returns the size of the string slice in bytes (not counting the size of the strings themselves).
//
//nolint:revive
func SizeOfStringSlice(s []unique.Handle[string]) int {
	return int(stringSliceSize) + len(s)*int(stringSize)
}

// DataSizeOfStringSlice returns the size of the content of the string slice in bytes.
func DataSizeOfStringSlice(v []unique.Handle[string]) int {
	size := 0
	for _, s := range v {
		size += len(s.Value())
	}
	return size
}
