// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelfReader(t *testing.T) {
	mountPoints, err := discoverCgroupMountPoints("", "/proc")
	assert.NoError(t, err)

	if isCgroup1(mountPoints) {
		selfReaderCgroupV1(t)
	} else {
		selfReaderCgroupV2(t)
	}
}

func selfReaderCgroupV1(t *testing.T) {
	t.Helper()

	selfReader, err := NewSelfReader("./testdata/self-reader-cgroupv1", true)
	assert.NoError(t, err)

	cgroup := selfReader.GetCgroup(SelfCgroupIdentifier)
	assert.NotNil(t, cgroup)

	cgV1 := cgroup.(*cgroupV1)
	assert.Equal(t, ".", cgV1.path)
	assert.Equal(t, "/sys/fs/cgroup/memory", cgV1.mountPoints[defaultBaseController])
}

func selfReaderCgroupV2(t *testing.T) {
	t.Helper()

	selfReader, err := NewSelfReader("./testdata/self-reader-cgroupv2", true)
	assert.NoError(t, err)

	cgroup := selfReader.GetCgroup(SelfCgroupIdentifier)
	assert.NotNil(t, cgroup)

	cgV2 := cgroup.(*cgroupV2)
	assert.Equal(t, ".", cgV2.relativePath)
	assert.Equal(t, "/sys/fs/cgroup", cgV2.cgroupRoot)
}
