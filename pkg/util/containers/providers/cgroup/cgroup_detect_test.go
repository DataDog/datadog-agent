// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCgroupMountPoints(t *testing.T) {
	for _, tc := range []struct {
		contents []string
		expected map[string]string
	}{
		{
			contents: []string{
				"",
				"foo bar",
				"cgroup /sys/fs/cgroup/cpuset cgroup rw,relatime,cpuset 0 0",
				"cgroup /sys/fs/cgroup/cpu,cpuacct cgroup ro,nosuid,nodev,noexec,relatime,cpu,cpuacct 0 0",
				"cgroup /sys/fs/cgroup/devices cgroup rw,relatime,devices 0 0",
				"cgroup /sys/fs/cgroup/perf_event cgroup rw,relatime,perf_event 0 0",
				"cgroup /sys/fs/cgroup/hugetlb cgroup rw,relatime,hugetlb 0 0",
			},
			expected: map[string]string{
				"cpuset":     "/sys/fs/cgroup/cpuset",
				"cpu":        "/sys/fs/cgroup/cpu,cpuacct",
				"cpuacct":    "/sys/fs/cgroup/cpu,cpuacct",
				"devices":    "/sys/fs/cgroup/devices",
				"perf_event": "/sys/fs/cgroup/perf_event",
				"hugetlb":    "/sys/fs/cgroup/hugetlb",
			},
		},
		{
			contents: []string{
				"proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0 ",
				"tmpfs /dev tmpfs rw,nosuid,mode=755 0 0 ",
				"devpts /dev/pts devpts rw,nosuid,noexec,relatime,gid=5,mode=620,ptmxmode=666 0 0 ",
				"sysfs /sys sysfs ro,nosuid,nodev,noexec,relatime 0 0 ",
				"tmpfs /sys/fs/cgroup tmpfs ro,nosuid,nodev,noexec,relatime,mode=755 0 0 ",
				"openrc /sys/fs/cgroup/openrc cgroup ro,nosuid,nodev,noexec,relatime,release_agent=/lib/rc/sh/cgroup-release-agent.sh,name=openrc 0 0 ",
				"cpuset /sys/fs/cgroup/cpuset cgroup ro,nosuid,nodev,noexec,relatime,cpuset 0 0 ",
				"cpu /sys/fs/cgroup/cpu cgroup ro,nosuid,nodev,noexec,relatime,cpu 0 0 ",
				"cpuacct /sys/fs/cgroup/cpuacct cgroup ro,nosuid,nodev,noexec,relatime,cpuacct 0 0 ",
				"blkio /sys/fs/cgroup/blkio cgroup ro,nosuid,nodev,noexec,relatime,blkio 0 0 ",
				"memory /sys/fs/cgroup/memory cgroup ro,nosuid,nodev,noexec,relatime,memory 0 0 ",
				"devices /sys/fs/cgroup/devices cgroup ro,nosuid,nodev,noexec,relatime,devices 0 0 ",
				"freezer /sys/fs/cgroup/freezer cgroup ro,nosuid,nodev,noexec,relatime,freezer 0 0 ",
				"net_cls /sys/fs/cgroup/net_cls cgroup ro,nosuid,nodev,noexec,relatime,net_cls 0 0 ",
				"perf_event /sys/fs/cgroup/perf_event cgroup ro,nosuid,nodev,noexec,relatime,perf_event 0 0 ",
				"net_prio /sys/fs/cgroup/net_prio cgroup ro,nosuid,nodev,noexec,relatime,net_prio 0 0 ",
				"hugetlb /sys/fs/cgroup/hugetlb cgroup ro,nosuid,nodev,noexec,relatime,hugetlb 0 0 ",
				"pids /sys/fs/cgroup/pids cgroup ro,nosuid,nodev,noexec,relatime,pids 0 0 ",
				"cgroup /sys/fs/cgroup/systemd cgroup ro,nosuid,nodev,noexec,relatime,name=systemd 0 0 ",
				"mqueue /dev/mqueue mqueue rw,nosuid,nodev,noexec,relatime 0 0 ",
				"/dev/xvdb1 /conf.d ext4 rw,relatime,data=ordered 0 0 ",
				"/dev/xvdb1 /checks.d ext4 rw,relatime,data=ordered 0 0 ",
				"proc /host/proc proc ro,relatime 0 0 ",
				"xenfs /host/proc/xen xenfs rw,nosuid,nodev,noexec,relatime 0 0 ",
				"binfmt_misc /host/proc/sys/fs/binfmt_misc binfmt_misc rw,nosuid,nodev,noexec,relatime 0 0 ",
				"/dev/xvdb1 /etc/resolv.conf ext4 rw,relatime,data=ordered 0 0 ",
				"/dev/xvdb1 /etc/hostname ext4 rw,relatime,data=ordered 0 0 ",
				"/dev/xvdb1 /etc/hosts ext4 rw,relatime,data=ordered 0 0 ",
				"shm /dev/shm tmpfs rw,nosuid,nodev,noexec,relatime,size=65536k 0 0 ",
				"tmpfs /run/docker.sock tmpfs rw,nosuid,nodev,noexec,relatime,size=404072k,mode=755 0 0 ",
				"cgroup_root /sys/fs/cgroup tmpfs ro,relatime,size=10240k,mode=755 0 0 ",
				"openrc /sys/fs/cgroup/openrc cgroup rw,nosuid,nodev,noexec,relatime,release_agent=/lib/rc/sh/cgroup-release-agent.sh,name=openrc 0 0 ",
				"cpuset /sys/fs/cgroup/cpuset cgroup rw,nosuid,nodev,noexec,relatime,cpuset 0 0 ",
				"cpu /sys/fs/cgroup/cpu cgroup rw,nosuid,nodev,noexec,relatime,cpu 0 0 ",
				"cpuacct /sys/fs/cgroup/cpuacct cgroup rw,nosuid,nodev,noexec,relatime,cpuacct 0 0 ",
				"blkio /sys/fs/cgroup/blkio cgroup rw,nosuid,nodev,noexec,relatime,blkio 0 0 ",
				"memory /sys/fs/cgroup/memory cgroup rw,nosuid,nodev,noexec,relatime,memory 0 0 ",
				"devices /sys/fs/cgroup/devices cgroup rw,nosuid,nodev,noexec,relatime,devices 0 0 ",
				"freezer /sys/fs/cgroup/freezer cgroup rw,nosuid,nodev,noexec,relatime,freezer 0 0 ",
				"net_cls /sys/fs/cgroup/net_cls cgroup rw,nosuid,nodev,noexec,relatime,net_cls 0 0 ",
				"perf_event /sys/fs/cgroup/perf_event cgroup rw,nosuid,nodev,noexec,relatime,perf_event 0 0 ",
				"net_prio /sys/fs/cgroup/net_prio cgroup rw,nosuid,nodev,noexec,relatime,net_prio 0 0 ",
				"hugetlb /sys/fs/cgroup/hugetlb cgroup rw,nosuid,nodev,noexec,relatime,hugetlb 0 0 ",
				"pids /sys/fs/cgroup/pids cgroup rw,nosuid,nodev,noexec,relatime,pids 0 0 ",
				"cgroup /sys/fs/cgroup/systemd cgroup rw,relatime,name=systemd 0 0 ",
				"proc /proc/bus proc ro,relatime 0 0 ",
				"proc /proc/fs proc ro,relatime 0 0 ",
				"proc /proc/irq proc ro,relatime 0 0 ",
				"proc /proc/sys proc ro,relatime 0 0 ",
				"proc /proc/sysrq-trigger proc ro,relatime 0 0 ",
				"tmpfs /proc/kcore tmpfs rw,nosuid,mode=755 0 0 ",
				"tmpfs /proc/timer_list tmpfs rw,nosuid,mode=755 0 0 ",
				"tmpfs /proc/sched_debug tmpfs rw,nosuid,mode=755 0 0 ",
				"tmpfs /sys/firmware tmpfs ro,relatime 0 0",
			},
			expected: map[string]string{
				"cpuset":     "/sys/fs/cgroup/cpuset",
				"cpu":        "/sys/fs/cgroup/cpu",
				"cpuacct":    "/sys/fs/cgroup/cpuacct",
				"devices":    "/sys/fs/cgroup/devices",
				"perf_event": "/sys/fs/cgroup/perf_event",
				"hugetlb":    "/sys/fs/cgroup/hugetlb",
				"blkio":      "/sys/fs/cgroup/blkio",
				"freezer":    "/sys/fs/cgroup/freezer",
				"memory":     "/sys/fs/cgroup/memory",
				"net_cls":    "/sys/fs/cgroup/net_cls",
				"net_prio":   "/sys/fs/cgroup/net_prio",
				"openrc":     "/sys/fs/cgroup/openrc",
				"pids":       "/sys/fs/cgroup/pids",
				"systemd":    "/sys/fs/cgroup/systemd",
			},
		},
		{
			contents: []string{
				"",
				"",
				"",
			},
			expected: map[string]string{},
		},
	} {
		contents := strings.NewReader(strings.Join(tc.contents, "\n"))
		assert.Equal(t, tc.expected, parseCgroupMountPoints(contents))
	}
}

