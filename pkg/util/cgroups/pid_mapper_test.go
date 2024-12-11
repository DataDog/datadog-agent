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
	"github.com/stretchr/testify/require"
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

// Notice that in this example the paths contain ":"
var cgroupV1ProcCgroupWithColons = `13:misc:/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
12:perf_event:/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
11:freezer:/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
10:pids:/kuberuntime.slice/containerd.service/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
9:cpuset:/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
8:memory:/kuberuntime.slice/containerd.service/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
7:rdma:/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
6:blkio:/kuberuntime.slice/containerd.service/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
5:devices:/kuberuntime.slice/containerd.service/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
4:net_cls,net_prio:/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
3:hugetlb:/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
2:cpu,cpuacct:/kuberuntime.slice/containerd.service/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
1:name=systemd:/kuberuntime.slice/containerd.service/kubepods-burstable-pod5be537b8_3a4e_4607_af71_31598b3a4fd3.slice:cri-containerd:1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120
0::/
`

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

var ecsFargateCgroup = `11:perf_event:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
10:hugetlb:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
9:memory:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
8:net_cls,net_prio:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
7:freezer:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
6:devices:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
5:cpu,cpuacct:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
4:blkio:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
3:pids:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
2:cpuset:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820
1:name=systemd:/ecs/8474ac4cec7a4f488834b00591271ec3/8474ac4cec7a4f488834b00591271ec3-3054012820`

var ecsFargateCgroupShort = `11:pids:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
10:blkio:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
9:cpu,cpuacct:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
8:perf_event:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
7:net_cls,net_prio:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
6:cpuset:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
5:devices:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
4:freezer:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
3:memory:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
2:hugetlb:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567
1:name=systemd:/ecs/0520ecd8e4194fd48309d1ae6eec92ec/0520ecd8e4194fd48309d1ae6eec92ec-946514567`

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

func TestIdentiferFromCgroupReferences(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		expectedID  string
	}{
		{
			name:        "cgroup v1",
			fileContent: cgroupV1ProcCgroup,
			expectedID:  "a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015f",
		},
		{
			name:        "cgroup v1 with colons",
			fileContent: cgroupV1ProcCgroupWithColons,
			expectedID:  "1c8f503430973935b7d8a80c4f58c0946b052a021e6855b358e5ec38601af120",
		},
		{
			name:        "docker in docker",
			fileContent: dindProcCgroup,
			expectedID:  "a51a9f7d073f848e7fc59e56e8f11524f330a2175a4ed26327da2dfe0d28015e",
		},
		{
			name:        "ecs fargate",
			fileContent: ecsFargateCgroup,
			expectedID:  "8474ac4cec7a4f488834b00591271ec3-3054012820",
		},
		{
			name:        "ecs fargate shorter",
			fileContent: ecsFargateCgroupShort,
			expectedID:  "0520ecd8e4194fd48309d1ae6eec92ec-946514567",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeFsPath := t.TempDir()
			procPath := filepath.Join(fakeFsPath, "proc")
			procPIDPath := filepath.Join(procPath, "123")
			require.NoErrorf(t, os.MkdirAll(procPIDPath, 0o750), "impossible to create temp directory '%s'", procPath)
			require.NoError(t, os.WriteFile(filepath.Join(procPIDPath, "cgroup"), []byte(test.fileContent), 0o640))

			id, err := IdentiferFromCgroupReferences(procPath, "123", defaultBaseController, ContainerFilter)
			require.NoError(t, err)
			assert.Equal(t, test.expectedID, id)
		})
	}
}
