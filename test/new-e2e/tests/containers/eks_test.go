// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	tifeks "github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
)

type eksSuite struct {
	k8sSuite
}

func refreshAWSCredentials() {
	// TODO: Only in CI
	if os.Getenv("GITLAB_CI") == "" {
		return
	}

	println("Assuming ddbuild-agent-ci role")

	// Assume role to fetch only once credentials and avoid rate limits
	cmd := "aws sts assume-role --role-arn arn:aws:iam::669783387624:role/ddbuild-agent-ci --external-id ddbuild-agent-ci --role-session-name RoleSession"
	roleoutput, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Fatalf("Failed to assume role: %v", err)
	}

	// TODO: Error case from aws print error
	// Parse json
	var output = make(map[string]interface{})
	err = json.Unmarshal(roleoutput, &output)
	if err != nil {
		log.Fatalf("Failed to parse role output: %v", err)
	}

	// Set env vars
	println("Got credentials ", len(output["Credentials"].(map[string]interface{})["AccessKeyId"].(string)))
	os.Setenv("AWS_ACCESS_KEY_ID", output["Credentials"].(map[string]interface{})["AccessKeyId"].(string))
	os.Setenv("AWS_SECRET_ACCESS_KEY", output["Credentials"].(map[string]interface{})["SecretAccessKey"].(string))
	os.Setenv("AWS_SESSION_TOKEN", output["Credentials"].(map[string]interface{})["SessionToken"].(string))

	// export AWS_ACCESS_KEY_ID="$(echo "$roleoutput" | jq -r '.Credentials.AccessKeyId')"
	// export AWS_SECRET_ACCESS_KEY="$(echo "$roleoutput" | jq -r '.Credentials.SecretAccessKey')"
	// export AWS_SESSION_TOKEN="$(echo "$roleoutput" | jq -r '.Credentials.SessionToken')"
}

func TestEKSSuite(t *testing.T) {
	// TODO: Do this after 59m
	refreshAWSCredentials()

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
