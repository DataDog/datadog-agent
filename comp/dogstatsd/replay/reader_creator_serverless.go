// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build serverless

package replay

// NewTrafficCaptureReader creates a TrafficCaptureReader instance
func NewTrafficCaptureReader(path string, depth int, mmap bool) (*TrafficCaptureReader, error) {
	panic("not implemented")
}
