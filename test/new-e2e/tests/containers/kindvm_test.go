// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/dynamic"
)

//go:embed fixtures/kubeai-model-crd.json
var kubeAIModelCRD []byte

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
				// The task memory is shared with a datadog-agent sidecar. With 31m of retention, a smaller
				// task gets OOM-killed and replaced by one with a new IP, failing all remaining queries.
				// Fargate requires 1024 CPU units for more than 4096MiB.
				fakeintake.WithCPU(1024),
				fakeintake.WithMemory(6144),
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

func (suite *kindSuite) TestDynamoInferenceTagsOnContainerMetric() {
	suite.testInferenceTagsOnContainerMetric(map[string]string{
		"nvidia.com/dynamo-graph-deployment-name": "my-model",
		"nvidia.com/dynamo-component-type":        "worker",
		"nvidia.com/dynamo-sub-component-type":    "decode",
	}, []string{
		`^dynamo_graph_deployment:my-model$`,
		`^inference_workload:my-model$`,
		`^inference_workload_component:decode$`,
	})
}

func (suite *kindSuite) TestKubeAIInferenceTagsOnContainerMetric() {
	ctx := suite.T().Context()
	client, err := dynamic.NewForConfig(suite.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	suite.Require().NoError(err)
	crds := client.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
	var crd unstructured.Unstructured
	suite.Require().NoError(json.Unmarshal(kubeAIModelCRD, &crd))
	_, err = crds.Create(ctx, &crd, metav1.CreateOptions{})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		ctx := context.Background()
		suite.Require().NoError(crds.Delete(ctx, crd.GetName(), metav1.DeleteOptions{}))
		suite.EventuallyWithT(func(c *assert.CollectT) {
			_, err := crds.Get(ctx, crd.GetName(), metav1.GetOptions{})
			assert.True(c, apierrors.IsNotFound(err), "wait for test CRD deletion: %v", err)
		}, time.Minute, time.Second)
	})
	suite.EventuallyWithT(func(c *assert.CollectT) {
		current, err := crds.Get(ctx, crd.GetName(), metav1.GetOptions{})
		require.NoError(c, err)
		conditions, _, err := unstructured.NestedSlice(current.Object, "status", "conditions")
		require.NoError(c, err)
		for _, condition := range conditions {
			fields, ok := condition.(map[string]interface{})
			if ok && fields["type"] == "Established" && fields["status"] == "True" {
				return
			}
		}
		assert.Fail(c, "test Model CRD is not established")
	}, time.Minute, time.Second)

	models := client.Resource(schema.GroupVersionResource{Group: "kubeai.org", Version: "v1", Resource: "models"}).Namespace("workload-cpustress")
	model, err := models.Create(ctx, &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubeai.org/v1",
		"kind":       "Model",
		"metadata":   map[string]interface{}{"name": "kubeai-model"},
	}}, metav1.CreateOptions{})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		suite.Require().NoError(models.Delete(context.Background(), model.GetName(), metav1.DeleteOptions{}))
	})

	// Kubelet does not refresh owner references on an existing pod. Set the Model
	// owner at creation, as KubeAI does, and reuse the provisioned stress workload's
	// image and pod spec without its ReplicaSet labels or owner.
	k8sClient := suite.Env().KubernetesCluster.Client()
	deployment, err := k8sClient.AppsV1().Deployments(model.GetNamespace()).Get(ctx, "stress-ng", metav1.GetOptions{})
	suite.Require().NoError(err)
	pods := k8sClient.CoreV1().Pods(model.GetNamespace())
	pod, err := pods.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kubeai-model-",
			// A conflicting label verifies that the workload name comes from the owner.
			Labels: map[string]string{"kubeai.org/model": "wrong-label-name"},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(model, schema.GroupVersionKind{
				Group: "kubeai.org", Version: "v1", Kind: "Model",
			})},
		},
		Spec: *deployment.Spec.Template.Spec.DeepCopy(),
	}, metav1.CreateOptions{})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		ctx := context.Background()
		err := pods.Delete(ctx, pod.Name, metav1.DeleteOptions{})
		suite.Require().True(err == nil || apierrors.IsNotFound(err), "delete test pod: %v", err)
		suite.EventuallyWithT(func(c *assert.CollectT) {
			_, err := pods.Get(ctx, pod.Name, metav1.GetOptions{})
			assert.True(c, apierrors.IsNotFound(err), "wait for test pod deletion: %v", err)
		}, time.Minute, time.Second)
	})
	suite.EventuallyWithT(func(c *assert.CollectT) {
		current, err := pods.Get(ctx, pod.Name, metav1.GetOptions{})
		require.NoError(c, err)
		for _, condition := range current.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return
			}
		}
		assert.Fail(c, "test Model pod is not ready")
	}, 2*time.Minute, time.Second)

	inferenceTags := []string{
		`^kube_namespace:` + regexp.QuoteMeta(pod.Namespace) + `$`,
		`^pod_name:` + regexp.QuoteMeta(pod.Name) + `$`,
		`^inference_workload:kubeai-model$`,
		`^inference_workload_component:inference$`,
		`^kube_ownerref_kind:model$`,
		`^kube_ownerref_name:kubeai-model$`,
	}
	expectedTags := append([]string{
		`^container_id:`,
		`^container_name:stress-ng$`,
		`^display_container_name:stress-ng_` + regexp.QuoteMeta(pod.Name) + `$`,
		`^git\.commit\.sha:[[:xdigit:]]{40}$`,
		`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`,
		`^image_id:.*/apps-stress-ng@sha256:`,
		`^image_name:.*/apps-stress-ng$`,
		`^image_tag:`,
		`^kube_container_name:stress-ng$`,
		`^kube_qos:Guaranteed$`,
		`^pod_phase:running$`,
		`^runtime:containerd$`,
		`^short_image:apps-stress-ng$`,
	}, inferenceTags...)
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: inferenceTags,
		},
		Expect: testMetricExpectArgs{
			Tags: &expectedTags,
		},
	})
}

