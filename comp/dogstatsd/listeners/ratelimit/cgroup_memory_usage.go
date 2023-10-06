// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !linux

package ratelimit

import (
	"errors"
)

var _ memoryUsage = (*hostMemoryUsage)(nil)

type cgroupMemoryUsage struct{}

func newCgroupMemoryUsage() (*cgroupMemoryUsage, error) {
	return nil, errors.New("not supported")
}

func (c *cgroupMemoryUsage) getMemoryStats() (float64, float64, error) {
	return 0, 0, errors.New("not supported")
}
