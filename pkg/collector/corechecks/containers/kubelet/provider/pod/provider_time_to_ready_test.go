// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package pod

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
)

var (
	baseTime      = time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	scheduledTime = baseTime.Add(1 * time.Second)
	runningTime   = baseTime.Add(11 * time.Second)
	readyTime     = baseTime.Add(15 * time.Second)
)

func newTestPod(id string) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   id,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "test-pod-" + id,
			Namespace: "default",
		},
		Phase:             podPhaseRunning,
		Ready:             true,
		CreationTimestamp: baseTime,
		Conditions: []workloadmeta.KubernetesPodCondition{
			{Type: podConditionTypePodScheduled, Status: "True", LastTransitionTime: scheduledTime},
			{Type: podConditionTypeReady, Status: "True", LastTransitionTime: readyTime},
		},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				Name:         "app",
				Ready:        true,
				RestartCount: 0,
				State: workloadmeta.KubernetesContainerState{
					Running: &workloadmeta.KubernetesContainerStateRunning{
						StartedAt: runningTime,
					},
				},
			},
		},
	}
}

// TestComputePodStartupTimings tests the pure timing computation.
func TestComputePodStartupTimings(t *testing.T) {
	t.Run("normal pod", func(t *testing.T) {
		timings, err := computePodStartupTimings(newTestPod("normal"), false)
		assert.NoError(t, err)
		assert.Equal(t, 14*time.Second, timings.timeToReady)
		assert.Equal(t, 10*time.Second, timings.timeToRunning)
	})

	t.Run("no PodScheduled condition returns error", func(t *testing.T) {
		p := newTestPod("no-scheduled")
		p.Conditions = []workloadmeta.KubernetesPodCondition{
			{Type: podConditionTypeReady, Status: "True", LastTransitionTime: readyTime},
		}
		_, err := computePodStartupTimings(p, false)
		assert.Error(t, err)
	})

	t.Run("no Ready condition emits only time_to_running", func(t *testing.T) {
		p := newTestPod("no-ready")
		p.Conditions = []workloadmeta.KubernetesPodCondition{
			{Type: podConditionTypePodScheduled, Status: "True", LastTransitionTime: scheduledTime},
		}
		timings, err := computePodStartupTimings(p, false)
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(0), timings.timeToReady)
		assert.Equal(t, 10*time.Second, timings.timeToRunning)
	})

	t.Run("no running containers emits only time_to_ready", func(t *testing.T) {
		p := newTestPod("no-running")
		p.ContainerStatuses = []workloadmeta.KubernetesContainerStatus{
			{Name: "app", State: workloadmeta.KubernetesContainerState{}},
		}
		timings, err := computePodStartupTimings(p, false)
		assert.NoError(t, err)
		assert.Equal(t, 14*time.Second, timings.timeToReady)
		assert.Equal(t, time.Duration(0), timings.timeToRunning)
	})

	t.Run("multiple containers picks earliest", func(t *testing.T) {
		p := newTestPod("multi")
		earlyRunning := baseTime.Add(8 * time.Second)
		p.ContainerStatuses = []workloadmeta.KubernetesContainerStatus{
			{
				Name: "sidecar",
				State: workloadmeta.KubernetesContainerState{
					Running: &workloadmeta.KubernetesContainerStateRunning{StartedAt: runningTime},
				},
			},
			{
				Name: "app",
				State: workloadmeta.KubernetesContainerState{
					Running: &workloadmeta.KubernetesContainerStateRunning{StartedAt: earlyRunning},
				},
			},
		}
		timings, err := computePodStartupTimings(p, false)
		assert.NoError(t, err)
		assert.Equal(t, 14*time.Second, timings.timeToReady)
		assert.Equal(t, 7*time.Second, timings.timeToRunning)
	})

	t.Run("init container statuses are ignored for time_to_running", func(t *testing.T) {
		p := newTestPod("with-init")
		p.InitContainerStatuses = []workloadmeta.KubernetesContainerStatus{
			{
				Name: "init",
				State: workloadmeta.KubernetesContainerState{
					Running: &workloadmeta.KubernetesContainerStateRunning{StartedAt: baseTime.Add(3 * time.Second)},
				},
			},
		}
		timings, err := computePodStartupTimings(p, false)
		assert.NoError(t, err)
		assert.Equal(t, 14*time.Second, timings.timeToReady)
		assert.Equal(t, 10*time.Second, timings.timeToRunning)
	})

	t.Run("container restart returns error", func(t *testing.T) {
		p := newTestPod("restarted")
		p.ContainerStatuses[0].RestartCount = 1
		_, err := computePodStartupTimings(p, false)
		assert.Error(t, err)
	})

	t.Run("init container restart returns error", func(t *testing.T) {
		p := newTestPod("init-restarted")
		p.InitContainerStatuses = []workloadmeta.KubernetesContainerStatus{
			{Name: "init", RestartCount: 2},
		}
		_, err := computePodStartupTimings(p, false)
		assert.Error(t, err)
	})

	t.Run("readiness failure skips time_to_ready but keeps time_to_running", func(t *testing.T) {
		timings, err := computePodStartupTimings(newTestPod("flaky"), true)
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(0), timings.timeToReady)
		assert.Equal(t, 10*time.Second, timings.timeToRunning)
	})

	t.Run("durations exceeding max are discarded", func(t *testing.T) {
		p := newTestPod("exceed")
		p.Conditions[1].LastTransitionTime = scheduledTime.Add(2 * time.Hour)
		p.ContainerStatuses[0].State.Running.StartedAt = scheduledTime.Add(2 * time.Hour)
		_, err := computePodStartupTimings(p, false)
		assert.Error(t, err)
	})

	t.Run("negative durations are discarded", func(t *testing.T) {
		p := newTestPod("negative")
		// Ready before scheduled
		p.Conditions[1].LastTransitionTime = scheduledTime.Add(-5 * time.Second)
		// Container started before scheduled
		p.ContainerStatuses[0].State.Running.StartedAt = scheduledTime.Add(-3 * time.Second)
		_, err := computePodStartupTimings(p, false)
		assert.Error(t, err)
	})
}

