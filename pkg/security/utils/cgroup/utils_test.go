// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroup

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/stretchr/testify/assert"
)

func TestParseCgroupLine(t *testing.T) {
	line := `5:cpu,cpuacct:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope`
	id, ctrl, path, err := ParseCgroupLine(line)

	assert.NoError(t, err)
	assert.Equal(t, "5", id)
	assert.Equal(t, "cpu,cpuacct", ctrl)
	assert.Equal(t, "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod98005c3b_b650_4efe_8b91_2164d784397f.slice/cri-containerd-e8ac3efec3322d7f13cfa0cdee4344754d01bd4e50fea44e0753e83fdb74cab3.scope", path)
}

func TestFindContainerIDFromEntries(t *testing.T) {
	t.Run("from-cgroup-eks", func(t *testing.T) {
		cgroupContent := `11:memory:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
10:perf_event:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
9:pids:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
8:cpuset:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
7:freezer:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
6:hugetlb:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
5:devices:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
4:cpu,cpuacct:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
3:blkio:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
2:net_cls,net_prio:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
1:name=systemd:/ecs/409b8b89ccd746bdb9b5e03418406d96/409b8b89ccd746bdb9b5e03418406d96-3057940393/kubepods/besteffort/podc00eb3e2-d6c0-4eb6-9e58-fe539629263f/7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981
`
		cid := FindContainerIDFromEntries(ParseProcCgroupDataLenient([]byte(cgroupContent)))

		assert.Equal(t, containerutils.ContainerID("7022ec9d5774c69f38feddd6460373c4681ef72a4e03bc6f2d374387e9bde981"), cid)
	})

	t.Run("from-cgroup-ecs", func(t *testing.T) {
		cgroupContent := `11:devices:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
10:memory:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
9:blkio:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
1:name=systemd:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770`
		cid := FindContainerIDFromEntries(ParseProcCgroupDataLenient([]byte(cgroupContent)))

		assert.Equal(t, containerutils.ContainerID("8a28a84664034325be01ca46b33d1dd3-4092616770"), cid)
	})
}

func TestCGroupIDFromEntries(t *testing.T) {
	t.Run("from-cgroup-v1", func(t *testing.T) {
		cgroupContent := `11:memory:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
10:cpu,cpuacct:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770
1:name=systemd:/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770`

		cgroupID := CGroupIDFromEntries(ParseProcCgroupDataLenient([]byte(cgroupContent)))
		assert.Equal(t, containerutils.CGroupID("/ecs/8a28a84664034325be01ca46b33d1dd3/8a28a84664034325be01ca46b33d1dd3-4092616770"), cgroupID)
	})

	t.Run("from-cgroup-v2", func(t *testing.T) {
		cgroupContent := `0::/system.slice/containerd.service/kubepods.slice/pod123/container123`

		cgroupID := CGroupIDFromEntries(ParseProcCgroupDataLenient([]byte(cgroupContent)))
		assert.Equal(t, containerutils.CGroupID("/system.slice/containerd.service/kubepods.slice/pod123/container123"), cgroupID)
	})
}
