// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

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
			expectedContainer: "",
			expectedPaths:     nil,
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
	} {
		contents := strings.NewReader(strings.Join(tc.contents, "\n"))
		c, p, err := parseCgroupPaths(contents)
		assert.NoError(t, err)
		assert.Equal(t, c, tc.expectedContainer)
		assert.Equal(t, p, tc.expectedPaths)
	}
}
