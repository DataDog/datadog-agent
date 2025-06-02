// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	tifeks "github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
)

type eksSuite struct {
	k8sSuite
}

func TestEKSSuite(t *testing.T) {
	e2e.Run(t, &eksSuite{}, e2e.WithProvisioner(awskubernetes.EKSProvisioner(
		awskubernetes.WithEKSOptions(
			tifeks.WithLinuxNodeGroup(),
			tifeks.WithWindowsNodeGroup(),
			tifeks.WithBottlerocketNodeGroup(),
			tifeks.WithLinuxARMNodeGroup(),
		),
		awskubernetes.WithDeployDogstatsd(),
		awskubernetes.WithDeployTestWorkload(),
		awskubernetes.WithAgentOptions(kubernetesagentparams.WithDualShipping()),
	)))
}

func (suite *eksSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

func (suite *eksSuite) TestEKSFargate() {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "eks.fargate.cpu.capacity",
			Tags: []string{
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^eks_fargate_node:fargate-ip-.*\.ec2\.internal$`,
				`^kube_cluster_name:`,
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:dogstatsd-fargate-`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:dogstatsd-fargate-`,
				`^orch_cluster_id:`,
				`^pod_name:dogstatsd-fargate-`,
				`^pod_phase:running$`,
				`^virtual_node:fargate-ip-.*\.ec2\.internal$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 0.25,
				Min: 0.25,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "eks.fargate.memory.capacity",
			Tags: []string{
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^eks_fargate_node:fargate-ip-.*\.ec2\.internal$`,
				`^kube_cluster_name:`,
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:dogstatsd-fargate-`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:dogstatsd-fargate-`,
				`^orch_cluster_id:`,
				`^pod_name:dogstatsd-fargate-`,
				`^pod_phase:running$`,
				`^virtual_node:fargate-ip-.*\.ec2\.internal$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 1024 * 1024 * 1024,
				Min: 1024 * 1024 * 1024,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "eks.fargate.pods.running",
			Tags: []string{
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^eks_fargate_node:fargate-ip-.*\.ec2\.internal$`,
				`^kube_cluster_name:`,
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:dogstatsd-fargate-`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:dogstatsd-fargate-`,
				`^orch_cluster_id:`,
				`^pod_name:dogstatsd-fargate-`,
				`^pod_phase:running$`,
				`^virtual_node:fargate-ip-.*\.ec2\.internal$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 1,
				Min: 1,
			},
		},
	})
}

func (suite *eksSuite) TestDogstatsdFargate() {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^eks_fargate_node:fargate-ip-.*\.ec2\.internal$`,
				`^kube_cluster_name:`,
				`^kube_deployment:dogstatsd-fargate$`,
				`^kube_namespace:workload-dogstatsd-fargate$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:dogstatsd-fargate-`,
				`^orch_cluster_id:`,
				`^pod_phase:running$`,
				`^series:`,
			},
		},
	})
}
