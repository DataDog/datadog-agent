// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type kindSuite struct {
	k8sSuite
}

func TestKindSuite(t *testing.T) {
	helmValues := `
datadog:
    logLevel: DEBUG
clusterAgent:
    envDict:
        DD_CLUSTER_AGENT_LANGUAGE_DETECTION_PATCHER_BASE_BACKOFF: "10s"
`
	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(provkind.Provisioner(
		provkind.WithRunOptions(
			scenkind.WithVMOptions(
				scenec2.WithInstanceType("t3.xlarge"),
			),
			scenkind.WithFakeintakeOptions(
				fakeintake.WithMemory(2048),
				fakeintake.WithRetentionPeriod("31m"),
			),
			scenkind.WithDeployDogstatsd(),
			scenkind.WithDeployTestWorkload(),
			scenkind.WithAgentOptions(
				kubernetesagentparams.WithDualShipping(),
				kubernetesagentparams.WithHelmValues(helmValues),
				kubernetesagentparams.WithHelmValues(containerHelmValues),
			),
			scenkind.WithDeployArgoRollout(),
		),
	)))
}

func (suite *kindSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

func (suite *kindSuite) TestControlPlane() {
	// Test `kube_apiserver` check is properly working
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_apiserver.apiserver_request_total",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^apiserver:`,
				`^code:[[:digit:]]{3}$`,
				`^component:(?:|apiserver)$`,
				`^container_id:`,
				`^container_name:kube-apiserver$`,
				`^display_container_name:kube-apiserver_kube-apiserver-.*-control-plane$`,
				`^dry_run:$`,
				`^group:`,
				`^image_id:`,
				`^image_name:(?:k8s\.gcr\.io|registry\.k8s\.io)/kube-apiserver$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-apiserver$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-apiserver-.*-control-plane$`,
				`^pod_phase:running$`,
				`^resource:`,
				`^scope:(?:|cluster|namespace|resource)$`,
				`^short_image:kube-apiserver$`,
				`^subresource:`,
				`^verb:(?:APPLY|DELETE|GET|LIST|PATCH|POST|PUT|PATCH|WATCH|TOTAL)$`,
				`^version:`,
			},
		},
		Optional: testMetricExpectArgs{
			Tags: &[]string{
				`^contentType:`,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_apiserver.api_resource",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^api_resource_kind:.*`,
				`^api_resource_group:.*`,
				`^api_resource_version:.*`,
				`^api_resource_name:.*`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	// Test `kube_controller_manager` check is properly working
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_controller_manager.queue.adds",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:kube-controller-manager$`,
				`^display_container_name:kube-controller-manager_kube-controller-manager-.*-control-plane$`,
				`^image_id:`,
				`^image_name:(?:k8s\.gcr\.io|registry\.k8s\.io)/kube-controller-manager$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-controller-manager$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-controller-manager-.*-control-plane$`,
				`^pod_phase:running$`,
				`^queue:`,
				`^short_image:kube-controller-manager$`,
			},
		},
	})

	// Test `kube_scheduler` check is properly working
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_scheduler.schedule_attempts",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:kube-scheduler$`,
				`^display_container_name:kube-scheduler_kube-scheduler-.*-control-plane$`,
				`^image_id:`,
				`^image_name:(?:k8s\.gcr\.io|registry\.k8s\.io)/kube-scheduler$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-scheduler$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-scheduler-.*-control-plane$`,
				`^pod_phase:running$`,
				`^profile:default-scheduler$`,
				`^result:(?:scheduled|unschedulable|error)$`,
				`^short_image:kube-scheduler$`,
			},
		},
	})
}

func (suite *kindSuite) TestHostTags() {
	expectedTags := []string{
		`^os:linux$`,
		`^arch:amd64$`,
		`^stackid:` + regexp.QuoteMeta(suite.clusterName) + `$`,
		`^kube_node:` + regexp.QuoteMeta(suite.clusterName) + `-control-plane`,
		`^cluster_name:` + regexp.QuoteMeta(suite.clusterName) + `$`,
		`^kube_cluster_name:` + regexp.QuoteMeta(suite.clusterName) + `$`,
		`^orch_cluster_id:[0-9a-f-]{36}$`,
	}

	// depending on the kubernetes version the expected tags for kube_node_rol varies.
	k8sVersion, err := suite.Env().KubernetesCluster.KubernetesClient.K8sClient.Discovery().ServerVersion()
	suite.NoError(err, "failed to request k8s server version to specify the appropriate expected host-tags")

	// depending on kube version we expect different 'kube_node_role' tag value
	// we only handle version we actually test (v1.19, v1.22, ...)
	switch {
	case k8sVersion.Minor == "19":
		expectedTags = append(expectedTags, "^kube_node_role:master$")
	case k8sVersion.Minor == "22":
		expectedTags = append(expectedTags, "^kube_node_role:master$", "^kube_node_role:control-plane$")
	default:
		expectedTags = append(expectedTags, "^kube_node_role:control-plane$")
	}

	// tag keys that are expected to be found on any k8s env
	args := &testHostTags{
		ExpectedTags: expectedTags,
	}

	suite.testHostTags(args)
}
