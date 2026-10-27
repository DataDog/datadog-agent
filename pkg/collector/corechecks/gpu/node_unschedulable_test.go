// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nodeunschedulable"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// fakeUnschedulableSource stands in for the cluster agent.
type fakeUnschedulableSource struct {
	result nodeunschedulable.Result
	err    error
}

func (f *fakeUnschedulableSource) NodeUnschedulable() (nodeunschedulable.Result, error) {
	return f.result, f.err
}

func (f *fakeUnschedulableSource) Probe() error { return errors.New("probe not expected") }

func newNodeUnschedulableTestCheck(t *testing.T, src *fakeUnschedulableSource, opts ...testutil.NvmlMockOption) (*Check, *mocksender.MockSender, taggermock.Mock) {
	t.Helper()
	nvmltestutil.SetupMockNVML(t, append([]testutil.NvmlMockOption{testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2)}, opts...)...)
	env.SetFeatures(t, env.Kubernetes)
	configmock.New(t).SetInTest(nodeunschedulable.EnabledConfigKey, true)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmeta := testutil.GetWorkloadMetaMock(t)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	mockSender := mocksender.NewMockSender(t, "gpu")
	mockSender.SetupAcceptAll()

	check := newConfiguredGPUCheck(t, fakeTagger, wmeta, senderManager, map[int]string{})
	require.NoError(t, check.deviceCache.Refresh())
	require.NotNil(t, check.nodeUnschedulable)
	check.nodeUnschedulable = nodeunschedulable.NewCache(src, time.Minute)
	return check, mockSender, fakeTagger
}

// runNodeUnschedulable does what one check run does: refresh the node state, then run the collectors.
func runNodeUnschedulable(check *Check, snd *mocksender.MockSender, now time.Time) {
	snd.ResetCalls()
	check.nodeUnschedulable.Refresh(now)
	_ = check.emitMetrics(snd, nil, nil, now)
}

// unschedulableCalls counts the gpu.device.unavailable{unavailable_reason:unschedulable} series.
func unschedulableCalls(snd *mocksender.MockSender) int {
	n := 0
	for _, call := range snd.Calls {
		if call.Method == "GaugeWithTimestamp" && call.Arguments.String(0) == "gpu.device.unavailable" &&
			slices.Contains(call.Arguments.Get(3).([]string), "unavailable_reason:unschedulable") {
			n++
		}
	}
	return n
}

func toTimestamp(t time.Time) float64 { return float64(t.UnixNano()) / float64(time.Second) }

func TestNodeUnschedulableEmitsAndCloses(t *testing.T) {
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned", "example.com/drain:true"}}}
	check, mockSender, _ := newNodeUnschedulableTestCheck(t, src)
	tags := []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned", "unschedulable_reason:example.com/drain:true"}

	t0 := time.Now()
	runNodeUnschedulable(check, mockSender, t0)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", tags, toTimestamp(t0))
	require.Equal(t, 2, unschedulableCalls(mockSender)) // one series per device

	// The node is schedulable again: the same tagset gets a closing zero, nothing gets a 1.
	src.result = nodeunschedulable.Result{Reasons: []string{}}
	t1 := t0.Add(time.Minute)
	runNodeUnschedulable(check, mockSender, t1)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", tags, toTimestamp(t1))
	require.Equal(t, 2, unschedulableCalls(mockSender))

	// After the closing window nothing is emitted.
	runNodeUnschedulable(check, mockSender, t1.Add(nodeunschedulable.ClosingWindow))
	require.Zero(t, unschedulableCalls(mockSender))
}

// Only physical GPUs report the node state, MIG parents included; MIG slices do not.
func TestNodeUnschedulableSkipsMIGSlices(t *testing.T) {
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	check, mockSender, fakeTagger := newNodeUnschedulableTestCheck(t, src,
		testutil.WithDeviceOptions(0,
			testutil.WithMIGEnabled(),
			testutil.WithMIGChildUUIDs(map[int]string{0: testutil.MIGUUIDs[0]}),
		),
		testutil.WithMIGDeviceCountCallback(func(deviceIdx int) int {
			if deviceIdx == 0 {
				return 1
			}
			return 0
		}),
	)
	sliceTags := []string{
		"gpu_uuid:" + strings.ToLower(testutil.MIGUUIDs[0]),
		"gpu_slicing_mode:mig",
		"gpu_parent_uuid:" + strings.ToLower(testutil.GPUUUIDs[0]),
	}
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.MIGUUIDs[0]), "foo", sliceTags, nil, nil, nil)

	now := time.Now()
	runNodeUnschedulable(check, mockSender, now)
	require.Equal(t, 2, unschedulableCalls(mockSender)) // the two physical GPUs, MIG parent included
	mockSender.AssertNotCalled(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1.0, "",
		append(slices.Clone(sliceTags), "unavailable_reason:unschedulable", "unschedulable_reason:cordoned"), toTimestamp(now))
}

