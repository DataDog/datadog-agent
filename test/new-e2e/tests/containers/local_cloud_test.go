// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the attach entry points for the containers suites whose
// Pulumi provisioners need e2ectl environment bases (see `e2ectl environments`
// for the registered ones: kind, eks, ec2-host, docker-host, local). Each
// entry below is the migration contract — the suite type, the environment
// type it will attach to, and the attach shape — and it SKIPS until every
// capability the suite's assertions depend on exists: attaching to nothing,
// or attaching a suite whose assertions describe EKS/ECS/Docker/OpenShift
// infrastructure to a kind cluster, would fail by design. A base registration
// alone is not attachability.
package containers

import (
	"testing"
)

// TestEKSSuiteOnEKS is the migration contract for the ORIGINAL EKS suite
// (eksSuite, the same suite TestEKSSuite runs on a Pulumi-provisioned EKS
// cluster): it will attach an environments.Kubernetes started from an EKS
// base config (the eks base IS registered — see e2ectl examples/eks.yml).
// Still pending before the attach below can run:
//   - the suite's node topology (Bottlerocket + ARM node groups) is not
//     exposed by the first-release typed config (linux/windows only)
//   - the workload catalog has no dogstatsd, test-workload or Argo Rollouts
//     entries, and no Fargate capacity exists (TestEKSFargate asserts it)
//   - the suite's Helm values (dual shipping, Windows agent image) need an
//     e2ectl expressible form (qa-plans/pending/qa-e2ectl-eks-scenario-plan.md).
func TestEKSSuiteOnEKS(t *testing.T) {
	t.Skip("pending suite capabilities on the registered eks base: Bottlerocket/ARM node groups, dogstatsd/test-workload/Argo Rollouts workloads, Fargate capacity and the suite's Helm values (qa-plans/pending/qa-e2ectl-eks-scenario-plan.md)")
}

// TestECSSuiteOnECS is the migration contract for the ORIGINAL ECS suite
// (ecsSuite, the same suite TestECSSuite runs on a Pulumi-provisioned ECS
// cluster): it will attach an environments.ECS started from an ECS base
// config, once that base is registered and its installer section exists.
func TestECSSuiteOnECS(t *testing.T) {
	t.Skip("pending e2ectl base registration: ecs (no ECS base or agent installer section today); the ECS cluster + Fargate capacity providers and test workloads are not expressible until then")
}

// TestDockerSuiteOnDockerHost is the migration contract for the ORIGINAL
// Docker suite (DockerSuite, the same suite TestDockerSuite runs on a
// Pulumi-provisioned EC2 VM with a docker-compose testing workload): it will
// attach an environments.DockerHost started from a docker-host base config
// (the docker-host base IS registered — see e2ectl examples/docker-host.yml).
// Still pending: the redis/dogstatsd docker-compose testing workload
// (scendocker.WithTestingWorkload) is deployed through the Pulumi agent
// compose, which e2ectl installs outside Pulumi — the suite needs a
// docker-compose workload mechanism for EC2 hosts first.
func TestDockerSuiteOnDockerHost(t *testing.T) {
	t.Skip("pending docker-compose workload support on the registered docker-host base (VM + docker runtime): the suite asserts metrics from the Pulumi-managed redis/dogstatsd compose testing workload")
}

// TestOpenShiftVMSuiteOnOpenShift is the migration contract for the ORIGINAL
// OpenShift suite (openShiftVMSuite, the same suite TestOpenShiftVMSuite runs
// on a GCP VM with an in-VM OpenShift cluster): it will attach an
// environments.Kubernetes started from an openshift base config, once that
// base is registered. The suite also depends on the Argo Rollouts workload
// (WithDeployArgoRollout), which is a pending e2ectl catalog entry.
func TestOpenShiftVMSuiteOnOpenShift(t *testing.T) {
	t.Skip("pending e2ectl base registration: openshift (GCP VM + in-VM OpenShift cluster); the Argo Rollouts workload catalog entry is also pending")
}
