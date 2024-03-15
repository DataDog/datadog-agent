// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package system

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeCPUCount struct {
	count int
	err   error
}

func newFakeCPUCount(count int, err error) *fakeCPUCount {
	f := fakeCPUCount{count: count, err: err}
	cpuInfoFunc = f.info
	hostCPUCount.Store(0)
	hostCPUFailedAttempts = 0
	return &f
}

func (f *fakeCPUCount) info(context.Context, bool) (int, error) {
	return f.count, f.err
}

func TestHostCPUCount(t *testing.T) {
	defer hostCPUCount.Store(defaultCPUCountUnitTest)

	f := newFakeCPUCount(10000, nil)
	assert.Equal(t, f.count, HostCPUCount())

	f = newFakeCPUCount(10000, errors.New("Some error"))
	assert.Equal(t, runtime.NumCPU(), HostCPUCount())
	f.err = nil
	assert.Equal(t, f.count, HostCPUCount())

	// Test permafail
	f = newFakeCPUCount(10000, errors.New("Some error"))
	for i := 0; i < maxHostCPUFailedAttempts; i++ {
		assert.Equal(t, runtime.NumCPU(), HostCPUCount())
	}
	f.err = nil
	assert.Equal(t, runtime.NumCPU(), HostCPUCount())
}
