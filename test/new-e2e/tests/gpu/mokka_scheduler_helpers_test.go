// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"context"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// schedulerGPUJob describes a one-GPU Job submitted to a batch scheduler (KAI, Kueue...).
type schedulerGPUJob struct {
	namespace string
	// jobLabels are set on the Job itself (e.g. the Kueue queue name).
	jobLabels map[string]string
	// podLabels are set on the pod template (e.g. the KAI queue).
	podLabels map[string]string
	// schedulerName is the pod schedulerName, empty for the default scheduler.
	schedulerName string
}

// schedulerGPUJobRun is a running schedulerGPUJob.
type schedulerGPUJobRun struct {
	podName string
	// allocatedGPU is the UUID (lowercase, as in the gpu_uuid tag) of the GPU the device plugin
	// gave to the pod, read from the container independently of the Agent.
	allocatedGPU string
	// runningSince is the Unix time at which the pod was seen running.
	runningSince int64
}

// runSchedulerGPUJob creates the Job, waits until its pod runs on a GPU and returns the pod and
// its GPU. The Job is deleted at the end of the test.
func runSchedulerGPUJob(s *e2e.BaseSuite[environments.Kubernetes], job schedulerGPUJob) schedulerGPUJobRun {
	ctx := context.Background()
	kubeClient := s.Env().KubernetesCluster.Client()

	jobName := strings.ToLower("gpu-job-" + common.RandString(5))
	backoffLimit := int32(0)
	spec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: job.namespace, Labels: job.jobLabels},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: job.podLabels},
				Spec: corev1.PodSpec{
					SchedulerName: job.schedulerName,
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name: "work",
						// Not pause: the Agent excludes pause containers from container metrics.
						// busybox is statically linked, so the LD_PRELOAD Mokka injects does not affect it.
						Image:   busyboxImage,
						Command: []string{"sleep", "3600"},
						Resources: corev1.ResourceRequirements{
							// Explicit requests, so that quota-based schedulers can admit the Job.
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("16Mi"),
							},
							Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
						},
					}},
				},
			},
		},
	}
	_, err := kubeClient.BatchV1().Jobs(job.namespace).Create(ctx, spec, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create the GPU job")
	s.T().Cleanup(func() {
		propagation := metav1.DeletePropagationBackground
		_ = kubeClient.BatchV1().Jobs(job.namespace).Delete(context.Background(), jobName, metav1.DeleteOptions{PropagationPolicy: &propagation})
	})

	var podName string
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := kubeClient.CoreV1().Pods(job.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("job-name", jobName).String(),
		})
		if !assert.NoError(c, err) || !assert.Len(c, pods.Items, 1, "job pod not created yet") {
			return
		}
		pod := pods.Items[0]
		assert.Equal(c, corev1.PodRunning, pod.Status.Phase, "job pod is not running: %s", podStartDiagnostics(ctx, kubeClient, &pod))
		podName = pod.Name
	}, 5*time.Minute, 5*time.Second, "the scheduler did not run the job on a mock GPU")
	runningSince := time.Now().Unix()

	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(job.namespace, podName, "work", []string{"printenv", "NVIDIA_VISIBLE_DEVICES"})
	s.Require().NoError(err, "failed to read the allocated GPU: %s", stderr)
	allocatedGPU := strings.ToLower(strings.TrimSpace(stdout))
	s.Require().NotEmpty(allocatedGPU, "the device plugin did not set NVIDIA_VISIBLE_DEVICES")
	s.Require().NotContains(allocatedGPU, ",", "the job requested one GPU but got %s", allocatedGPU)
	s.T().Logf("GPU allocated to %s: %s", podName, allocatedGPU)

	return schedulerGPUJobRun{podName: podName, allocatedGPU: allocatedGPU, runningSince: runningSince}
}

