package docker

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
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
	} {
		contents := strings.NewReader(strings.Join(tc.contents, "\n"))
		c, p, err := parseCgroupPaths(contents)
		assert.NoError(t, err)
		assert.Equal(t, c, tc.expectedContainer)
		assert.Equal(t, p, tc.expectedPaths)
	}
}
