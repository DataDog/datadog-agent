// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utility related to the model
package utils

// TraceID is a 128-bit identifier for a trace.
type TraceID struct {
	Lo uint64
	Hi uint64
}
