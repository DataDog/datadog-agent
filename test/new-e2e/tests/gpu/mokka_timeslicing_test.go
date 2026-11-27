// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/mokka"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// timeSlicingReplicas is the number of replicas the device plugin splits the simulated GPU into.
const timeSlicingReplicas = 4

// timeSlicingDevicePluginConfig shares each GPU between timeSlicingReplicas workloads.
var timeSlicingDevicePluginConfig = fmt.Sprintf(`version: v1
sharing:
  timeSlicing:
    resources:
    - name: nvidia.com/gpu
      replicas: %d
`, timeSlicingReplicas)

// gpuMokkaTimeSlicingK8sSuite runs the GPU sharing tests on a CPU-only kind cluster with one GPU
// simulated by Mokka, shared with time-slicing by the device plugin of the NVIDIA GPU Operator,
// whose GPU Feature Discovery labels the node with the sharing strategy and replica count.
type gpuMokkaTimeSlicingK8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestGPUMokkaTimeSlicingK8sSuite runs the time-slicing tests against one simulated NVIDIA T4 GPU.
func TestGPUMokkaTimeSlicingK8sSuite(t *testing.T) {
	provisioner := gpuMokkaK8sProvisioner(mokkaProfileParams{
		profile: "t4",
		// A single GPU, so that every replica a pod holds is a replica of the same GPU.
		gpuCount: 1,
		mokkaOptions: []mokka.Option{
			mokka.WithGPUOperator(mokka.WithGPUOperatorDevicePluginConfig(timeSlicingDevicePluginConfig)),
		},
	})

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &gpuMokkaTimeSlicingK8sSuite{}, suiteParams...)
}

// TestTimeSlicedGPUMetrics checks that the Agent reports the time-sliced GPU as such: the
// gpu_sharing_mode tag, the GPU's replica count, and the replicas each pod holds.
func (s *gpuMokkaTimeSlicingK8sSuite) TestTimeSlicedGPUMetrics() {
	// The operator starts the device plugin after its Helm release is ready: wait for the replicas,
	// so that a pod pending on them does not look like an Agent failure.
	kubeClient := s.Env().KubernetesCluster.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		nodes, err := kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if !assert.NoError(c, err) {
			return
		}
		var allocatable []string
		ready := false
		for _, node := range nodes.Items {
			gpus := node.Status.Allocatable["nvidia.com/gpu"]
			allocatable = append(allocatable, fmt.Sprintf("%s=%s", node.Name, gpus.String()))
			ready = ready || gpus.Value() == timeSlicingReplicas
		}
		assert.True(c, ready, "no node advertises %d nvidia.com/gpu replicas yet: %v", timeSlicingReplicas, allocatable)
	}, 10*time.Minute, 10*time.Second)

	// One pod holding one replica and one holding two, both of the single GPU.
	suffix := strings.ToLower(common.RandString(5))
	replicasByPod := map[string]int{
		"gpu-sliced-one-" + suffix: 1,
		"gpu-sliced-two-" + suffix: 2,
	}
	var since int64
	for podName, replicas := range replicasByPod {
		since = max(since, startGPUPod(&s.BaseSuite, podName, replicas))
	}

	timeSliced := "gpu_sharing_mode:time-slicing"
	s.EventuallyWithT(func(c *assert.CollectT) {
		fakeIntake := s.Env().FakeIntake.Client()

		// The replica count describes the GPU, not its workloads: no pod tags.
		replicas, err := fakeIntake.FilterMetrics("gpu.device.sharing_replicas")
		if assert.NoError(c, err) {
			value, ok := latestValueSince(replicas, since, func(tags []string) bool {
				return slices.Contains(tags, timeSliced) && tagValue(tags, "pod_name") == ""
			})
			if assert.True(c, ok, "no gpu.device.sharing_replicas with %s and no pod tags since the pods started", timeSliced) {
				assert.EqualValues(c, timeSlicingReplicas, value, "gpu.device.sharing_replicas")
			}
		}

		used, err := fakeIntake.FilterMetrics("gpu.device.sharing_replicas_used")
		if assert.NoError(c, err) {
			for podName, expected := range replicasByPod {
				value, ok := latestValueSince(used, since, func(tags []string) bool {
					return hasTags(tags, []string{timeSliced, "pod_name:" + podName, "kube_namespace:" + jobNamespace})
				})
				if assert.True(c, ok, "no gpu.device.sharing_replicas_used of %s with %s since the pods started", podName, timeSliced) {
					assert.EqualValues(c, expected, value, "gpu.device.sharing_replicas_used of %s", podName)
				}
			}
		}

		// The other GPU metrics of the pods carry the sharing mode too.
		limits, err := fakeIntake.FilterMetrics("gpu.core.limit")
		if assert.NoError(c, err) {
			for podName := range replicasByPod {
				_, ok := latestValueSince(limits, since, func(tags []string) bool {
					return hasTags(tags, []string{timeSliced, "pod_name:" + podName})
				})
				assert.True(c, ok, "no gpu.core.limit of %s with %s since the pods started", podName, timeSliced)
			}
		}
	}, 5*time.Minute, 10*time.Second)
}

// latestValueSince returns the value of the most recent point at or after since among the series
// whose tags match, and whether there is one.
func latestValueSince(metrics []*aggregator.MetricSeries, since int64, match func(tags []string) bool) (float64, bool) {
	var latest *float64
	var latestTimestamp int64
	for _, metric := range metrics {
		if !match(metric.GetTags()) {
			continue
		}
		for _, point := range metric.GetPoints() {
			if point.GetTimestamp() >= since && (latest == nil || point.GetTimestamp() > latestTimestamp) {
				value := point.GetValue()
				latest, latestTimestamp = &value, point.GetTimestamp()
			}
		}
	}
	if latest == nil {
		return 0, false
	}
	return *latest, true
}
