// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package containerd

import (
	"sort"
	"testing"
	"time"

	wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	v1 "github.com/containerd/cgroups/stats/v1"
	"github.com/containerd/containerd/containers"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	containersutil "github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

func int64Ptr(v int64) *int64 {
	return &v
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

// TestCollectTags checks the collectTags method
func TestCollectTags(t *testing.T) {
	tests := []struct {
		name      string
		labels    map[string]string
		imageName string
		runtime   string
		expected  []string
		err       error
	}{
		{
			"all functioning",
			map[string]string{"foo": "bar"},
			"redis",
			"containerd",
			[]string{"runtime:containerd", "image:redis", "foo:bar"},
			nil,
		}, {
			"missing labels",
			map[string]string{},
			"imagename",
			"containerd",
			[]string{"runtime:containerd", "image:imagename"},
			nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctn := containers.Container{
				Image:   test.imageName,
				Labels:  test.labels,
				Runtime: containers.RuntimeInfo{Name: test.runtime},
			}
			list, err := collectTags(ctn)
			if err != nil {
				require.Error(t, test.err, err)
			}
			sort.Strings(list)
			sort.Strings(test.expected)
			require.EqualValues(t, test.expected, list)
		})
	}
}

// TestComputeEvents checks the conversion of Containerd events to Datadog events
func TestComputeEvents(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	var err error
	defer containersutil.ResetSharedFilter()
	containerdCheck.filters, err = containersutil.GetSharedMetricFilter()
	require.NoError(t, err)

	tests := []struct {
		name          string
		events        []containerdEvent
		expectedTitle string
		expectedTags  []string
		numberEvents  int
	}{
		{
			name:          "No events",
			events:        []containerdEvent{},
			expectedTitle: "",
			numberEvents:  0,
		},
		{
			name: "Events on wrong type",
			events: []containerdEvent{
				{
					Topic: "/containers/delete/extra",
				}, {
					Topic: "containers/delete",
				},
			},
			expectedTitle: "",
			numberEvents:  0,
		},
		{
			name: "High cardinality Events with one invalid",
			events: []containerdEvent{
				{
					Topic:     "/containers/delete",
					Timestamp: time.Now(),
					Extra:     map[string]string{"foo": "bar"},
					Message:   "Container xxx deleted",
					ID:        "xxx",
				}, {
					Topic: "containers/delete",
				},
			},
			expectedTitle: "Event on containers from Containerd",
			expectedTags:  []string{"foo:bar"},
			numberEvents:  1,
		},
		{
			name: "Low cardinality Event",
			events: []containerdEvent{
				{
					Topic:     "/images/update",
					Timestamp: time.Now(),
					Extra:     map[string]string{"foo": "baz"},
					Message:   "Image yyy updated",
					ID:        "yyy",
				},
			},
			expectedTitle: "Event on images from Containerd",
			expectedTags:  []string{"foo:baz"},
			numberEvents:  1,
		},
		{
			name: "Filtered event",
			events: []containerdEvent{
				{
					Topic:     "/images/create",
					Timestamp: time.Now(),
					Extra:     map[string]string{},
					Message:   "Image kubernetes/pause created",
					ID:        "kubernetes/pause",
				},
			},
			expectedTitle: "Event on images from Containerd",
			expectedTags:  nil,
			numberEvents:  0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeEvents(test.events, mocked, containerdCheck.filters)
			mocked.On("Event", mock.AnythingOfType("metrics.Event"))
			if len(mocked.Calls) > 0 {
				res := (mocked.Calls[0].Arguments.Get(0)).(metrics.Event)
				assert.Contains(t, res.Title, test.expectedTitle)
				assert.ElementsMatch(t, res.Tags, test.expectedTags)
			}
			mocked.AssertNumberOfCalls(t, "Event", test.numberEvents)
			mocked.ResetCalls()
		})
	}
}

