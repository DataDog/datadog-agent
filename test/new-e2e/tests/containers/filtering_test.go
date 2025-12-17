// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	_ "embed"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awskind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed fixtures/datadog-agent-legacy-exclude.yml
var legacyContainerExcludeConfig string

//go:embed fixtures/datadog-agent-cel-exclude.yml
var celContainerExcludeConfig string

const (
	filteredAppName   = "redis"
	filteredNamespace = "filtered-ns"
)

// k8sFilteringSuiteBase provides common test methods for filtering test suites
type k8sFilteringSuiteBase struct {
	baseSuite[environments.Kubernetes]
}

func (suite *k8sFilteringSuiteBase) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

// TestWorkloadExcludeNoMetrics verifies that the container core check does not collect
// telemetry for workloads that match the exclusion filter
func (suite *k8sFilteringSuiteBase) TestWorkloadExcludeNoMetrics() {
	// nginx workload in filtered namespace should never have metrics
	suite.Never(func() bool {
		metrics, err := suite.Fakeintake.FilterMetrics(
			"container.cpu.usage",
			fakeintake.WithTags[*aggregator.MetricSeries]([]string{
				`kube_namespace:` + filteredNamespace,
			}),
		)
		suite.NoError(err, "Error querying metrics")
		return len(metrics) > 0
	}, 1*time.Minute, 5*time.Second, "Metrics were found for a workload in a filtered namespace")
}

// TestWorkloadExcludeForAutodiscovery verifies that integrations are NOT auto-discovered
// on workloads that match the exclusion filter, even for auto config enabled integrations
func (suite *k8sFilteringSuiteBase) TestWorkloadExcludeForAutodiscovery() {
	// redis workload is excluded and should not have auto-config metrics
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

// TestUnfilteredWorkloadsHaveTelemetry confirms that workloads not matched by the exclude filter
// continue to run and collect telemetry.
func (suite *k8sFilteringSuiteBase) TestUnfilteredWorkloadsHaveTelemetry() {
	// nginx workload in default namespace should have metrics
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.memory.usage",
			Tags: []string{
				`^container_name:nginx$`,
				`^kube_namespace:workload-nginx$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags:                 &[]string{},
			AcceptUnexpectedTags: true,
		},
	})
}

// k8sLegacyFilteringSuite tests legacy container filtering behavior in Kubernetes environments
type k8sLegacyFilteringSuite struct {
	k8sFilteringSuiteBase
}

// TestK8SLegacyFilteringSuite runs the Kubernetes legacy filtering test suite
func TestK8SLegacyFilteringSuite(t *testing.T) {
	e2e.Run(t, &k8sLegacyFilteringSuite{}, e2e.WithProvisioner(
		awskind.Provisioner(
			awskind.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(legacyContainerExcludeConfig),
				),
				scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
					return nginx.K8sAppDefinition(e, kubeProvider, "workload-nginx", 80, "", false, nil)
				}),
				scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
					return redis.K8sAppDefinition(e, kubeProvider, "default", false, nil)
				}),
				// Deploy additional nginx workload except in an excluded namespace
				scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
					return nginx.K8sAppDefinition(e, kubeProvider, filteredNamespace, 80, "", false, nil)
				}),
			)),
	))
}

// k8sCELFilteringSuite tests CEL-based workload filtering in Kubernetes environments
type k8sCELFilteringSuite struct {
	k8sFilteringSuiteBase
}

// TestK8SCELFilteringSuite runs the Kubernetes CEL filtering test suite
func TestK8SCELFilteringSuite(t *testing.T) {
	e2e.Run(t, &k8sCELFilteringSuite{}, e2e.WithProvisioner(
		awskind.Provisioner(
			awskind.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(celContainerExcludeConfig),
				),
				scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
					return nginx.K8sAppDefinition(e, kubeProvider, "workload-nginx", 80, "", false, nil)
				}),
				scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
					return redis.K8sAppDefinition(e, kubeProvider, "default", false, nil)
				}),
				// Deploy additional nginx workload except in an excluded namespace
				scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
					return nginx.K8sAppDefinition(e, kubeProvider, filteredNamespace, 80, "", false, nil)
				}),
			)),
	))
}
