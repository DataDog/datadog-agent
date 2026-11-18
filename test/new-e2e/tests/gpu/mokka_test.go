// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// mokkaT4System describes the GPUs simulated by Mokka with the T4 profile. Mokka implements
// the NVML APIs the GPU check needs, but does not execute CUDA, so the system-probe
// (eBPF) tests are skipped.
var mokkaT4System = systemData{
	hasAllNVMLCriticalAPIs:       true,
	supportsSystemProbeComponent: false,
}

// gpuMokkaK8sSuite runs the GPU Kubernetes tests on a CPU-only kind cluster where the GPUs
// are simulated by Mokka (NVIDIA nvml-mock). It inherits every gpuBaseSuite test, plus the
// Mokka-specific ones below.
type gpuMokkaK8sSuite struct {
	gpuK8sSuite
}

// TestGPUMokkaK8sSuite runs the GPU Kubernetes tests against two simulated NVIDIA T4 GPUs.
// Not to be run in parallel, as some tests wait until the checks are available.
func TestGPUMokkaK8sSuite(t *testing.T) {
	provisioner := gpuMokkaK8sProvisioner(mokkaProfileParams{profile: "t4", gpuCount: 2})

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	suite := &gpuMokkaK8sSuite{
		gpuK8sSuite: gpuK8sSuite{
			gpuBaseSuite: gpuBaseSuite[environments.Kubernetes]{
				provisioner: provisioner,
				systemData:  mokkaT4System,
			},
		},
	}

	e2e.Run(t, suite, suiteParams...)
}

func (s *gpuMokkaK8sSuite) SetupSuite() {
	// The base suite needs the capabilities struct, so set it before calling the base SetupSuite.
	// Kind nodes have no NVIDIA RuntimeClass: Mokka injects the GPUs through its NRI plugin.
	s.caps = &kubernetesCapabilities{
		suite:          &s.BaseSuite,
		containerToJob: make(map[string]string),
	}
	s.gpuBaseSuite.SetupSuite()
}

// TestMockGPUAllocationTags checks that the metrics of a GPU allocated to a pod carry the
// tags of that pod, and that only one GPU does. The allocation comes from the kubelet
// pod-resources API, fed by the NVIDIA device plugin running on the mock driver.
func (s *gpuMokkaK8sSuite) TestMockGPUAllocationTags() {
	ctx := context.Background()
	podName := strings.ToLower("gpu-sleeper-" + common.RandString(5))
	kubeClient := s.Env().KubernetesCluster.Client()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: jobNamespace},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name: "main",
				// The pause image is already on the kind nodes, and as a static binary it is not
				// affected by the LD_PRELOAD that Mokka injects into GPU containers.
				Image: "registry.k8s.io/pause:3.10",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
				},
			}},
		},
	}
	_, err := kubeClient.CoreV1().Pods(jobNamespace).Create(ctx, pod, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create the GPU pod")
	s.T().Cleanup(func() {
		_ = kubeClient.CoreV1().Pods(jobNamespace).Delete(context.Background(), podName, metav1.DeleteOptions{})
	})

	s.EventuallyWithT(func(c *assert.CollectT) {
		p, err := kubeClient.CoreV1().Pods(jobNamespace).Get(ctx, podName, metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, corev1.PodRunning, p.Status.Phase, "GPU pod is not running: %s %s", p.Status.Reason, p.Status.Message)
	}, 3*time.Minute, 5*time.Second)

	expectedTags := []string{"pod_name:" + podName, "kube_namespace:" + jobNamespace, "kube_container_name:main"}
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.caps.FakeIntake().Client().FilterMetrics("gpu.core.limit", client.WithTags[*aggregator.MetricSeries](expectedTags))
		if !assert.NoError(c, err) {
			return
		}
		if !assertMetricsHaveExpectedTagKeys(c, metrics, mandatoryMetricTags, "gpu.core.limit") {
			return
		}

		uuids := make(map[string]struct{})
		for _, metric := range metrics {
			for _, tag := range metric.GetTags() {
				if strings.HasPrefix(tag, "gpu_uuid:") {
					uuids[tag] = struct{}{}
				}
			}
		}
		assert.Len(c, uuids, 1, "the pod requested one GPU, but its tags are on the metrics of %v", uuids)
	}, 5*time.Minute, 10*time.Second)
}
