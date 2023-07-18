// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var sampleCgroupProcs = `1142219
1142238
1142208
1142129`

func TestCgroupProcsPidMapper(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")
	cfs.enableControllers(defaultBaseController)

	cgFooV1 := cfs.createCgroupV1("foov1", containerCgroupKubePod(false))
	cgFooV1.pidMapper = &cgroupProcsPidMapper{
		fr: cfs,
		cgroupProcsFilePathBuilder: func(relativeCgroupPath string) string {
			return filepath.Join(cfs.rootPath, defaultBaseController, relativeCgroupPath, cgroupProcsFile)
		},
	}
	cfs.setCgroupV1File(cgFooV1, defaultBaseController, cgroupProcsFile, sampleCgroupProcs)

	cgFooV2 := cfs.createCgroupV2("foov2", containerCgroupKubePod(false))
	cgFooV2.pidMapper = &cgroupProcsPidMapper{
		fr: cfs,
		cgroupProcsFilePathBuilder: func(relativeCgroupPath string) string {
			return filepath.Join(cfs.rootPath, "", relativeCgroupPath, cgroupProcsFile)
		},
	}
	cfs.setCgroupV2File(cgFooV2, cgroupProcsFile, sampleCgroupProcs)

	pids, err := cgFooV1.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{1142219, 1142238, 1142208, 1142129}, pids)

	pids, err = cgFooV2.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{1142219, 1142238, 1142208, 1142129}, pids)
}

var cgroupV1ProcCgroup = `12:rdma:/
11:memory:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
10:hugetlb:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
9:devices:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
8:perf_event:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
7:cpuset:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
6:net_cls,net_prio:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
5:cpu,cpuacct:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
4:blkio:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
3:freezer:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
2:pids:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
1:name=systemd:/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f
0::/system.slice/containerd.service`

var dindProcCgroup = `14:name=systemd:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
13:rdma:/
12:pids:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
11:hugetlb:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
10:net_prio:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
9:perf_event:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
8:net_cls:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
7:freezer:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
6:devices:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
5:memory:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
4:blkio:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
3:cpuacct:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
2:cpu:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
1:cpuset:/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e
0::/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/system.slice/containerd.service`

func TestProcPidMapperCgroupV1(t *testing.T) {
	fakeFsPath := t.TempDir()
	paths := []string{
		"proc/420",
		"proc/421",
		"proc/430",
		"proc/440",
	}

	for _, p := range paths {
		finalPath := filepath.Join(fakeFsPath, p)
		assert.NoErrorf(t, os.MkdirAll(finalPath, 0o750), "impossible to create temp directory '%s'", finalPath)
	}

	pidMapperV1 := &procPidMapper{
		procPath:         filepath.Join(fakeFsPath, "/proc"),
		cgroupController: defaultBaseController,
		readerFilter:     ContainerFilter,
	}

	cgCgroupV1 := cgroupV1{
		identifier: "a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f",
		path:       "kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f",
		pidMapper:  pidMapperV1,
	}
	assert.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "/proc/420/cgroup"), []byte(cgroupV1ProcCgroup), 0o640))
	assert.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "/proc/421/cgroup"), []byte(cgroupV1ProcCgroup), 0o640))

	pids, err := cgCgroupV1.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{420, 421}, pids)

	cgDindv1 := cgroupV1{
		identifier: "a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e",
		path:       "docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df/kubelet/kubepods/burstable/poda5ea884f-9e60-4912-bd62-fef9a31db47a/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e",
		pidMapper:  pidMapperV1,
	}
	assert.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "/proc/430/cgroup"), []byte(dindProcCgroup), 0o640))

	pids, err = cgDindv1.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{430}, pids)
}

var (
	cgroupV2ProcCgroup       = `0::/kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f`
	cgroupV2ProcCgroupNoHost = `0::../../../kubepods-burstable.slice/kubepods-burstable-pod562c01d6_6aba_49fe_ae52_61c40c04eca4.slice/docker-a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f.scope`
)

func TestProcPidMapperCgroupV2(t *testing.T) {
	fakeFsPath := t.TempDir()
	paths := []string{
		"proc/420",
		"proc/421",
		"proc/430",
	}

	for _, p := range paths {
		finalPath := filepath.Join(fakeFsPath, p)
		assert.NoErrorf(t, os.MkdirAll(finalPath, 0o750), "impossible to create temp directory '%s'", finalPath)
	}

	cgFooV2 := cgroupV2{
		identifier:   "a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f",
		relativePath: "kubepods/burstable/pod15513b48-e7a5-48fc-b9e3-92f713f36504/a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f",
		pidMapper: &procPidMapper{
			procPath:         filepath.Join(fakeFsPath, "/proc"),
			cgroupController: "",
			readerFilter:     ContainerFilter,
		},
	}
	assert.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "/proc/420/cgroup"), []byte(cgroupV2ProcCgroup), 0o640))
	assert.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "/proc/430/cgroup"), []byte(cgroupV2ProcCgroupNoHost), 0o640))

	pids, err := cgFooV2.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{420, 430}, pids)
}
