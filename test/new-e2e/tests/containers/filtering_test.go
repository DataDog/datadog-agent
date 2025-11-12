// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
)

// k8sFilteringSuite tests container filtering behavior in Kubernetes environments
type k8sFilteringSuite struct {
	baseSuite[environments.Kubernetes]
}

const (
	filteredAppName      = "nginx"
	filteredAppNamespace = "filtered-ns"
)

// TestK8SFilteringSuite runs the Kubernetes filtering test suite
func TestK8SFilteringSuite(t *testing.T) {
	e2e.Run(t, &k8sFilteringSuite{}, e2e.WithProvisioner(
		awskubernetes.KindProvisioner(
			awskubernetes.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(`
datadog:
  containerExclude: "kube_namespace:filtered-ns name:redis*"
`),
			),
			awskubernetes.WithDeployTestWorkload(),
			awskubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, filteredAppNamespace, "", false, nil)
			}),
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
	suite.Never(func() bool {
		containerMetrics := []string{
			"container.cpu.usage",
			"container.memory.usage",
			"container.memory.working_set",
			"container.io.read_bytes",
			"container.io.write_bytes",
		}

		foundMetric := false
		for _, metricName := range containerMetrics {
			metrics, err := suite.Fakeintake.FilterMetrics(
				metricName,
				fakeintake.WithTags[*aggregator.MetricSeries]([]string{
					`kube_namespace` + filteredAppNamespace,
				}),
			)
			suite.NoError(err, "Error querying metrics")
			if len(metrics) > 0 {
				foundMetric = true
			}
		}
		return foundMetric
	}, 2*time.Minute, 15*time.Second, "Metrics were found for a container in a filtered namespace")

	suite.Never(func() bool {
		containerMetrics := []string{
			"container.cpu.usage",
			"container.memory.usage",
			"container.memory.working_set",
			"container.io.read_bytes",
			"container.io.write_bytes",
		}

		foundMetric := false
		for _, metricName := range containerMetrics {
			metrics, err := suite.Fakeintake.FilterMetrics(
				metricName,
				fakeintake.WithTags[*aggregator.MetricSeries]([]string{
					`container_name:` + "redis",
				}),
			)
			suite.NoError(err, "Error querying metrics")
			if len(metrics) > 0 {
				foundMetric = true
			}
		}
		return foundMetric
	}, 2*time.Minute, 15*time.Second, "Metrics were found for a container filtered by name")
}

// TestFilteredContainerNoAutodiscovery verifies that integrations are NOT auto-discovered
// on containers that match the exclusion filter, even if they have AD annotations
func (suite *k8sFilteringSuite) TestFilteredContainerNoAutodiscovery() {
	// Query for nginx integration metrics should NOT exist
	suite.Never(func() bool {
		nginxMetrics := []string{
			"nginx.net.request_per_s",
			"nginx.net.conn_active",
			"nginx.net.conn_reading",
			"nginx.net.conn_writing",
			"nginx.net.conn_waiting",
		}

		foundMetric := false
		for _, metricName := range nginxMetrics {
			suite.Run(metricName, func() {
				metrics, err := suite.Fakeintake.FilterMetrics(
					metricName,
					fakeintake.WithTags[*aggregator.MetricSeries]([]string{
						`container_name:` + filteredAppName,
					}),
				)
				suite.NoError(err, "Error querying metrics")
				if len(metrics) > 0 {
					foundMetric = true
				}
			})
		}
		return foundMetric
	}, 2*time.Minute, 15*time.Second, "Metrics were found for a container filtered by namespace")
}
