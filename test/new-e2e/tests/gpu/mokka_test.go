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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// mokkaT4GPUCount is the number of GPUs simulated by the Mokka suites.
const mokkaT4GPUCount = 2

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
	provisioner := gpuMokkaK8sProvisioner(mokkaProfileParams{profile: "t4", gpuCount: mokkaT4GPUCount})

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

// TestMockGPUAllocationTags checks that the pod's tags are on the metrics of exactly the GPU the
// device plugin allocated to it, and not on the other GPUs. The allocated GPU is read from the
// container (NVIDIA_VISIBLE_DEVICES, set by the device plugin), independently of the Agent, which
// maps GPUs to pods through the kubelet pod-resources API.
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
				// busybox has a shell to read the allocated GPU, and is statically linked, so the
				// LD_PRELOAD that Mokka injects into GPU containers does not affect it.
				Image:   busyboxImage,
				Command: []string{"sleep", "3600"},
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

	// The reference time comes from the container's start time, set by the kubelet on the VM, and
	// not from the test machine's clock: the Agent timestamps its points with the VM clock too.
	var podRunningSince int64
	s.EventuallyWithT(func(c *assert.CollectT) {
		p, err := kubeClient.CoreV1().Pods(jobNamespace).Get(ctx, podName, metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}
		if !assert.Equal(c, corev1.PodRunning, p.Status.Phase, "GPU pod is not running: %s", podStartDiagnostics(ctx, kubeClient, p)) {
			return
		}
		if assert.NotEmpty(c, p.Status.ContainerStatuses, "no container status yet") &&
			assert.NotNil(c, p.Status.ContainerStatuses[0].State.Running, "the container is not running yet") {
			podRunningSince = p.Status.ContainerStatuses[0].State.Running.StartedAt.Unix()
		}
	}, 3*time.Minute, 5*time.Second)

	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(jobNamespace, podName, "main", []string{"printenv", "NVIDIA_VISIBLE_DEVICES"})
	s.Require().NoError(err, "failed to read the allocated GPU: %s", stderr)
	allocatedUUID := strings.ToLower(strings.TrimSpace(stdout))
	s.Require().NotEmpty(allocatedUUID, "the device plugin did not set NVIDIA_VISIBLE_DEVICES")
	s.Require().NotContains(allocatedUUID, ",", "the pod requested one GPU but got %s", allocatedUUID)
	s.T().Logf("GPU allocated to %s: %s", podName, allocatedUUID)

	podTags := []string{"pod_name:" + podName, "kube_namespace:" + jobNamespace, "kube_container_name:main"}
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.caps.FakeIntake().Client().FilterMetrics("gpu.core.limit")
		if !assert.NoError(c, err) {
			return
		}

		// Every GPU must have reported since the pod started, so that a missing series of another
		// GPU can't hide misplaced pod tags.
		reportedUUIDs := make(map[string]struct{})
		var allocatedWithPodTags []*aggregator.MetricSeries
		for _, metric := range metrics {
			uuid := tagValue(metric.GetTags(), "gpu_uuid")
			if !hasPointSince(metric, podRunningSince) {
				continue
			}
			reportedUUIDs[uuid] = struct{}{}
			if !hasTags(metric.GetTags(), podTags) {
				continue
			}
			if assert.Equal(c, allocatedUUID, uuid, "the pod tags are on a GPU that was not allocated to it: %v", metric.GetTags()) {
				allocatedWithPodTags = append(allocatedWithPodTags, metric)
			}
		}
		assert.Len(c, reportedUUIDs, mokkaT4GPUCount, "not every GPU reported gpu.core.limit since the pod started: %v", reportedUUIDs)
		if assert.NotEmpty(c, allocatedWithPodTags, "no gpu.core.limit of the allocated GPU %s with the pod tags %v", allocatedUUID, podTags) {
			assertMetricsHaveExpectedTagKeys(c, allocatedWithPodTags, mandatoryMetricTags, "gpu.core.limit")
		}
	}, 5*time.Minute, 10*time.Second)
}

// tagValue returns the value of the tag key, or an empty string.
func tagValue(tags []string, key string) string {
	for _, tag := range tags {
		if v, ok := strings.CutPrefix(tag, key+":"); ok {
			return v
		}
	}
	return ""
}

// hasTags reports whether tags contains every expected tag.
func hasTags(tags []string, expected []string) bool {
	for _, e := range expected {
		if !slices.Contains(tags, e) {
			return false
		}
	}
	return true
}

// hasPointSince reports whether the series has a point at or after the given Unix timestamp.
func hasPointSince(metric *aggregator.MetricSeries, since int64) bool {
	for _, point := range metric.GetPoints() {
		if point.GetTimestamp() >= since {
			return true
		}
	}
	return false
}

// podStartDiagnostics describes why a pod is not running: its phase, the waiting reason and message
// of its containers (for example ErrImagePull), and its events. A Pending pod has no reason of its own.
func podStartDiagnostics(ctx context.Context, kubeClient kubernetes.Interface, pod *corev1.Pod) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "phase=%s reason=%q message=%q", pod.Status.Phase, pod.Status.Reason, pod.Status.Message)
	for _, cs := range pod.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil {
			fmt.Fprintf(&sb, "; container %s waiting: %s: %s", cs.Name, w.Reason, w.Message)
		}
	}
	events, err := kubeClient.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + pod.Name})
	if err != nil {
		fmt.Fprintf(&sb, "; could not list the pod events: %v", err)
		return sb.String()
	}
	for _, e := range events.Items {
		fmt.Fprintf(&sb, "; event %s %s: %s", e.Type, e.Reason, e.Message)
	}
	return sb.String()
}
