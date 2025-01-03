// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestCGroupvParseLine(t *testing.T) {
	line := `5:cpu,cpuacct:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope`
	id, ctrl, path, err := parseCgroupLine(line)

	assert.Nil(t, err)
	assert.Equal(t, "5", id)
	assert.Equal(t, "cpu,cpuacct", ctrl)
	assert.Equal(t, "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope", path)
}

func TestCGroupv1(t *testing.T) {
	data := `13:blkio:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
12:memory:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
11:misc:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
10:pids:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
9:hugetlb:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
8:rdma:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
7:perf_event:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
6:cpuset:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
5:cpu,cpuacct:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
4:net_cls,net_prio:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
3:freezer:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
2:devices:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
1:name=systemd:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope
0::/
`
	var (
		containerID   containerutils.ContainerID
		runtime       containerutils.CGroupFlags
		cgroupContext model.CGroupContext
	)

	err := parseProcControlGroupsData([]byte(data), func(id, ctrl, path string) bool {
		if path == "/" {
			return false
		}
		cgroup, err := makeControlGroup(id, ctrl, path)
		if err != nil {
			return false
		}

		containerID, runtime = cgroup.GetContainerContext()
		cgroupContext.CGroupID = containerutils.CGroupID(cgroup.Path)
		cgroupContext.CGroupFlags = runtime

		return true
	})

	assert.Nil(t, err)
	assert.NotEmpty(t, containerID)
	assert.NotZero(t, runtime)
	assert.Equal(t, containerID, containerutils.ContainerID("e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3"))
	assert.Equal(t, runtime, containerutils.CGroupFlags(containerutils.CGroupManagerCRI))
}

func TestCGroupv2(t *testing.T) {
	data := `0::/system.slice/docker-473a28bd49fcbf3a24eb55563125720311181ee184ae9b88fc9a3fbb30031e47.scope
`
	var (
		containerID   containerutils.ContainerID
		runtime       containerutils.CGroupFlags
		cgroupContext model.CGroupContext
	)

	err := parseProcControlGroupsData([]byte(data), func(id, ctrl, path string) bool {
		if path == "/" {
			return false
		}
		cgroup, err := makeControlGroup(id, ctrl, path)
		if err != nil {
			return false
		}

		containerID, runtime = cgroup.GetContainerContext()
		cgroupContext.CGroupID = containerutils.CGroupID(cgroup.Path)
		cgroupContext.CGroupFlags = runtime

		return true
	})

	assert.Nil(t, err)
	assert.NotEmpty(t, containerID)
	assert.NotZero(t, runtime)
	assert.Equal(t, containerID, containerutils.ContainerID("473a28bd49fcbf3a24eb55563125720311181ee184ae9b88fc9a3fbb30031e47"))
	assert.Equal(t, runtime, containerutils.CGroupFlags(containerutils.CGroupManagerDocker))
}
