// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"strconv"
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
	// IntSize is the size of an int in bytes.
	IntSize = strconv.IntSize / 8
	// StringSize is the size of a string structure in bytes.
	StringSize = unsafe.Sizeof("")
	// StringSliceSize is the size of the string slice in bytes (not counting the size of the strings themselves).
	StringSliceSize = unsafe.Sizeof([]string{})

	// BytesKindTelemetryKey is the tag key used to identify the kind of telemetry value.
	BytesKindTelemetryKey = "bytes_kind"
	// BytesKindStruct is the tag value used to mark bytes as struct.
	BytesKindStruct = "struct"
	// BytesKindData is the tag value used to mark bytes as data. Those are likely to be interned strings.
	BytesKindData = "data"
)

// SizeOfStringSlice returns the size of the string slice in bytes (not counting the size of the strings themselves).
func SizeOfStringSlice(s []string) int {
	return int(StringSliceSize) + len(s)*int(StringSize)
}

// DataSizeOfStringSlice returns the size of the content of the string slice in bytes.
func DataSizeOfStringSlice(v []string) int {
	size := 0
	for _, s := range v {
		size += len(s)
	}
	return size
}
