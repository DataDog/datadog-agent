// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/kaischeduler"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
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
	_, err := s.Env().KubernetesCluster.Client().CoreV1().Namespaces().Create(context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: kaiWorkloadNamespace}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		s.Require().NoError(err, "failed to create the workload namespace")
	}

	run := runSchedulerGPUJob(&s.BaseSuite, schedulerGPUJob{
		namespace:     kaiWorkloadNamespace,
		podLabels:     map[string]string{kaischeduler.QueueLabel: kaiTestQueue},
		schedulerName: kaischeduler.SchedulerName,
	})

	assertSchedulerTags(&s.BaseSuite, run, []string{
		"pod_name:" + run.podName,
		tags.KubeNamespace + ":" + kaiWorkloadNamespace,
		tags.Scheduler + ":kai",
		tags.SchedulerQueue + ":" + kaiTestQueue,
		tags.KAIQueue + ":" + kaiTestQueue,
	}, tags.KAIPodGroup)
}
