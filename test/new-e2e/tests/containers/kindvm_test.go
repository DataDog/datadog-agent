// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type kindSuite struct {
	k8sSuite
}

func TestKindSuite(t *testing.T) {
	helmValues := `
datadog:
    logLevel: DEBUG
    envDict:
        # These tests require image-derived tags on the first check configuration.
        DD_AD_TAG_COMPLETENESS_MAX_WAIT: "60"
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
				// Kind suite uses EndpointSlices backed providers while the EKS suite uses the default
				// (legacy) Endpoints providers. This covers tests for both without having to create
				// independent test suites for both configurations or modify providers at test runtime.
				kubernetesagentparams.WithKubernetesUseEndpointSlices(),
			),
			scenkind.WithDeployArgoRollout(),
		),
	)))
}

func (suite *kindSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

func (suite *kindSuite) TestDynamoGraphDeploymentTagOnContainerMetric() {
	const (
		deploymentLabel = "nvidia.com/dynamo-graph-deployment-name"
		deploymentName  = "my-model"
		namespace       = "workload-cpustress"
	)

	ctx := suite.T().Context()
	pods := suite.Env().KubernetesCluster.Client().CoreV1().Pods(namespace)
	list, err := pods.List(ctx, metav1.ListOptions{})
	suite.Require().NoError(err)

	var podName string
	for _, pod := range list.Items {
		if strings.HasPrefix(pod.Name, "stress-ng-") {
			podName = pod.Name
			break
		}
	}
	suite.Require().NotEmpty(podName, "no stress-ng pod found for Dynamo tag test")

	setLabel := func(ctx context.Context, value string) error {
		pod, err := pods.Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if value == "" {
			delete(pod.Labels, deploymentLabel)
		} else {
			pod.Labels[deploymentLabel] = value
		}
		_, err = pods.Update(ctx, pod, metav1.UpdateOptions{})
		return err
	}
	suite.Require().NoError(setLabel(ctx, deploymentName))
	defer func() {
		suite.Require().NoError(setLabel(context.Background(), ""), "restore stress-ng pod labels")
	}()

	suite.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := suite.Fakeintake.FilterMetrics(
			"container.cpu.usage",
			fakeintakeclient.WithTags[*aggregator.MetricSeries]([]string{
				"kube_namespace:" + namespace,
				"pod_name:" + podName,
				"dynamo_graph_deployment:" + deploymentName,
			}),
		)
		require.NoError(c, err)
		assert.NotEmpty(c, metrics, "container CPU metric did not reach fakeintake with the Dynamo deployment tag")
	}, 2*time.Minute, 10*time.Second)
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