// A lost or unhealthy GPU reports 0 for the node state, so it is unavailable for one reason only.
func TestNodeUnschedulableYieldsToGPUHealth(t *testing.T) {
	lost := atomic.Bool{}
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	check, mockSender, fakeTagger := newNodeUnschedulableTestCheck(t, src,
		testutil.WithDeviceOptions(0, testutil.WithFieldValuesPartialOverride(map[uint32]testutil.MockFieldValue{
			nvml.FI_DEV_GET_GPU_RECOVERY_ACTION: testutil.NewFieldValue(uint64(nvml.GPU_RECOVERY_ACTION_GPU_RESET)),
		})),
		testutil.WithDeviceOptions(1, testutil.WithCustomHook(func(d *testutil.MockDevice) {
			getIndex := d.GetIndexFunc
			d.GetIndexFunc = func() (int, nvml.Return) {
				if lost.Load() {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return getIndex()
			}
		})),
	)
	tags := make([][]string, 2)
	for i := range tags {
		tags[i] = []string{"gpu_uuid:" + strings.ToLower(testutil.GPUUUIDs[i])}
		fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[i]), "foo", tags[i], nil, nil, nil)
	}
	unsched := func(i int) []string {
		return append(slices.Clone(tags[i]), "unavailable_reason:unschedulable", "unschedulable_reason:cordoned")
	}
	unhealthy := func(i int) []string { return append(slices.Clone(tags[i]), "unavailable_reason:unhealthy") }

	// GPU 0 needs a recovery action: unhealthy, not unschedulable. GPU 1 is only on a cordoned node.
	t0 := time.Now()
	runNodeUnschedulable(check, mockSender, t0)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", unhealthy(0), toTimestamp(t0))
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", unsched(0), toTimestamp(t0))
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", unhealthy(1), toTimestamp(t0))
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", unsched(1), toTimestamp(t0))

	// GPU 1 is lost: only lost is 1.
	lost.Store(true)
	t1 := t0.Add(time.Minute)
	runNodeUnschedulable(check, mockSender, t1)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", unhealthy(1), toTimestamp(t1))
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", unsched(1), toTimestamp(t1))
}

// No gpu.device.unavailable series has workload tags, even on a GPU used by a container.
func TestNodeUnschedulableSkipsWorkloadTags(t *testing.T) {
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	check, mockSender, fakeTagger := newNodeUnschedulableTestCheck(t, src)
	container := &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: "container0", Kind: workloadmeta.KindContainer}}
	check.wmeta.(workloadmetamock.Mock).Set(container)
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, container.EntityID.ID), "foo", []string{"container_id:container0"}, nil, nil, nil)
	gpuToContainers := map[string][]*workloadmeta.Container{testutil.GPUUUIDs[0]: {container}}

	hasContainerTag := func(reason string) bool {
		for _, call := range mockSender.Calls {
			tags, _ := call.Arguments.Get(3).([]string)
			if call.Method == "GaugeWithTimestamp" && call.Arguments.String(0) == "gpu.device.unavailable" &&
				slices.Contains(tags, "unavailable_reason:"+reason) && slices.Contains(tags, "container_id:container0") {
				return true
			}
		}
		return false
	}

	mockSender.ResetCalls()
	check.nodeUnschedulable.Refresh(time.Now())
	_ = check.emitMetrics(mockSender, gpuToContainers, nil, time.Now())
	require.Equal(t, 2, unschedulableCalls(mockSender))
	for _, reason := range []string{"lost", "unhealthy", "unschedulable"} {
		require.False(t, hasContainerTag(reason), reason)
	}
	mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.device.total", mock.Anything, "", mock.MatchedBy(func(tags []string) bool {
		return slices.Contains(tags, "container_id:container0")
	}), mock.Anything)
}

