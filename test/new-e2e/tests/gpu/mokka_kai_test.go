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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/kaischeduler"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	// kaiTestQueue is the KAI leaf queue the test workloads are submitted to.
	kaiTestQueue = "team-a"
	// kaiWorkloadNamespace holds the test workloads: KAI must not schedule pods of its own
	// namespace, and Mokka does not inject GPUs into its own namespace or kube-system.
	kaiWorkloadNamespace = "kai-e2e"
)

// gpuMokkaKAISuite checks the KAI Scheduler integration on a CPU-only kind cluster: KAI schedules
// pods on GPUs simulated by Mokka, and the Agent tags them with their KAI PodGroup and queue.
type gpuMokkaKAISuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestGPUMokkaKAISuite runs the KAI Scheduler tagging tests against two simulated NVIDIA T4 GPUs.
func TestGPUMokkaKAISuite(t *testing.T) {
	provisioner := gpuMokkaK8sProvisioner(mokkaProfileParams{
		profile:  "t4",
		gpuCount: 2,
		runOptions: []kindvm.RunOption{
			kindvm.WithKAIScheduler(
				kaischeduler.WithQueues(kaiTestQueue),
				kaischeduler.WithChartRepository(ghcrChartRepository("kai-scheduler/kai-scheduler")),
			),
			// KAI adds seven controllers on top of Mokka, the Agent and the Cluster Agent.
			kindvm.WithVMOptions(ec2.WithInstanceType("t3.xlarge")),
		},
	})

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}
	e2e.Run(t, &gpuMokkaKAISuite{}, suiteParams...)
}

// TestKAIJobTags submits a one-GPU Job to a KAI queue and checks that the scheduler tags reach
// both the metrics of its container and the metrics of the GPU allocated to it.
func (s *gpuMokkaKAISuite) TestKAIJobTags() {
	ctx := context.Background()
	kubeClient := s.Env().KubernetesCluster.Client()

	_, err := kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: kaiWorkloadNamespace}}, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		s.Require().NoError(err, "failed to create the workload namespace")
	}

	jobName := strings.ToLower("kai-gpu-job-" + common.RandString(5))
	backoffLimit := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: kaiWorkloadNamespace},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{kaischeduler.QueueLabel: kaiTestQueue}},
				Spec: corev1.PodSpec{
					SchedulerName: kaischeduler.SchedulerName,
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name: "work",
						// Not pause: the Agent excludes pause containers from container metrics.
						// busybox is statically linked, so the LD_PRELOAD Mokka injects does not affect it.
						Image:   busyboxImage,
						Command: []string{"sleep", "3600"},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
						},
					}},
				},
			},
		},
	}
	_, err = kubeClient.BatchV1().Jobs(kaiWorkloadNamespace).Create(ctx, job, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create the KAI job")
	s.T().Cleanup(func() {
		propagation := metav1.DeletePropagationBackground
		_ = kubeClient.BatchV1().Jobs(kaiWorkloadNamespace).Delete(context.Background(), jobName, metav1.DeleteOptions{PropagationPolicy: &propagation})
	})

	var podName string
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := kubeClient.CoreV1().Pods(kaiWorkloadNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("job-name", jobName).String(),
		})
		if !assert.NoError(c, err) || !assert.Len(c, pods.Items, 1, "KAI job pod not created yet") {
			return
		}
		pod := pods.Items[0]
		assert.Equal(c, corev1.PodRunning, pod.Status.Phase, "KAI job pod is not running: %s", podStartDiagnostics(ctx, kubeClient, &pod))
		podName = pod.Name
	}, 5*time.Minute, 5*time.Second, "KAI did not schedule the job on a mock GPU")

	expectedTags := []string{
		"pod_name:" + podName,
		tags.KubeNamespace + ":" + kaiWorkloadNamespace,
		tags.Scheduler + ":kai",
		tags.SchedulerQueue + ":" + kaiTestQueue,
		tags.KAIQueue + ":" + kaiTestQueue,
	}

	s.Run("container metrics", func() {
		s.EventuallyWithT(func(c *assert.CollectT) {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("container.memory.usage", client.WithTags[*aggregator.MetricSeries](expectedTags))
			if !assert.NoError(c, err) || !assert.NotEmpty(c, metrics, "no container.memory.usage with tags %v", expectedTags) {
				return
			}
			// The PodGroup reaches the node Agent after the pod starts, so the first series of the
			// pod only have the queue tags. fakeintake keeps them all: look for one with the PodGroup.
			assertSchedulerWorkloadTags(c, metrics)
		}, 5*time.Minute, 10*time.Second)
	})

	s.Run("allocated GPU metrics", func() {
		s.EventuallyWithT(func(c *assert.CollectT) {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("gpu.core.limit", client.WithTags[*aggregator.MetricSeries](expectedTags))
			if !assert.NoError(c, err) || !assert.NotEmpty(c, metrics, "no gpu.core.limit with tags %v", expectedTags) {
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
			assert.Len(c, uuids, 1, "the job requested one GPU, but its tags are on the metrics of %v", uuids)
			assertSchedulerWorkloadTags(c, metrics)
		}, 5*time.Minute, 10*time.Second)
	})
}

// assertSchedulerWorkloadTags checks that at least one of the series carries both the generic
// scheduler workload tag and the KAI PodGroup tag, naming the same PodGroup. Series from before
// the PodGroup reached the node Agent only have the queue tags and are ignored.
func assertSchedulerWorkloadTags(c *assert.CollectT, metrics []*aggregator.MetricSeries) {
	for _, metric := range metrics {
		workload := tagValue(metric.GetTags(), tags.SchedulerWorkload)
		if workload != "" && workload == tagValue(metric.GetTags(), tags.KAIPodGroup) {
			return
		}
	}
	seen := make([][]string, 0, len(metrics))
	for _, metric := range metrics {
		seen = append(seen, metric.GetTags())
	}
	assert.Fail(c, "no series with matching "+tags.SchedulerWorkload+" and "+tags.KAIPodGroup+" tags", "series tags: %v", seen)
}
