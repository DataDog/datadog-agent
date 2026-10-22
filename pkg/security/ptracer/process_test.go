// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessCacheRemoveLeaderRemovesThreads(t *testing.T) {
	tc := NewProcessCache()

	// pids 0 and 1 are the indices of the thread slice below, and must survive
	tc.Add(1, NewProcess(1))

	leader := NewProcess(100)
	tc.Add(leader.Pid, leader)
	for _, pid := range []int{101, 102} {
		thread := NewProcess(pid)
		thread.Tgid = leader.Tgid
		tc.Add(pid, thread)
	}

	tc.Remove(leader)

	assert.Nil(t, tc.Get(100))
	assert.Nil(t, tc.Get(101))
	assert.Nil(t, tc.Get(102))
	assert.NotNil(t, tc.Get(1))
	assert.Empty(t, tc.tgid2Pid)
}
