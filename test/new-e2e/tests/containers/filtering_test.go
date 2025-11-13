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
	filteredAppName   = "redis"
	filteredNamespace = "filtered-ns"
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
				return nginx.K8sAppDefinition(e, kubeProvider, filteredNamespace, "", false, nil)
			}),
		),
	))
}

func (suite *k8sFilteringSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.clusterName = suite.Env().KubernetesCluster.ClusterName
}

// TestContainerExcludeNoMetrics verifies that the container core check does not collect
// telemetry for containers that match the exclusion filter
func (suite *k8sFilteringSuite) TestContainerExcludeNoMetrics() {
	// nginx workload in filtered namespace should never have metrics
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
					`kube_namespace` + filteredNamespace,
				}),
			)
			suite.NoError(err, "Error querying metrics")
			if len(metrics) > 0 {
				foundMetric = true
			}
		}
		return foundMetric
	}, 1*time.Minute, 5*time.Second, "Metrics were found for a container in a filtered namespace")

	// nginx workload in default namespace should have metrics
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: []string{
				`^container_name:nginx$`,
				`^kube_namespace:workload-nginx$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_name:nginx$`,
				`^display_container_name:nginx`,
				`^kube_container_name:nginx$`,
				`^kube_deployment:nginx$`,
				`^kube_namespace:workload-nginx$`,
				`^kube_service:nginx$`,
			},
			AcceptUnexpectedTags: true,
		},
	})
}

// TestContainerExcludeForAutodiscovery verifies that integrations are NOT auto-discovered
// on containers that match the exclusion filter, even for auto config enabled integrations
func (suite *k8sFilteringSuite) TestContainerExcludeForAutodiscovery() {
	// redis workload is excluded by its container name and should not have auto-config metrics
	suite.Never(func() bool {
		metrics, err := suite.Fakeintake.FilterMetrics(
			"redis.net.instantaneous_ops_per_sec",
			fakeintake.WithTags[*aggregator.MetricSeries]([]string{
				`container_name:` + filteredAppName,
			}),
		)
		suite.NoError(err, "Error querying metrics")
		return len(metrics) > 0
	}, 1*time.Minute, 5*time.Second, "Metrics were found for filtered redis workload")
}
