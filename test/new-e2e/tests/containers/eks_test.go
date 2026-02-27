// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	_ "embed"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	sceneks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	proveks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
)

//go:embed values.yaml
var containerHelmValues string

type eksSuite struct {
	k8sSuite
}

func TestEKSSuite(t *testing.T) {
	e2e.Run(t, &eksSuite{}, e2e.WithProvisioner(proveks.Provisioner(
		proveks.WithRunOptions(
			sceneks.WithEKSOptions(
				sceneks.WithLinuxNodeGroup(),
				sceneks.WithWindowsNodeGroup(),
				sceneks.WithBottlerocketNodeGroup(),
				sceneks.WithLinuxARMNodeGroup(),
			),
			sceneks.WithDeployDogstatsd(),
			sceneks.WithDeployTestWorkload(),
			sceneks.WithFakeIntakeOptions(
				fakeintake.WithRetentionPeriod("31m"),
			),
			sceneks.WithAgentOptions(
				kubernetesagentparams.WithDualShipping(),
				kubernetesagentparams.WithWindowsImage(),
				kubernetesagentparams.WithHelmValues(containerHelmValues),
			),
			sceneks.WithDeployArgoRollout(),
		),
	)))
}

func (suite *eksSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.envSpecificClusterTags = []string{"^kube_distribution:eks$"}
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
				"^kube_distribution:eks$",
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
				"^kube_distribution:eks$",
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
				"^kube_distribution:eks$",
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
				"^kube_distribution:eks$",
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

func (suite *eksSuite) TestNginxFargate() {
	// `nginx` check is configured via AD annotation on pods
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "nginx.net.request_per_s",
			Tags: []string{
				`^kube_namespace:workload-nginx-fargate$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:nginx$`,
				`^display_container_name:nginx`,
				`^eks_fargate_node:fargate-ip-.*\.ec2\.internal$`,
				`^image_id:ghcr\.io/datadog/apps-nginx-server@sha256:`,
				`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^kube_cluster_name:`,
				"^kube_distribution:eks$",
				`^kube_container_name:nginx$`,
				`^kube_deployment:nginx$`,
				`^kube_namespace:workload-nginx-fargate$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:nginx-[[:alnum:]]+$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:nginx-[[:alnum:]]+$`,
				`^kube_service:nginx$`,
				`^nginx_host:`,
				`^orch_cluster_id:`,
				`^pod_name:nginx-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^port:`,
				`^short_image:apps-nginx-server$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	// `http_check` is configured via AD annotation on service
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "network.http.response_time",
			Tags: []string{
				`^kube_namespace:workload-nginx-fargate$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^cluster_name:`,
				`^instance:My_Nginx$`,
				`^kube_cluster_name:`,
				"^kube_distribution:eks$",
				`^orch_cluster_id:`,
				`^kube_namespace:workload-nginx-fargate$`,
				`^kube_service:nginx$`,
				`^url:http://`,
			},
		},
	})

	// Test Nginx logs
	suite.testLog(&testLogArgs{
		Filter: testLogFilterArgs{
			Service: "nginx-fargate",
			Tags: []string{
				`^kube_namespace:workload-nginx-fargate$`,
			},
		},
		Expect: testLogExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:nginx$`,
				`^display_container_name:nginx`,
				`^eks_fargate_node:fargate-ip-.*\.ec2\.internal$`,
				`^image_id:ghcr\.io/datadog/apps-nginx-server@sha256:`,
				`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^kube_cluster_name:`,
				"^kube_distribution:eks$",
				`^kube_container_name:nginx$`,
				`^kube_deployment:nginx$`,
				`^kube_namespace:workload-nginx-fargate$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:nginx-[[:alnum:]]+$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:nginx-[[:alnum:]]+$`,
				`^kube_service:nginx$`,
				`^orch_cluster_id:`,
				`^pod_name:nginx-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^short_image:apps-nginx-server$`,
			},
			Message: `GET / HTTP/1\.1`,
		},
	})
}

func (suite *eksSuite) TestHostTags() {
	// tag keys that are expected to be found on any k8s env
	args := &testHostTags{
		// EKS suite run multiple hosts, with various OS, various CPU architecture
		// The only common tags are: `stackid`, `kube_distribution`, `kube_node`, `orch_cluster_id`
		//
		// the below 4 tags should be present on all nodes
		ExpectedTags: []string{
			`^kube_distribution:eks$`,
			`^kube_node:ip-([0-9]{1,3}-){3}[0-9]{1,3}\.ec2\.internal$`,
			`^orch_cluster_id:[0-9a-f-]{36}$`,
			`^stackid:` + regexp.QuoteMeta(suite.clusterName) + `$`,
		},
		// the below list of tags is from various hosts
		// if the tag is present
		OptionalTags: []string{
			`^arch:(amd|arm)64$`,
			`^cluster_name:` + regexp.QuoteMeta(suite.clusterName) + `$`,
			`^kube_cluster_name:` + regexp.QuoteMeta(suite.clusterName) + `$`,
			`nodegroup-image:ami-[0-9a-f]{17}`,
			`^os:linux$`,
		},
	}

	suite.testHostTags(args)
}
