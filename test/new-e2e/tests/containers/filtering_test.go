// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	localkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/local/kubernetes"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

// k8sFilteringSuite tests container filtering behavior in Kubernetes environments
type k8sFilteringSuite struct {
	baseSuite[environments.Kubernetes]
}

const (
	filteredAppName      = "filtered-nginx"
	filteredAppNamespace = "workload-filtering"
)

// TestK8SFilteringSuite runs the Kubernetes filtering test suite
func TestK8SFilteringSuite(t *testing.T) {
	e2e.Run(t, &k8sFilteringSuite{}, e2e.WithProvisioner(
		localkubernetes.Provisioner(
			localkubernetes.WithAgentOptions(
				// Configure agent to exclude containers matching the filter
				kubernetesagentparams.WithHelmValues(`
datadog:
  containerExclude: "name:filtered-.*"
`),
			),
			localkubernetes.WithWorkloadApp(deployFilteredNginxWorkload),
		),
	))
}

func (suite *k8sFilteringSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.clusterName = suite.Env().KubernetesCluster.ClusterName
}

// TestFilteredContainerNoMetrics verifies that the container core check does not collect
// telemetry for containers that match the exclusion filter
func (suite *k8sFilteringSuite) TestFilteredContainerNoMetrics() {
	ctx := context.Background()

	// Wait for the workload to be deployed and running
	suite.EventuallyWithT(func(c *assert.CollectT) {
		deployment, err := suite.Env().KubernetesCluster.Client().AppsV1().Deployments(filteredAppNamespace).Get(
			ctx,
			filteredAppName,
			metav1.GetOptions{},
		)
		assert.NoError(c, err, "Failed to get filtered nginx deployment")
		if deployment != nil {
			assert.Greater(c, deployment.Status.ReadyReplicas, int32(0), "No ready replicas for filtered nginx")
		}
	}, 2*time.Minute, 10*time.Second)

	// Wait a reasonable time for metrics to potentially arrive (if filtering is broken)
	time.Sleep(30 * time.Second)

	// Query for container metrics that should NOT exist for the filtered container
	containerMetrics := []string{
		"container.cpu.usage",
		"container.memory.usage",
		"container.memory.working_set",
		"container.io.read_bytes",
		"container.io.write_bytes",
	}

	for _, metricName := range containerMetrics {
		suite.Run(metricName, func() {
			metrics, err := suite.Fakeintake.FilterMetrics(
				metricName,
				client.WithTags[*aggregator.MetricSeries]([]string{
					`container_name:` + filteredAppName,
				}),
			)
			suite.NoError(err, "Error querying metrics")
			suite.Empty(metrics, "Expected no %s metrics for filtered container %s, but found %d metrics",
				metricName, filteredAppName, len(metrics))
		})
	}

	// Also verify Kubernetes-specific metrics are not collected
	k8sMetrics := []string{
		"kubernetes.cpu.usage.total",
		"kubernetes.memory.usage",
		"kubernetes.memory.working_set",
	}

	for _, metricName := range k8sMetrics {
		suite.Run(metricName, func() {
			metrics, err := suite.Fakeintake.FilterMetrics(
				metricName,
				client.WithTags[*aggregator.MetricSeries]([]string{
					`kube_container_name:` + filteredAppName,
				}),
			)
			suite.NoError(err, "Error querying metrics")
			suite.Empty(metrics, "Expected no %s metrics for filtered container %s, but found %d metrics",
				metricName, filteredAppName, len(metrics))
		})
	}
}