func TestComputeCPULinux(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	mocked.SetupAcceptAll()
	testTime := time.Now()

	tests := []struct {
		name        string
		cpu         *v1.CPUStat
		cpuLimit    *specs.LinuxCPU
		startTime   time.Time
		currentTime time.Time
		expected    map[string]float64
	}{
		{
			name: "CPU Usage, no limits, no throttling",
			cpu: &v1.CPUStat{
				Usage: &v1.CPUUsage{
					Kernel: 10,
					Total:  40,
					User:   30,
				},
			},
			startTime:   testTime.Add(-10 * time.Second),
			currentTime: testTime,
			expected: map[string]float64{
				"containerd.cpu.system": 10,
				"containerd.cpu.total":  40,
				"containerd.cpu.user":   30,
				"containerd.cpu.limit":  1e10 * float64(system.HostCPUCount()),
			},
		},
		{
			name: "CPU Usage, with limits, with throttling",
			cpu: &v1.CPUStat{
				Usage: &v1.CPUUsage{
					Kernel: 10,
					Total:  40,
					User:   30,
				},
				Throttling: &v1.Throttle{
					ThrottledPeriods: 1,
					ThrottledTime:    2,
				},
			},
			cpuLimit: &specs.LinuxCPU{
				Period: uint64Ptr(100),
				Quota:  int64Ptr(200),
			},
			startTime:   testTime.Add(-10 * time.Second),
			currentTime: testTime,
			expected: map[string]float64{
				"containerd.cpu.system":            10,
				"containerd.cpu.total":             40,
				"containerd.cpu.user":              30,
				"containerd.cpu.limit":             2e10,
				"containerd.cpu.throttled.periods": 1,
				"containerd.cpu.throttled.time":    2,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeCPULinux(mocked, test.cpu, test.cpuLimit, test.startTime, test.currentTime, []string{})
			for name, val := range test.expected {
				mocked.AssertMetric(t, "Rate", name, val, "", []string{})
			}
		})
	}
}

func TestComputeCPUWindows(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	mocked.SetupAcceptAll()

	tests := []struct {
		name                    string
		cpuStats                *wstats.WindowsContainerProcessorStatistics
		tags                    []string
		expectedMetricsReported map[string]float64
	}{
		{
			name: "CPU stats not nil",
			cpuStats: &wstats.WindowsContainerProcessorStatistics{
				TotalRuntimeNS:  50,
				RuntimeUserNS:   10,
				RuntimeKernelNS: 40,
			},
			tags: []string{"foo:bar"},
			expectedMetricsReported: map[string]float64{
				"containerd.cpu.total":  50,
				"containerd.cpu.user":   10,
				"containerd.cpu.system": 40,
			},
		},
		{
			name:                    "CPU stats is nil",
			cpuStats:                nil,
			tags:                    []string{"foo:bar"},
			expectedMetricsReported: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeCPUWindows(mocked, test.cpuStats, test.tags)

			if test.cpuStats == nil {
				mocked.AssertNotCalled(t, "Rate")
			} else {
				for metricName, metricVal := range test.expectedMetricsReported {
					mocked.AssertMetric(t, "Rate", metricName, metricVal, "", test.tags)
				}
			}
		})
	}
}

func TestComputeMemLinux(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	mocked.SetupAcceptAll()

	tests := []struct {
		name     string
		mem      *v1.MemoryStat
		expected map[string]float64
	}{
		{
			name:     "call with empty mem",
			mem:      nil,
			expected: map[string]float64{},
		},
		{
			name:     "nothing",
			mem:      &v1.MemoryStat{},
			expected: map[string]float64{},
		},
		{
			name: "missing one of the MemoryEntries, missing entries in the others",
			mem: &v1.MemoryStat{
				Usage: &v1.MemoryEntry{
					Usage: 1,
				},
				Kernel: &v1.MemoryEntry{
					Max: 2,
				},
				Swap: &v1.MemoryEntry{
					Limit: 3,
				},
			},
			expected: map[string]float64{
				"containerd.mem.current.usage": 1,
				"containerd.mem.kernel.max":    2,
				"containerd.mem.swap.limit":    3,
			},
		},
		{
			name: "full MemoryEntries, some regular metrics",
			mem: &v1.MemoryStat{
				Usage: &v1.MemoryEntry{
					Usage:   1,
					Max:     2,
					Limit:   3,
					Failcnt: 0,
				},
				Kernel: &v1.MemoryEntry{
					Usage:   1,
					Max:     2,
					Limit:   3,
					Failcnt: 0,
				},
				Swap: &v1.MemoryEntry{
					Usage:   1,
					Max:     2,
					Limit:   3,
					Failcnt: 0,
				},
				Cache:        20,
				RSSHuge:      1212,
				InactiveAnon: 1234,
			},
			expected: map[string]float64{
				"containerd.mem.current.usage":   1,
				"containerd.mem.current.max":     2,
				"containerd.mem.current.limit":   3,
				"containerd.mem.current.failcnt": 0,
				"containerd.mem.kernel.max":      2,
				"containerd.mem.kernel.usage":    1,
				"containerd.mem.kernel.limit":    3,
				"containerd.mem.kernel.failcnt":  0,
				"containerd.mem.swap.limit":      3,
				"containerd.mem.swap.max":        2,
				"containerd.mem.swap.usage":      1,
				"containerd.mem.swap.failcnt":    0,
				"containerd.mem.cache":           20,
				"containerd.mem.rsshuge":         1212,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeMemLinux(mocked, test.mem, []string{})
			for name, val := range test.expected {
				mocked.AssertMetric(t, "Gauge", name, val, "", []string{})
			}
		})
	}
}