func (suite *kindSuite) testInferenceTagsOnContainerMetric(podLabels map[string]string, inferenceTags []string) {
	const namespace = "workload-cpustress"

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
	suite.Require().NotEmpty(podName, "no stress-ng pod found for inference tag test")

	// Restore pod metadata so a retry can use the same pod.
	pod, err := pods.Get(ctx, podName, metav1.GetOptions{})
	suite.Require().NoError(err)
	originalLabels := pod.Labels
	setLabels := func(ctx context.Context, labels map[string]string) error {
		pod, err := pods.Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		pod.Labels = labels
		_, err = pods.Update(ctx, pod, metav1.UpdateOptions{})
		return err
	}
	labels := make(map[string]string, len(originalLabels)+len(podLabels))
	for key, value := range originalLabels {
		labels[key] = value
	}
	for key, value := range podLabels {
		labels[key] = value
	}
	suite.Require().NoError(setLabels(ctx, labels))
	defer func() {
		suite.Require().NoError(setLabels(context.Background(), originalLabels), "restore stress-ng pod labels")
	}()

	expectedTags := append([]string{
		`^container_id:`,
		`^container_name:stress-ng$`,
		`^display_container_name:stress-ng`,
		`^git\.commit\.sha:[[:xdigit:]]{40}$`,
		`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`,
		`^image_id:.*/apps-stress-ng@sha256:`,
		`^image_name:.*/apps-stress-ng$`,
		`^image_tag:`,
		`^kube_container_name:stress-ng$`,
		`^kube_deployment:stress-ng$`,
		`^kube_namespace:` + namespace + `$`,
		`^kube_ownerref_kind:replicaset$`,
		`^kube_ownerref_name:stress-ng-[[:alnum:]]+$`,
		`^kube_qos:Guaranteed$`,
		`^kube_replica_set:stress-ng-[[:alnum:]]+$`,
		`^pod_name:` + regexp.QuoteMeta(podName) + `$`,
		`^pod_phase:running$`,
		`^runtime:containerd$`,
		`^short_image:apps-stress-ng$`,
	}, inferenceTags...)

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: append([]string{
				`^kube_namespace:` + namespace + `$`,
				`^pod_name:` + regexp.QuoteMeta(podName) + `$`,
			}, inferenceTags...),
		},
		Expect: testMetricExpectArgs{
			Tags: &expectedTags,
		},
	})
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

	serverVersion, err := suite.Env().KubernetesCluster.KubernetesClient.K8sClient.Discovery().ServerVersion()
	suite.Require().NoError(err, "failed to request the Kubernetes server version")
	k8sVersion, err := utilversion.ParseGeneric(serverVersion.GitVersion)
	suite.Require().NoError(err, "failed to parse the Kubernetes server version")

	// apiserver_storage_objects was deprecated in Kubernetes 1.34 and is hidden starting in 1.37.
	// Its replacement, apiserver_resource_objects, is Alpha and intentionally unsupported here.
	if !k8sVersion.AtLeast(utilversion.MajorMinor(1, 37)) {
		suite.testMetric(&testMetricArgs{
			Filter: testMetricFilterArgs{
				Name: "kube_apiserver.storage_objects",
			},
			Expect: testMetricExpectArgs{
				Tags: &[]string{
					`^resource:.*`,
				},
				AcceptUnexpectedTags: true,
			},
		})
	}

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