// TestAnyContainerRestarted tests the restart check heuristic.
func TestAnyContainerRestarted(t *testing.T) {
	tests := []struct {
		name string
		pod  *workloadmeta.KubernetesPod
		want bool
	}{
		{
			name: "no restarts",
			pod:  newTestPod("no-restarts"),
			want: false,
		},
		{
			name: "regular container restarted",
			pod: func() *workloadmeta.KubernetesPod {
				p := newTestPod("restarted")
				p.ContainerStatuses[0].RestartCount = 1
				return p
			}(),
			want: true,
		},
		{
			name: "init container restarted",
			pod: func() *workloadmeta.KubernetesPod {
				p := newTestPod("init-restarted")
				p.InitContainerStatuses = []workloadmeta.KubernetesContainerStatus{
					{Name: "init", RestartCount: 2},
				}
				return p
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, anyContainerRestarted(tt.pod))
		})
	}
}

func newTestProvider(t *testing.T) (*Provider, *mocksender.MockSender) {
	t.Helper()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Register tags for test pod UIDs at orchestrator cardinality
	for _, uid := range []string{"normal", "restarted", "flaky", "not-ready", "exceed-max"} {
		entityID := taggertypes.NewEntityID(taggertypes.KubernetesPodUID, uid)
		fakeTagger.SetTags(entityID, "test",
			[]string{"kube_namespace:default", "kube_deployment:test-deployment"},
			[]string{"pod_name:test-pod-" + uid},
			nil, nil,
		)
	}

	config := &common.KubeletConfig{
		OpenmetricsInstance: types.OpenmetricsInstance{
			Tags:    []string{"cluster:test"},
			Timeout: 10,
		},
	}

	mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
	mockSender.SetupAcceptAll()

	provider := &Provider{
		config: config,
		tagger: fakeTagger,
		now:    time.Now,
	}

	return provider, mockSender
}