func TestComputeMemWindows(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	mocked.SetupAcceptAll()

	tests := []struct {
		name                    string
		memStats                *wstats.WindowsContainerMemoryStatistics
		tags                    []string
		expectedMetricsReported map[string]float64
	}{
		{
			name: "Memory stats not nil",
			memStats: &wstats.WindowsContainerMemoryStatistics{
				MemoryUsageCommitBytes:            1024,
				MemoryUsageCommitPeakBytes:        2048,
				MemoryUsagePrivateWorkingSetBytes: 512,
			},
			tags: []string{"foo:bar"},
			expectedMetricsReported: map[string]float64{
				"containerd.mem.commit":              1024,
				"containerd.mem.commit_peak":         2048,
				"containerd.mem.private_working_set": 512,
			},
		},
		{
			name:                    "Memory stats is nil",
			memStats:                nil,
			tags:                    []string{"foo:bar"},
			expectedMetricsReported: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeMemWindows(mocked, test.memStats, test.tags)

			if test.memStats == nil {
				mocked.AssertNotCalled(t, "Gauge")
			} else {
				for metricName, metricVal := range test.expectedMetricsReported {
					mocked.AssertMetric(t, "Gauge", metricName, metricVal, "", test.tags)
				}
			}
		})
	}
}

func TestComputeStorageWindows(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	mocked.SetupAcceptAll()

	tests := []struct {
		name                    string
		storageStats            *wstats.WindowsContainerStorageStatistics
		tags                    []string
		expectedMetricsReported map[string]float64
	}{
		{
			name: "Storage stats not nil",
			storageStats: &wstats.WindowsContainerStorageStatistics{
				ReadSizeBytes:  256,
				WriteSizeBytes: 128,
			},
			tags: []string{"foo:bar"},
			expectedMetricsReported: map[string]float64{
				"containerd.storage.read":  256,
				"containerd.storage.write": 128,
			},
		},
		{
			name:                    "Storage stats is nil",
			storageStats:            nil,
			tags:                    []string{"foo:bar"},
			expectedMetricsReported: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeStorageWindows(mocked, test.storageStats, test.tags)

			if test.storageStats == nil {
				mocked.AssertNotCalled(t, "Rate")
			} else {
				for metricName, metricVal := range test.expectedMetricsReported {
					mocked.AssertMetric(t, "Rate", metricName, metricVal, "", test.tags)
				}
			}
		})
	}
}

func TestComputeUptime(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	mocked.SetupAcceptAll()

	currentTime := time.Now()

	tests := []struct {
		name     string
		ctn      containers.Container
		expected map[string]float64
	}{
		{
			name: "Normal check",
			ctn: containers.Container{
				CreatedAt: currentTime.Add(-60 * time.Second),
			},
			expected: map[string]float64{
				"containerd.uptime": 60.0,
			},
		},
		{
			name: "Created in the future",
			ctn: containers.Container{
				CreatedAt: currentTime.Add(60 * time.Second),
			},
			expected: map[string]float64{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeUptime(mocked, test.ctn, currentTime, []string{})
			for name, val := range test.expected {
				mocked.AssertMetric(t, "Gauge", name, val, "", []string{})
			}
		})
	}
}

// TestisExcluded tests the filtering of containers in the compute metrics method
func TestIsExcluded(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	var err error
	// GetShareFilter gives us the OOB exclusion of pause container images from most supported platforms
	config.Datadog.Set("container_exclude", "kube_namespace:shouldexclude")
	defer config.Datadog.SetDefault("container_exclude", "")
	defer containersutil.ResetSharedFilter()
	containerdCheck.filters, err = containersutil.GetSharedMetricFilter()
	require.NoError(t, err)
	c := containers.Container{
		Image: "kubernetes/pause",
	}
	// kubernetes/pause is excluded
	isEc := isExcluded(c, containerdCheck.filters)
	require.True(t, isEc)

	c = containers.Container{
		Image: "kubernetes/pawz",
	}
	// kubernetes/pawz although not an available image (yet ?) is not ignored
	isEc = isExcluded(c, containerdCheck.filters)
	require.False(t, isEc)

	// Namespace based filtering
	c = containers.Container{
		Image: "kubernetes/pawz",
		Labels: map[string]string{
			"io.kubernetes.pod.namespace": "shouldexclude",
		},
	}
	require.True(t, isExcluded(c, containerdCheck.filters))

	// Pause container filtering
	c = containers.Container{
		Image: "foo",
		Labels: map[string]string{
			"io.kubernetes.pod.name": "foo",
		},
	}
	require.True(t, isExcluded(c, containerdCheck.filters))
}
