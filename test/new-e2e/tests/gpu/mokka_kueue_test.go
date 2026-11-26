// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/kueue"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

const (
	// kueueTestLocalQueue is the LocalQueue the test workloads are submitted to.
	kueueTestLocalQueue = "team-a"
	// kueueWorkloadNamespace holds the test workloads and their LocalQueue. Mokka does not
	// inject GPUs into its own namespace or kube-system.
	kueueWorkloadNamespace = "kueue-e2e"
)

// k8sChartRepository returns the OCI repository of a chart published on registry.k8s.io under
// path, through the chart mirror when one is configured (see chartMirrorEnvVar).
func k8sChartRepository(path string) string {
	if mirror := os.Getenv(chartMirrorEnvVar); mirror != "" {
		return "oci://" + mirror + "/k8s/" + path
	}
	return "oci://registry.k8s.io/" + path
}

// gpuMokkaKueueSuite checks the Kueue integration on a CPU-only kind cluster: Kueue admits Jobs
// on GPUs simulated by Mokka, and the Agent tags them with their Kueue Workload and queues.
type gpuMokkaKueueSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestGPUMokkaKueueSuite runs the Kueue tagging tests against two simulated NVIDIA T4 GPUs.
func TestGPUMokkaKueueSuite(t *testing.T) {
	provisioner := gpuMokkaK8sProvisioner(mokkaProfileParams{
		profile:  "t4",
		gpuCount: 2,
		runOptions: []kindvm.RunOption{
			kindvm.WithKueue(
				kueue.WithLocalQueue(kueueWorkloadNamespace, kueueTestLocalQueue),
				kueue.WithChartRepository(k8sChartRepository("kueue/charts")),
			),
			kindvm.WithVMOptions(ec2.WithInstanceType("t3.xlarge")),
		},
	})

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}
	e2e.Run(t, &gpuMokkaKueueSuite{}, suiteParams...)
}

// TestKueueJobTags submits a one-GPU Job to a Kueue LocalQueue and checks that the scheduler tags
// reach both the metrics of its container and the metrics of the GPU allocated to it. The
// scheduler queue of a Kueue workload is its ClusterQueue, which holds the quota.
func (s *gpuMokkaKueueSuite) TestKueueJobTags() {
	run := runSchedulerGPUJob(&s.BaseSuite, schedulerGPUJob{
		namespace: kueueWorkloadNamespace,
		jobLabels: map[string]string{kueue.QueueNameLabel: kueueTestLocalQueue},
	})

	assertSchedulerTags(&s.BaseSuite, run, []string{
		"pod_name:" + run.podName,
		tags.KubeNamespace + ":" + kueueWorkloadNamespace,
		tags.Scheduler + ":kueue",
		tags.SchedulerQueue + ":" + kueue.DefaultClusterQueue,
		tags.KueueClusterQueue + ":" + kueue.DefaultClusterQueue,
		tags.KueueLocalQueue + ":" + kueueTestLocalQueue,
	}, tags.KueueWorkload)
}
