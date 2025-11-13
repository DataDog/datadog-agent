// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	_ "embed"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
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
					`kube_namespace:` + filteredNamespace,
				}),
			)
			suite.NoError(err, "Error querying metrics")
			if len(metrics) > 0 {
				foundMetric = true
			}
		}
		return foundMetric
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

	suite.testLog(&testLogArgs{
		Filter: testLogFilterArgs{
			Service: "apps-nginx-server",
			Tags: []string{
				`^kube_namespace:workload-nginx$`,
			},
		},
		Expect: testLogExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:nginx$`,
				`^dirname:/var/log/pods/workload-nginx_nginx-`,
				`^display_container_name:nginx`,
				`^filename:[[:digit:]]+.log$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`, // org.opencontainers.image.revision docker image label
				`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source docker image label
				`^image_id:ghcr\.io/datadog/apps-nginx-server@sha256:`,
				`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^kube_container_name:nginx$`,
				`^kube_deployment:nginx$`,
				`^kube_namespace:workload-nginx$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:nginx-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:nginx-[[:alnum:]]+$`,
				`^kube_service:nginx$`,
				`^pod_name:nginx-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^short_image:apps-nginx-server$`,
				`^domain:deployment$`,
				`^mail:team-container-platform@datadoghq.com$`,
				`^org:agent-org$`,
				`^parent-name:nginx$`,
				`^team:contp$`,
			},
			Message: `GET / HTTP/1\.1`,
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
		awskubernetes.KindProvisioner(
			awskubernetes.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(legacyContainerExcludeConfig),
			),
			awskubernetes.WithDeployTestWorkload(),
			// Deploy additional nginx workload except in an excluded namespace
			awskubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, filteredNamespace, "", false, nil)
			}),
		),
	))
}

// k8sCELFilteringSuite tests CEL-based workload filtering in Kubernetes environments
type k8sCELFilteringSuite struct {
	k8sFilteringSuiteBase
}

// TestK8SCELFilteringSuite runs the Kubernetes CEL filtering test suite
func TestK8SCELFilteringSuite(t *testing.T) {
	e2e.Run(t, &k8sCELFilteringSuite{}, e2e.WithProvisioner(
		awskubernetes.KindProvisioner(
			awskubernetes.WithAgentOptions(
				kubernetesagentparams.WithAgentFullImagePath("public.ecr.aws/datadog/agent:7.73.0-rc.6"),
				kubernetesagentparams.WithHelmValues(celContainerExcludeConfig),
			),
			awskubernetes.WithDeployTestWorkload(),
			// Deploy additional nginx workload except in an excluded namespace
			awskubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, filteredNamespace, "", false, nil)
			}),
		),
	))
}
