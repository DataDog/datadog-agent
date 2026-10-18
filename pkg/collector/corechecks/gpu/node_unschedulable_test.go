// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
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

func newNodeUnschedulableTestCheck(t *testing.T, src *fakeUnschedulableSource) (*Check, *mocksender.MockSender) {
	t.Helper()
	nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2))
	env.SetFeatures(t, env.Kubernetes)
	configmock.New(t).SetInTest(nodeUnschedulableEnabledKey, true)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmeta := testutil.GetWorkloadMetaMock(t)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	mockSender := mocksender.NewMockSender(t, "gpu")
	mockSender.SetupAcceptAll()

	check := newConfiguredGPUCheck(t, fakeTagger, wmeta, senderManager, map[int]string{})
	require.NoError(t, check.deviceCache.Refresh())
	require.NotNil(t, check.nodeUnschedulable)
	check.nodeUnschedulable.source = src
	return check, mockSender
}

func toTimestamp(t time.Time) float64 { return float64(t.UnixNano()) / float64(time.Second) }

func TestNodeUnschedulableEmitsAndCloses(t *testing.T) {
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned", "example.com/drain:true"}}}
	check, mockSender := newNodeUnschedulableTestCheck(t, src)
	tags := []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned", "unschedulable_reason:example.com/drain:true"}

	t0 := time.Now()
	check.emitNodeUnschedulable(mockSender, t0)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", tags, toTimestamp(t0))
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 2) // one series per device

	// The node is schedulable again: the same tagset gets a closing zero, nothing gets a 1.
	src.result = nodeunschedulable.Result{Reasons: []string{}}
	mockSender.ResetCalls()
	t1 := t0.Add(time.Minute)
	check.emitNodeUnschedulable(mockSender, t1)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 0, "", tags, toTimestamp(t1))
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 2)

	// After the closing window nothing is emitted.
	mockSender.ResetCalls()
	check.emitNodeUnschedulable(mockSender, t1.Add(nodeunschedulable.ClosingWindow))
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 0)
}

// A failed fetch keeps the last result with no time limit: the series keeps its 1.
func TestNodeUnschedulableKeepsLastResultOnFailure(t *testing.T) {
	src := &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	check, mockSender := newNodeUnschedulableTestCheck(t, src)
	tags := []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned"}

	t0 := time.Now()
	check.emitNodeUnschedulable(mockSender, t0)

	src.err = errors.New("connection refused")
	mockSender.ResetCalls()
	t1 := t0.Add(6 * time.Hour)
	check.emitNodeUnschedulable(mockSender, t1)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", 1, "", tags, toTimestamp(t1))
}

func TestNodeUnschedulableNeverReadEmitsNothing(t *testing.T) {
	check, mockSender := newNodeUnschedulableTestCheck(t, &fakeUnschedulableSource{err: errors.New("connection refused")})

	check.emitNodeUnschedulable(mockSender, time.Now())
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 0)
}

func TestNodeUnschedulableSupportedAnswerThatIsSchedulableEmitsNothing(t *testing.T) {
	check, mockSender := newNodeUnschedulableTestCheck(t, &fakeUnschedulableSource{result: nodeunschedulable.Result{Reasons: []string{}}})

	check.emitNodeUnschedulable(mockSender, time.Now())
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 0)
}

func TestNodeUnschedulableNotOnKubernetes(t *testing.T) {
	check, mockSender := newNodeUnschedulableTestCheck(t, &fakeUnschedulableSource{result: nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}})
	env.SetFeatures(t)

	check.emitNodeUnschedulable(mockSender, time.Now())
	mockSender.AssertNotCalled(t, "GaugeWithTimestamp", "gpu.device.unavailable", mock.Anything, "", mock.Anything, mock.Anything)
}

// The node-level tags are declared in the metric spec, so the spec validator accepts them.
func TestNodeUnschedulableTagsMatchSpec(t *testing.T) {
	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	expected, _, err := gpuspec.ExpectedTagsForMetric(specs.Tags, specs.Metrics.Metrics["device.unavailable"])
	require.NoError(t, err)
	tags := []string{
		"unavailable_reason:unschedulable",
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
func TestNodeUnschedulableDisabledByDefault(t *testing.T) {
	nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2))
	configmock.New(t)
	check := newConfiguredGPUCheck(t, taggerfxmock.SetupFakeTagger(t), testutil.GetWorkloadMetaMock(t), mocksender.CreateDefaultDemultiplexer(t), map[int]string{})
	require.Nil(t, check.nodeUnschedulable)
}

// Invalid rules are ignored: only a cordoned node is still reported.
func TestNodeUnschedulableInvalidRulesFallBackToCordon(t *testing.T) {
	nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2))
	cfg := configmock.New(t)
	cfg.SetInTest(nodeUnschedulableEnabledKey, true)
	cfg.SetInTest(nodeunschedulable.LabelsConfigKey, []map[string]interface{}{{"key": "example.com/drain"}})
	check := newConfiguredGPUCheck(t, taggerfxmock.SetupFakeTagger(t), testutil.GetWorkloadMetaMock(t), mocksender.CreateDefaultDemultiplexer(t), map[int]string{})
	require.NotNil(t, check.nodeUnschedulable)

	rules := check.nodeUnschedulable.source.(*dcaUnschedulableSource).rules
	require.False(t, rules.HasLabelRules())
	require.Equal(t, []string{"cordoned"}, rules.Evaluate(map[string]string{"example.com/drain": "true"}, nil, true))
}
