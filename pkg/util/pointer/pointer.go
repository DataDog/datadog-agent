// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pointer

// UInt64Ptr returns a pointer from a value. It will allocate a new heap object for it.
func UInt64Ptr(v uint64) *uint64 {
	return &v
}

// UInt32Ptr returns a pointer from a value. It will allocate a new heap object for it.
func UInt32Ptr(v int64) *uint32 {
	vUInt32 := uint32(v)
	return &vUInt32
}

// Float64Ptr returns a pointer from a value. It will allocate a new heap object for it.
func Float64Ptr(v float64) *float64 {
	return &v
}

// UIntToFloatPtr converts a uint64 value to float64 and returns a pointer.
func UIntToFloatPtr(u uint64) *float64 {
	f := float64(u)
	return &f
}

// BoolPtr returns a pointer from a value. It will allocate a new heap object for it.
func BoolPtr(b bool) *bool {
	return &b
}