// assertSchedulerTags checks that expectedTags are on both the container metrics of the job and
// the metrics of the GPU allocated to it (and on no other GPU), and that the generic scheduler
// workload tag and the scheduler-specific workloadTagKey are set and name the same workload.
//
// Tags are eventually consistent: until the Cluster Agent streams the scheduler workload object
// to the node, pods only get the tags derived from their own labels (e.g. the queue). fakeintake
// keeps every series of the run, so the checks look for series that already carry the workload.
func assertSchedulerTags(s *e2e.BaseSuite[environments.Kubernetes], run schedulerGPUJobRun, expectedTags []string, workloadTagKey string) {
	s.Run("container metrics", func() {
		s.EventuallyWithT(func(c *assert.CollectT) {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("container.memory.usage", client.WithTags[*aggregator.MetricSeries](expectedTags))
			if !assert.NoError(c, err) || !assert.NotEmpty(c, metrics, "no container.memory.usage with tags %v", expectedTags) {
				return
			}
			assert.NotEmpty(c, withSameTagValue(metrics, tags.SchedulerWorkload, workloadTagKey),
				"no container.memory.usage with matching "+tags.SchedulerWorkload+" and %s tags, got: %v", workloadTagKey, tagSets(metrics))
		}, 5*time.Minute, 10*time.Second)
	})

	s.Run("allocated GPU metrics", func() {
		s.EventuallyWithT(func(c *assert.CollectT) {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("gpu.core.limit")
			if !assert.NoError(c, err) {
				return
			}

			// Every GPU must have reported since the pod started, so that a missing series of another
			// GPU can't hide misplaced job tags.
			reportedUUIDs := make(map[string]struct{})
			var allocated []*aggregator.MetricSeries
			for _, metric := range metrics {
				if !hasPointSince(metric, run.runningSince) {
					continue
				}
				// A series without gpu_uuid is not a real GPU and must not count towards the GPUs that reported.
				uuid := tagValue(metric.GetTags(), tags.KubeGPUUUID)
				if uuid == "" {
					continue
				}
				reportedUUIDs[uuid] = struct{}{}
				// Any series tagged with the job pod, even before the scheduler tags arrive, must be
				// of the allocated GPU.
				if !hasTags(metric.GetTags(), []string{"pod_name:" + run.podName}) {
					continue
				}
				if !assert.Equal(c, run.allocatedGPU, uuid, "the job pod tags are on a GPU that was not allocated to it: %v", metric.GetTags()) {
					continue
				}
				if hasTags(metric.GetTags(), expectedTags) {
					allocated = append(allocated, metric)
				}
			}
			assert.Len(c, reportedUUIDs, mokkaT4GPUCount, "not every GPU reported gpu.core.limit since the pod started: %v", reportedUUIDs)
			if !assert.NotEmpty(c, allocated, "no gpu.core.limit of the allocated GPU %s with tags %v", run.allocatedGPU, expectedTags) {
				return
			}
			assert.NotEmpty(c, withSameTagValue(allocated, tags.SchedulerWorkload, workloadTagKey),
				"no gpu.core.limit of the allocated GPU with matching "+tags.SchedulerWorkload+" and %s tags, got: %v", workloadTagKey, tagSets(allocated))
		}, 5*time.Minute, 10*time.Second)
	})
}

// withSameTagValue returns the series whose tags keyA and keyB are both set, with the same value.
func withSameTagValue(metrics []*aggregator.MetricSeries, keyA, keyB string) []*aggregator.MetricSeries {
	var out []*aggregator.MetricSeries
	for _, metric := range metrics {
		var valueA, valueB string
		for _, tag := range metric.GetTags() {
			if v, ok := strings.CutPrefix(tag, keyA+":"); ok {
				valueA = v
			}
			if v, ok := strings.CutPrefix(tag, keyB+":"); ok {
				valueB = v
			}
		}
		if valueA != "" && valueA == valueB {
			out = append(out, metric)
		}
	}
	return out
}

// tagSets returns the tags of each series, for failure messages.
func tagSets(metrics []*aggregator.MetricSeries) [][]string {
	out := make([][]string, 0, len(metrics))
	for _, metric := range metrics {
		out = append(out, metric.GetTags())
	}
	return out
}
