// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

var _ memoryUsage = (*hostMemoryUsage)(nil)

type hostMemoryUsage struct{}

func newHostMemoryUsage() *hostMemoryUsage {
	panic("not called")
}

func (m *hostMemoryUsage) getMemoryStats() (float64, float64, error) {
	panic("not called")
}