// TestFilteredContainerNoAutodiscovery verifies that integrations are NOT auto-discovered
// on containers that match the exclusion filter, even if they have AD annotations
func (suite *k8sFilteringSuite) TestFilteredContainerNoAutodiscovery() {
	ctx := context.Background()

	// Wait for the workload to be deployed and running
	suite.EventuallyWithT(func(c *assert.CollectT) {
		deployment, err := suite.Env().KubernetesCluster.Client().AppsV1().Deployments(filteredAppNamespace).Get(
			ctx,
			filteredAppName,
			metav1.GetOptions{},
		)
		assert.NoError(c, err, "Failed to get filtered nginx deployment")
		if deployment != nil {
			assert.Greater(c, deployment.Status.ReadyReplicas, int32(0), "No ready replicas for filtered nginx")
		}
	}, 2*time.Minute, 10*time.Second)

	// Wait a reasonable time for autodiscovery to potentially occur (if filtering is broken)
	time.Sleep(30 * time.Second)

	// Query for nginx integration metrics that should NOT exist
	nginxMetrics := []string{
		"nginx.net.request_per_s",
		"nginx.net.conn_active",
		"nginx.net.conn_reading",
		"nginx.net.conn_writing",
		"nginx.net.conn_waiting",
	}

	for _, metricName := range nginxMetrics {
		suite.Run(metricName, func() {
			metrics, err := suite.Fakeintake.FilterMetrics(
				metricName,
				client.WithTags[*aggregator.MetricSeries]([]string{
					`kube_container_name:` + filteredAppName,
				}),
			)
			suite.NoError(err, "Error querying metrics")
			suite.Empty(metrics, "Expected no nginx integration metric %s for filtered container, but found %d metrics",
				metricName, len(metrics))
		})
	}

	// Verify that no nginx check runs exist for the filtered container
	suite.Run("check_runs", func() {
		checkRuns, err := suite.Fakeintake.FilterCheckRuns("nginx")
		suite.NoError(err, "Error querying check runs")

		// Filter check runs to find any that match our filtered container
		filteredCheckRuns := make([]*aggregator.CheckRun, 0)
		for _, checkRun := range checkRuns {
			for _, tag := range checkRun.Tags {
				if matched, _ := regexp.MatchString(`kube_container_name:`+filteredAppName, tag); matched {
					filteredCheckRuns = append(filteredCheckRuns, checkRun)
					break
				}
			}
		}

		suite.Empty(filteredCheckRuns, "Expected no nginx check runs for filtered container, but found %d check runs",
			len(filteredCheckRuns))
	})
}

// deployFilteredNginxWorkload deploys an nginx application with autodiscovery annotations
// but with a name that matches the container exclusion filter
func deployFilteredNginxWorkload(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	namespace, err := corev1.NewNamespace(e.Ctx(), e.Namer().ResourceName(filteredAppNamespace), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(filteredAppNamespace),
		},
	}, pulumi.Provider(kubeProvider))
	if err != nil {
		return nil, err
	}

	// Deploy nginx with autodiscovery annotations
	// The container name matches the filter pattern "name:filtered-.*"
	// so it should be excluded despite having AD annotations
	deployment, err := appsv1.NewDeployment(e.Ctx(), e.Namer().ResourceName(filteredAppName), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(filteredAppName),
			Namespace: namespace.Metadata.Name(),
			Annotations: pulumi.StringMap{
				// Autodiscovery annotations for nginx integration
				"ad.datadoghq.com/filtered-nginx.check_names":  pulumi.String(`["nginx"]`),
				"ad.datadoghq.com/filtered-nginx.init_configs": pulumi.String(`[{}]`),
				"ad.datadoghq.com/filtered-nginx.instances":    pulumi.String(`[{"nginx_status_url": "http://%%host%%:8080/nginx_status"}]`),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String(filteredAppName),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String(filteredAppName),
					},
					Annotations: pulumi.StringMap{
						// Pod-level annotations for autodiscovery
						"ad.datadoghq.com/filtered-nginx.check_names":  pulumi.String(`["nginx"]`),
						"ad.datadoghq.com/filtered-nginx.init_configs": pulumi.String(`[{}]`),
						"ad.datadoghq.com/filtered-nginx.instances":    pulumi.String(`[{"nginx_status_url": "http://%%host%%:8080/nginx_status"}]`),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String(filteredAppName),
							Image: pulumi.String("ghcr.io/datadog/apps-nginx:" + apps.Version),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(8080),
									Name:          pulumi.String("http"),
								},
							},
						},
					},
				},
			},
		},
	}, pulumi.Provider(kubeProvider), pulumi.DependsOn([]pulumi.Resource{namespace}))
	if err != nil {
		return nil, err
	}

	return &kubeComp.Workload{
		Deployment: deployment,
	}, nil
}