// A lost MIG parent reports lost, so it stays unavailable when its node state yields.
func TestNodeUnschedulableLostMIGParent(t *testing.T) {
	lost := atomic.Bool{}
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	check, mockSender, fakeTagger := newNodeUnschedulableTestCheck(t, src,
		testutil.WithDeviceOptions(0,
			testutil.WithMIGEnabled(),
			testutil.WithMIGChildUUIDs(map[int]string{0: testutil.MIGUUIDs[0]}),
			testutil.WithCustomHook(func(d *testutil.MockDevice) {
				getIndex := d.GetIndexFunc
				d.GetIndexFunc = func() (int, nvml.Return) {
					if lost.Load() {
						return 0, nvml.ERROR_GPU_IS_LOST
					}
					return getIndex()
				}
			}),
		),
		testutil.WithMIGDeviceCountCallback(func(deviceIdx int) int {
			if deviceIdx == 0 {
				return 1
			}
			return 0
		}),
	)
	parentTags := []string{"gpu_uuid:" + strings.ToLower(testutil.GPUUUIDs[0])}
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[0]), "foo", parentTags, nil, nil, nil)
	reason := func(tags ...string) []string { return append(slices.Clone(parentTags), tags...) }
	unsched := reason("unavailable_reason:unschedulable", "unschedulable_reason:cordoned")

	t0 := time.Now()
	runNodeUnschedulable(check, mockSender, t0)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", reason("unavailable_reason:lost"), toTimestamp(t0))
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", unsched, toTimestamp(t0))

	lost.Store(true)
	t1 := t0.Add(time.Minute)
	runNodeUnschedulable(check, mockSender, t1)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", reason("unavailable_reason:lost"), toTimestamp(t1))
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", reason("unavailable_reason:unhealthy"), toTimestamp(t1))
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", unsched, toTimestamp(t1))
}

// A failed fetch keeps the last result with no time limit: the series keeps its 1.
func TestNodeUnschedulableKeepsLastResultOnFailure(t *testing.T) {
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	check, mockSender, _ := newNodeUnschedulableTestCheck(t, src)
	tags := []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned"}

	t0 := time.Now()
	runNodeUnschedulable(check, mockSender, t0)

	src.err = errors.New("connection refused")
	t1 := t0.Add(6 * time.Hour)
	runNodeUnschedulable(check, mockSender, t1)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", tags, toTimestamp(t1))
}

func TestNodeUnschedulableNeverReadEmitsNothing(t *testing.T) {
	check, mockSender, _ := newNodeUnschedulableTestCheck(t, &fakeUnschedulableSource{err: errors.New("connection refused")})

	runNodeUnschedulable(check, mockSender, time.Now())
	require.Zero(t, unschedulableCalls(mockSender))
}

func TestNodeUnschedulableSupportedAnswerThatIsSchedulableEmitsNothing(t *testing.T) {
	check, mockSender, _ := newNodeUnschedulableTestCheck(t, &fakeUnschedulableSource{result: nodeunschedulable.Result{Reasons: []string{}}})

	runNodeUnschedulable(check, mockSender, time.Now())
	require.Zero(t, unschedulableCalls(mockSender))
}

func TestNodeUnschedulableNotOnKubernetes(t *testing.T) {
	check, mockSender, _ := newNodeUnschedulableTestCheck(t, &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}})
	env.SetFeatures(t)

	runNodeUnschedulable(check, mockSender, time.Now())
	require.Zero(t, unschedulableCalls(mockSender))
}

// The node-level tags are declared in the metric spec, so the spec validator accepts them.
func TestNodeUnschedulableTagsMatchSpec(t *testing.T) {
	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	expected, _, err := gpuspec.ExpectedTagsForMetric(specs.Tags, specs.Metrics.Metrics["device.unavailable"])
	require.NoError(t, err)
	tags := []string{
		"unavailable_reason:unschedulable",
		"unavailable_reason:unhealthy",
		"unschedulable_reason:not_ready",
		"unschedulable_reason:example.com/drain:true",
		"unschedulable_reason:nvidia.com/mig.config.state:failed",
		"unschedulable_reason:maintenance:true",
		"unschedulable_reason:cordoned",
	}
	for key, values := range gpuspec.TagsToKeyValues(tags) {
		tagSpec, found := expected[key]
		require.True(t, found, "tag %s is not in the spec", key)
		for _, value := range values {
			require.True(t, tagSpec.Regex == nil || tagSpec.Regex.MatchString(value), "%s:%s does not match the spec", key, value)
		}
	}
}

// Off by default: nothing is configured and nothing is read from the cluster agent.
func TestNodeUnschedulableEnabledByDefault(t *testing.T) {
	nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2))
	configmock.New(t)
	check := newConfiguredGPUCheck(t, taggerfxmock.SetupFakeTagger(t), testutil.GetWorkloadMetaMock(t), mocksender.CreateDefaultDemultiplexer(t), map[int]string{})
	require.NotNil(t, check.nodeUnschedulable)
}

func TestNodeUnschedulableCanBeDisabled(t *testing.T) {
	nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2))
	configmock.New(t).SetInTest(nodeunschedulable.EnabledConfigKey, false)
	check := newConfiguredGPUCheck(t, taggerfxmock.SetupFakeTagger(t), testutil.GetWorkloadMetaMock(t), mocksender.CreateDefaultDemultiplexer(t), map[int]string{})
	require.Nil(t, check.nodeUnschedulable)
}