func TestParseCgroupPaths(t *testing.T) {
	for _, tc := range []struct {
		contents          []string
		expectedContainer string
		expectedPaths     map[string]string
	}{
		// test parsing of garden container cgroups in cloudfoundry
		{
			contents: []string{
				"11:net_cls:/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
				"9:cpu,cpuacct:/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
				"8:memory:/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
				"7:blkio:/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
			},
			expectedContainer: "bc3362fa-913c-4977-5812-d628",
			expectedPaths: map[string]string{
				"net_cls": "/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
				"cpu":     "/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
				"cpuacct": "/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
				"memory":  "/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
				"blkio":   "/sytem.slice/garden.service/bc3362fa-913c-4977-5812-d628",
			},
		},
		{
			contents: []string{
				"11:net_cls:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"9:cpu,cpuacct:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"8:memory:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"7:blkio:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
			},
			expectedContainer: "47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
			expectedPaths: map[string]string{
				"net_cls": "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"cpu":     "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"cpuacct": "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"memory":  "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"blkio":   "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
			},
		},
		{
			contents: []string{
				"",
				"11:net_cls:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"9:cpu,cpuacct:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
			},
			expectedContainer: "47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
			expectedPaths: map[string]string{
				"net_cls": "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"cpu":     "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
				"cpuacct": "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e",
			},
		},
		{
			contents: []string{
				"6:memory:/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"5:cpuacct:/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"3:cpuset:/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			},
			expectedContainer: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			expectedPaths: map[string]string{
				"memory":  "/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"cpuacct": "/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"cpuset":  "/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				// CPU is mising so we will automatically use from cpuacct
				"cpu": "/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			},
		},
		{
			// Melting pot of known cgroup formats
			contents: []string{
				// Kubernetes < 1.6
				"1:kube1.6:/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				// New CoreOS / most systems
				"2:classic:/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				// Rancher
				"3:rancher:/docker/864daa0a0b19aa4703231b6c76f85c6f369b2452a5a7f777f0c9101c0fd5772a/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				// Kubernetes 1.7+
				"4:kube1.7:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				// Legacy CoreOS 7xx
				"5:coreos_7xx:/system.slice/docker-a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope",
				// Legacy systems
				"6:legacy:a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope",
			},
			expectedContainer: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			expectedPaths: map[string]string{
				"kube1.6":    "/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"classic":    "/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"rancher":    "/docker/864daa0a0b19aa4703231b6c76f85c6f369b2452a5a7f777f0c9101c0fd5772a/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"kube1.7":    "/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
				"coreos_7xx": "/system.slice/docker-a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope",
				"legacy":     "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope",
			},
		},
		{
			// Make sure parsing works even if we have some invalid cases
			contents: []string{
				"12:rdma:/",
				"11:devices:/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
				"10:cpu,cpuacct:/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
				"9:freezer:/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
				"8:foo:/",
				"7:cpuset:/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
			},
			expectedContainer: "9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
			expectedPaths: map[string]string{
				"devices": "/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
				"cpu":     "/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
				"cpuacct": "/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
				"freezer": "/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
				"cpuset":  "/docker/9d76460be0e91bb0dd495e2ddcfa10db40c1ef0c1dbdb6a447a4974a9b58576e",
			},
		},
		{
			contents: []string{
				"14:name=systemd:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"13:pids:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"12:hugetlb:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"11:net_prio:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"10:perf_event:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"9:net_cls:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"8:freezer:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"7:devices:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"6:memory:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"5:blkio:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"4:cpuacct:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"3:cpu:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"2:cpuset:/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"1:name=openrc:/docker",
			},
			expectedContainer: "af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
			expectedPaths: map[string]string{
				"name=systemd": "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"pids":         "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"hugetlb":      "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"net_prio":     "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"perf_event":   "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"net_cls":      "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"freezer":      "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"devices":      "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"memory":       "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"blkio":        "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"cpuacct":      "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"cpu":          "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
				"cpuset":       "/docker/af1c1c0b02c6e45e0b6cb6151cd68fd02c7a6d91ad70d9bd72ccec8e83607841",
			},
		},
	} {
		contents := strings.NewReader(strings.Join(tc.contents, "\n"))
		c, p, err := parseCgroupPaths(contents, "")
		assert.NoError(t, err)
		assert.Equal(t, tc.expectedContainer, c)
		assert.Equal(t, tc.expectedPaths, p)
	}
}

func TestContainerIDFromCgroup(t *testing.T) {
	for _, tc := range []struct {
		path       string
		expectedID string
	}{
		{
			// Kubernetes < 1.6
			path:       "1:kube1.6:/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			expectedID: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
		},
		{
			// New CoreOS / most systems
			path:       "2:classic:/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			expectedID: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
		},
		{
			// Rancher
			path:       "3:rancher:/docker/864daa0a0b19aa4703231b6c76f85c6f369b2452a5a7f777f0c9101c0fd5772a/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			expectedID: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
		},
		{
			// Kubernetes 1.7+
			path:       "4:kube1.7:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
			expectedID: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
		},
		{
			// Legacy CoreOS 7xx
			path:       "5:coreos_7xx:/system.slice/docker-a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope",
			expectedID: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
		},
		{
			// Legacy systems
			path:       "6:legacy:a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope",
			expectedID: "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419",
		},
		{
			// Cloudfoundry
			path:       "7:cf:/system.slice/garden.service/bc3362fa-913c-4977-5812-d628",
			expectedID: "bc3362fa-913c-4977-5812-d628",
		},
	} {
		c, err := containerIDFromCgroup(tc.path, "")
		assert.True(t, err)
		assert.Equal(t, c, tc.expectedID)
	}
}

func TestCgroupPrefixFiltering(t *testing.T) {
	c, ok := containerIDFromCgroup("2:classic:/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419", "")
	assert.True(t, ok)
	assert.Equal(t, "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419", c)

	c, ok = containerIDFromCgroup("2:classic:/docker/a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419", "/docker/")
	assert.True(t, ok)
	assert.Equal(t, "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419", c)

	c, ok = containerIDFromCgroup("5:coreos_7xx:/system.slice/docker-a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope", "")
	assert.True(t, ok)
	assert.Equal(t, "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419", c)

	c, ok = containerIDFromCgroup("5:coreos_7xx:/system.slice/docker-a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419.scope", "/docker/")
	assert.False(t, ok)
	assert.Equal(t, "", c)
}

// TestDindContainer is to test if our agent can handle dind container correctly
func TestDindContainer(t *testing.T) {
	containerID := "6ab998413f7ae63bb26403dfe9e7ec02aa92b5cfc019de79da925594786c985f"
	tempFolder, cgroup, err := newDindContainerCgroup("dind-container", "memory", containerID)
	assert.NoError(t, err)
	tempFolder.add("memory.limit_in_bytes", "1234")
	defer tempFolder.removeAll()

	value, err := cgroup.MemLimit()
	assert.NoError(t, err)
	assert.Equal(t, value, uint64(1234))
}