// TestGeneratePodStartupMetrics tests the full metric emission with heuristics.
func TestGeneratePodStartupMetrics(t *testing.T) {
	t.Run("emits both metrics for healthy pod", func(t *testing.T) {
		provider, mockSender := newTestProvider(t)
		pod := newTestPod("normal")

		provider.generatePodStartupMetrics(mockSender, pod, nil)

		mockSender.AssertMetric(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_ready", 14.0, "", []string{"kube_namespace:default", "kube_deployment:test-deployment", "pod_name:test-pod-normal", "cluster:test"})
		mockSender.AssertMetric(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_running", 10.0, "", []string{"kube_namespace:default", "kube_deployment:test-deployment", "pod_name:test-pod-normal", "cluster:test"})
	})

	t.Run("skips pod with container restarts", func(t *testing.T) {
		provider, mockSender := newTestProvider(t)
		pod := newTestPod("restarted")
		pod.ContainerStatuses[0].RestartCount = 1

		provider.generatePodStartupMetrics(mockSender, pod, nil)

		mockSender.AssertNotCalled(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_ready")
		mockSender.AssertNotCalled(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_running")
	})

	t.Run("skips time_to_ready but emits time_to_running when readiness probes failed", func(t *testing.T) {
		provider, mockSender := newTestProvider(t)
		pod := newTestPod("flaky")

		failures := readinessFailureCounts{
			"flaky": {"app": 5},
		}

		provider.generatePodStartupMetrics(mockSender, pod, failures)

		mockSender.AssertMetricNotTaggedWith(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_ready", []string{"pod_name:test-pod-flaky"})
		mockSender.AssertMetric(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_running", 10.0, "", []string{"kube_namespace:default", "kube_deployment:test-deployment", "pod_name:test-pod-flaky", "cluster:test"})
	})

	t.Run("skips pod that is not ready", func(t *testing.T) {
		provider, mockSender := newTestProvider(t)
		pod := newTestPod("not-ready")
		pod.Ready = false

		provider.generatePodStartupMetrics(mockSender, pod, nil)

		mockSender.AssertNotCalled(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_ready")
		mockSender.AssertNotCalled(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_running")
	})

	t.Run("skips metrics exceeding max duration", func(t *testing.T) {
		provider, mockSender := newTestProvider(t)
		pod := newTestPod("exceed-max")
		// Set ready time to 2 hours after scheduled
		pod.Conditions[1].LastTransitionTime = scheduledTime.Add(2 * time.Hour)
		// Set running time to 2 hours after scheduled
		pod.ContainerStatuses[0].State.Running.StartedAt = scheduledTime.Add(2 * time.Hour)

		provider.generatePodStartupMetrics(mockSender, pod, nil)

		mockSender.AssertNotCalled(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_ready")
		mockSender.AssertNotCalled(t, "Gauge", common.KubeletMetricsPrefix+"pod.scheduled_time_to_running")
	})
}

// TestPodHasTooManyReadinessFailures tests the per-container failure threshold logic.
func TestPodHasTooManyReadinessFailures(t *testing.T) {
	t.Run("no failures returns false", func(t *testing.T) {
		pod := newTestPod("clean")
		assert.False(t, podHasTooManyReadinessFailures(pod, nil, nil))
	})

	t.Run("failures below default threshold returns false", func(t *testing.T) {
		pod := newTestPod("below")
		failures := readinessFailureCounts{
			"below": {"app": 2},
		}
		assert.False(t, podHasTooManyReadinessFailures(pod, failures, nil))
	})

	t.Run("failures at default threshold returns true", func(t *testing.T) {
		pod := newTestPod("at")
		failures := readinessFailureCounts{
			"at": {"app": 3},
		}
		assert.True(t, podHasTooManyReadinessFailures(pod, failures, nil))
	})

	t.Run("failures above default threshold returns true", func(t *testing.T) {
		pod := newTestPod("above")
		failures := readinessFailureCounts{
			"above": {"app": 4},
		}
		assert.True(t, podHasTooManyReadinessFailures(pod, failures, nil))
	})

	t.Run("pod not in failures map returns false", func(t *testing.T) {
		pod := newTestPod("missing")
		failures := readinessFailureCounts{
			"other-pod": {"app": 100},
		}
		assert.False(t, podHasTooManyReadinessFailures(pod, failures, nil))
	})

	t.Run("nil failures map returns false", func(t *testing.T) {
		pod := newTestPod("nil-map")
		assert.False(t, podHasTooManyReadinessFailures(pod, nil, nil))
	})
}
