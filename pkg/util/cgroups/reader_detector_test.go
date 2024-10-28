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

func TestDiscoverCgroupMountPoints(t *testing.T) {
	tests := []struct {
		name          string
		hostPrefix    string
		procFsPath    string
		expectedPaths map[string]string
	}{
		{
			name:       "cgroupv1 mounted in a container",
			hostPrefix: "/host",
			procFsPath: "./testdata/container-cgroupv1",
			expectedPaths: map[string]string{
				"blkio":      "/host/sys/fs/cgroup/blkio",
				"cpu":        "/host/sys/fs/cgroup/cpu,cpuacct",
				"cpuacct":    "/host/sys/fs/cgroup/cpu,cpuacct",
				"cpuset":     "/host/sys/fs/cgroup/cpuset",
				"devices":    "/host/sys/fs/cgroup/devices",
				"freezer":    "/host/sys/fs/cgroup/freezer",
				"hugetlb":    "/host/sys/fs/cgroup/hugetlb",
				"memory":     "/host/sys/fs/cgroup/memory",
				"net_cls":    "/host/sys/fs/cgroup/net_cls,net_prio",
				"net_prio":   "/host/sys/fs/cgroup/net_cls,net_prio",
				"perf_event": "/host/sys/fs/cgroup/perf_event",
				"pids":       "/host/sys/fs/cgroup/pids",
				"rdma":       "/host/sys/fs/cgroup/rdma",
				"systemd":    "/host/sys/fs/cgroup/systemd",
			},
		},
		{
			name:       "cgroupv2 mounted in a container",
			hostPrefix: "/host",
			procFsPath: "./testdata/container-cgroupv2",
			expectedPaths: map[string]string{
				"cgroupv2": "/host/sys/fs/cgroup",
			},
		},
		{
			name:       "dind cgroupv1",
			hostPrefix: "/host",
			procFsPath: "./testdata/dind-cgroupv1",
			expectedPaths: map[string]string{
				"blkio":      "/host/sys/fs/cgroup/blkio",
				"cpu":        "/host/sys/fs/cgroup/cpu",
				"cpuacct":    "/host/sys/fs/cgroup/cpuacct",
				"cpuset":     "/host/sys/fs/cgroup/cpuset",
				"devices":    "/host/sys/fs/cgroup/devices",
				"freezer":    "/host/sys/fs/cgroup/freezer",
				"hugetlb":    "/host/sys/fs/cgroup/hugetlb",
				"memory":     "/host/sys/fs/cgroup/memory",
				"net_cls":    "/host/sys/fs/cgroup/net_cls",
				"net_prio":   "/host/sys/fs/cgroup/net_prio",
				"perf_event": "/host/sys/fs/cgroup/perf_event",
				"pids":       "/host/sys/fs/cgroup/pids",
				"rdma":       "/host/sys/fs/cgroup/rdma",
				"systemd":    "/host/sys/fs/cgroup/systemd",
				// Not a real controller but mounted as such in DinD
				"kubelet": "/host/sys/fs/cgroup/cpu/kubelet",
				"88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df": "/host/sys/fs/cgroup/cpu/docker/88ea268ece65a02d68b169fd74bcbcb427eb7f28900db0e3b906fb2eeb7341df",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths, err := discoverCgroupMountPoints(tt.hostPrefix, tt.procFsPath)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPaths, paths)
		})
	}
}
