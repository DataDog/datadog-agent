// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

// UInt64Ptr returns a pointer from a value
func UInt64Ptr(v uint64) *uint64 {
	return &v
}

// Float64Ptr returns a pointer from a value
func Float64Ptr(v float64) *float64 {
	return &v
}
