// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCPU(t *testing.T) {
	tempFolder, err := newTempFolder("cpu-stats")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cpuacctStats := dummyCgroupStat{
		"user":   64140,
		"system": 18327,
	}
	tempFolder.add("cpuacct/cpuacct.stat", cpuacctStats.String())
	tempFolder.add("cpuacct/cpuacct.usage", "915266418275")

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "cpuacct")

	timeStat, err := cgroup.CPU()
	assert.Nil(t, err)
	assert.Equal(t, timeStat.ContainerID, "dummy")
	assert.Equal(t, timeStat.User, uint64(64140))
	assert.Equal(t, timeStat.System, uint64(18327))
	assert.InDelta(t, timeStat.UsageTotal, 91526.6418275, 0.0000001)
}

func TestCPUNrThrottled(t *testing.T) {
	tempFolder, err := newTempFolder("cpu-throttled")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "cpu")

	// No file
	value, err := cgroup.CPUNrThrottled()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Invalid file
	tempFolder.add("cpu/cpu.stat", "200")
	_, err = cgroup.CPUNrThrottled()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Valid file
	cpuStats := dummyCgroupStat{
		"nr_periods":     0,
		"nr_throttled":   10,
		"throttled_time": 18327,
	}
	tempFolder.add("cpu/cpu.stat", cpuStats.String())
	value, err = cgroup.CPUNrThrottled()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(10))
}

func TestMemLimit(t *testing.T) {
	tempFolder, err := newTempFolder("mem-limit")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "memory")

	// No file
	value, err := cgroup.MemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Invalid file
	tempFolder.add("memory/memory.limit_in_bytes", "ab")
	value, err = cgroup.MemLimit()
	assert.NotNil(t, err)
	assert.IsType(t, err, &strconv.NumError{})
	assert.Equal(t, value, uint64(0))

	// Overflow value
	tempFolder.add("memory/memory.limit_in_bytes", strconv.Itoa(int(math.Pow(2, 61))))
	value, err = cgroup.MemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Valid value
	tempFolder.add("memory/memory.limit_in_bytes", "1234")
	value, err = cgroup.MemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(1234))
}

func TestParseSingleStat(t *testing.T) {
	tempFolder, err := newTempFolder("test-parse-single-stat")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "cpu")

	// No file
	_, err = cgroup.ParseSingleStat("cpu", "notfound")
	assert.NotNil(t, err)
	assert.True(t, os.IsNotExist(err))

	// Several lines
	tempFolder.add("cpu/cpu.test", "1234\nbla")
	_, err = cgroup.ParseSingleStat("cpu", "cpu.test")
	assert.NotNil(t, err)
	t.Log(err)
	assert.Contains(t, err.Error(), "wrong file format")

	// Not int
	tempFolder.add("cpu/cpu.test", "1234bla")
	_, err = cgroup.ParseSingleStat("cpu", "cpu.test")
	assert.NotNil(t, err)
	t.Log(err)
	assert.Equal(t, err.Error(), "strconv.ParseUint: parsing \"1234bla\": invalid syntax")

	// Valid file
	tempFolder.add("cpu/cpu.test", "1234")
	value, err := cgroup.ParseSingleStat("cpu", "cpu.test")
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(1234))
}

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
	} {
		contents := strings.NewReader(strings.Join(tc.contents, "\n"))
		c, p, err := parseCgroupPaths(contents)
		assert.NoError(t, err)
		assert.Equal(t, c, tc.expectedContainer)
		assert.Equal(t, p, tc.expectedPaths)
	}
}

func TestContainerIDFromCgroup(t *testing.T) {
	for _, tc := range []string{
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
	} {
		c, err := containerIDFromCgroup(tc)
		assert.True(t, err)
		assert.Equal(t, c, "a27f1331f6ddf72629811aac65207949fc858ea90100c438768b531a4c540419")
	}
}
