// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

type autoscalingSuite struct {
	baseSuite[environments.Kubernetes]
}

// Local Kind test runner for autoscaling suite
type kindAutoscalingSuite struct {
	autoscalingSuite
}

func TestEKSAutoscalingSuite(t *testing.T) {
	e2e.Run(t, &kindAutoscalingSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithEC2VMOptions(
			ec2.WithInstanceType("t3.xlarge"),
		),
		awskubernetes.WithFakeIntakeOptions(fakeintake.WithMemory(2048)),
		awskubernetes.WithAgentOptions(
			kubernetesagentparams.WithDualShipping(),
		),
	)))
}

func (suite *kindAutoscalingSuite) SetupSuite() {
	suite.autoscalingSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

func (suite *autoscalingSuite) TestAutoscalingRecommendations() {
	ctx := context.Background()

	deploymentName := "test-autoscaling-deployment"
	namespace := "workload-autoscaling"
	autoscalerName := "test-autoscaler"

	dynamicClient, err := dynamic.NewForConfig(suite.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	suite.Require().NoError(err)

	defer func() {
		_ = dynamicClient.Resource(DatadogPodAutoscalerGVR).Namespace(namespace).Delete(ctx, autoscalerName, metav1.DeleteOptions{})
		_ = suite.Env().KubernetesCluster.Client().AppsV1().Deployments(namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
		_ = suite.Env().KubernetesCluster.Client().CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	}()

	_, err = suite.Env().KubernetesCluster.Client().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})
	suite.Require().NoError(err)

	// Create a deployment with main and init containers
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": deploymentName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": deploymentName,
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:    "init-sidecar-container",
							Image:   "busybox:1.35",
							Command: []string{"sh", "-c", "while true; do echo 'Init sidecar container running'; sleep 30; done"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("64Mi"),
								},
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse("50m"),
									"memory": resource.MustParse("32Mi"),
								},
							},
							RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "main-container",
							Image:   "busybox:1.35",
							Command: []string{"sh", "-c", "while true; do echo 'Main container running'; sleep 30; done"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									"cpu":    resource.MustParse("200m"),
									"memory": resource.MustParse("128Mi"),
								},
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("64Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = suite.Env().KubernetesCluster.Client().AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	suite.Require().NoError(err)

	suite.EventuallyWithTf(func(c *assert.CollectT) {
		dep, err := suite.Env().KubernetesCluster.Client().AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, int32(1), dep.Status.ReadyReplicas, "Deployment should have 1 ready replica")
	}, 2*time.Minute, 10*time.Second, "Deployment should be ready")

	autoscaler := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: namespace,
		},
		Spec: datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			},
			Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
			Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
				{
					Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
					PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
						Name: "cpu",
						Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
							Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
							Utilization: pointer.Ptr(int32(50)),
						},
					},
				},
			},
		},
	}

	unstructuredAutoscaler, err := convertDatadogPodAutoscalerToUnstructured(autoscaler)
	suite.Require().NoError(err)

	_, err = dynamicClient.Resource(DatadogPodAutoscalerGVR).Namespace(namespace).Create(ctx, unstructuredAutoscaler, metav1.CreateOptions{})
	suite.Require().NoError(err)

	suite.EventuallyWithTf(func(c *assert.CollectT) {
		autoscaler, err := dynamicClient.Resource(DatadogPodAutoscalerGVR).Namespace(namespace).Get(ctx, autoscalerName, metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}
		assert.NotNil(c, autoscaler, "Autoscaler should exist")
	}, 1*time.Minute, 5*time.Second, "Autoscaler should be created")

	suite.EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + deploymentName,
		})
		if !assert.NoError(c, err) {
			return
		}

		if !assert.Greater(c, len(pods.Items), 0, "Should have at least one pod") {
			return
		}

		pod := pods.Items[0]

		assert.Len(c, pod.Spec.InitContainers, 1, "Pod should have 1 init container")
		assert.Len(c, pod.Spec.Containers, 1, "Pod should have 1 main container")

		initContainer := pod.Spec.InitContainers[0]
		assert.Equal(c, "init-sidecar-container", initContainer.Name)
		assert.Equal(c, resource.MustParse("100m"), initContainer.Resources.Limits["cpu"])
		assert.Equal(c, resource.MustParse("64Mi"), initContainer.Resources.Limits["memory"])

		mainContainer := pod.Spec.Containers[0]
		assert.Equal(c, "main-container", mainContainer.Name)
		assert.Equal(c, resource.MustParse("200m"), mainContainer.Resources.Limits["cpu"])
		assert.Equal(c, resource.MustParse("128Mi"), mainContainer.Resources.Limits["memory"])

		assert.Equal(c, corev1.PodRunning, pod.Status.Phase, "Pod should be running")
	}, 3*time.Minute, 10*time.Second, "Pod should be running with correct initial resources")

	// Update autoscaler status with new replica count
	currentAutoscaler, err := dynamicClient.Resource(DatadogPodAutoscalerGVR).Namespace(namespace).Get(ctx, autoscalerName, metav1.GetOptions{})
	suite.Require().NoError(err)

	var typedAutoscaler datadoghq.DatadogPodAutoscaler
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(currentAutoscaler.Object, &typedAutoscaler)
	suite.Require().NoError(err)

	typedAutoscaler.Status.CurrentReplicas = pointer.Ptr(int32(2))

	unstructuredAutoscaler, err = convertDatadogPodAutoscalerToUnstructured(&typedAutoscaler)
	suite.Require().NoError(err)

	_, err = dynamicClient.Resource(DatadogPodAutoscalerGVR).Namespace(namespace).UpdateStatus(ctx, unstructuredAutoscaler, metav1.UpdateOptions{})
	suite.Require().NoError(err)

	// Wait for status to be properly applied to autoscaler object in cluster
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		updatedAutoscaler, err := dynamicClient.Resource(DatadogPodAutoscalerGVR).Namespace(namespace).Get(ctx, autoscalerName, metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}

		status, found, err := unstructured.NestedFieldNoCopy(updatedAutoscaler.Object, "status")
		if !assert.NoError(c, err) {
			return
		}
		if !assert.True(c, found, "Status field should exist") {
			return
		}

		statusMap, ok := status.(map[string]interface{})
		if !assert.True(c, ok, "Status should be a map") {
			return
		}

		currentReplicas, found, err := unstructured.NestedInt64(statusMap, "currentReplicas")
		if !assert.NoError(c, err) {
			return
		}
		if !assert.True(c, found, "currentReplicas field should exist") {
			return
		}

		assert.Equal(c, int64(2), currentReplicas, "Current replicas should be 2")
	}, 1*time.Minute, 5*time.Second, "Autoscaler status should be updated with 2 replicas")

	// Wait and check that number of pods in test-autoscaling-deployment became 2
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		deployment, err := suite.Env().KubernetesCluster.Client().AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, int32(2), deployment.Status.ReadyReplicas, "Deployment should have 2 ready replicas")
	}, 5*time.Minute, 15*time.Second, "Deployment should scale to 2 replicas")
}
