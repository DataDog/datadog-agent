// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

const (
	kubeNamespaceDogstatsWorkload           = "workload-dogstatsd"
	kubeNamespaceDogstatsStandaloneWorkload = "workload-dogstatsd-standalone"
	kubeDeploymentDogstatsdUDP              = "dogstatsd-udp"
	kubeDeploymentDogstatsdUDPOrigin        = "dogstatsd-udp-origin-detection"
	kubeDeploymentDogstatsdUDPExternalData  = "dogstatsd-udp-external-data-only"
	kubeDeploymentDogstatsdUDS              = "dogstatsd-uds"
)

var GitCommit string

type k8sSuite struct {
	baseSuite[environments.Kubernetes]
}

func (suite *k8sSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.clusterName = suite.Env().KubernetesCluster.ClusterName
}

func (suite *k8sSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/qcp-brm-ysc/e2e-tests-containers-k8s?refresh_mode=paused&tpl_var_kube_cluster_name%%5B0%%5D=%s&tpl_var_fake_intake_task_family%%5B0%%5D=%s-fakeintake-ecs&from_ts=%d&to_ts=%d&live=false",
		suite.clusterName,
		suite.clusterName,
		suite.StartTime().UnixMilli(),
		suite.EndTime().UnixMilli(),
	))
}

// Once pulumi has finished to create a stack, it can still take some time for the images to be pulled,
// for the containers to be started, for the agent collectors to collect workload information
// and to feed workload meta and the tagger.
//
// We could increase the timeout of all tests to cope with the agent tagger warmup time.
// But in case of a single bug making a single tag missing from every metric,
// all the tests would time out and that would be a waste of time.
//
// It’s better to have the first test having a long timeout to wait for the agent to warmup,
// and to have the following tests with a smaller timeout.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The 00 in Test00UpAndRunning is here to guarantee that this test, waiting for the agent pods to be ready,
// is run first.
func (suite *k8sSuite) Test00UpAndRunning() {
	suite.testUpAndRunning(10 * time.Minute)
}

// An agent restart (because of a health probe failure or because of a OOM kill for ex.)
// can cause a completely random failure on a completely random test.
// A metric can be fully missing if the agent is restarted when the metric is checked.
// Only a subset of tags can be missing if the agent has just restarted, but not all the
// collectors have finished to feed workload meta and the tagger.
// So, checking if any agent has restarted during the tests can be valuable for investigations.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The ZZ in TestZZUpAndRunning is here to guarantee that this test, is run last.
func (suite *k8sSuite) TestZZUpAndRunning() {
	suite.testUpAndRunning(1 * time.Minute)
}

func (suite *k8sSuite) testUpAndRunning(waitFor time.Duration) {
	ctx := context.Background()

	suite.Run("agent pods are ready and not restarting", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			linuxNodes, err := suite.Env().KubernetesCluster.Client().CoreV1().Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "linux").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Linux nodes") {
				return
			}

			windowsNodes, err := suite.Env().KubernetesCluster.Client().CoreV1().Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "windows").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Windows nodes") {
				return
			}

			linuxPods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Linux datadog agent pods") {
				return
			}

			windowsPods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.WindowsNodeAgent.LabelSelectors["app"]).String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Windows datadog agent pods") {
				return
			}

			clusterAgentPods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list datadog cluster agent pods") {
				return
			}

			clusterChecksPods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxClusterChecks.LabelSelectors["app"]).String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list datadog cluster checks runner pods") {
				return
			}

			dogstatsdPods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("dogstatsd-standalone").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", "dogstatsd-standalone").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list dogstatsd standalone pods") {
				return
			}

			assert.Len(c, linuxPods.Items, len(linuxNodes.Items))
			assert.Len(c, windowsPods.Items, len(windowsNodes.Items))
			assert.NotEmpty(c, clusterAgentPods.Items)
			assert.NotEmpty(c, clusterChecksPods.Items)
			assert.Len(c, dogstatsdPods.Items, len(linuxNodes.Items))

			for _, podList := range []*corev1.PodList{linuxPods, windowsPods, clusterAgentPods, clusterChecksPods, dogstatsdPods} {
				for _, pod := range podList.Items {
					for _, containerStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
						assert.Truef(c, containerStatus.Ready, "Container %s of pod %s isn’t ready", containerStatus.Name, pod.Name)
						assert.Zerof(c, containerStatus.RestartCount, "Container %s of pod %s has restarted", containerStatus.Name, pod.Name)
					}
				}
			}
		}, waitFor, 10*time.Second, "Not all agents eventually became ready in time.")
	})
}

func (suite *k8sSuite) TestDogstatsdInAgent() {
	// Test with UDS
	suite.testDogstatsd(kubeNamespaceDogstatsWorkload, kubeDeploymentDogstatsdUDS)
	// Test with UDP + Origin detection
	//suite.testDogstatsd(kubeNamespaceDogstatsWorkload, kubeDeploymentDogstatsdUDPOrigin)
	// Test with UDP + DD_ENTITY_ID
	//suite.testDogstatsd(kubeNamespaceDogstatsWorkload, kubeDeploymentDogstatsdUDP)
	// Test with UDP + External Data
	//suite.testDogstatsd(kubeNamespaceDogstatsWorkload, kubeDeploymentDogstatsdUDPExternalData)
}

func (suite *k8sSuite) TestDogstatsdStandalone() {
	// Test with UDS
	suite.testDogstatsd(kubeNamespaceDogstatsStandaloneWorkload, kubeDeploymentDogstatsdUDS)
	// Dogstatsd standalone does not support origin detection
	// Test with UDP + DD_ENTITY_ID
	//suite.testDogstatsd(kubeNamespaceDogstatsWorkload, kubeDeploymentDogstatsdUDP)
}

func (suite *k8sSuite) testDogstatsd(kubeNamespace, kubeDeployment string) {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				"^kube_deployment:" + regexp.QuoteMeta(kubeDeployment) + "$",
				"^kube_namespace:" + regexp.QuoteMeta(kubeNamespace) + "$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:dogstatsd$`,
				`^display_container_name:dogstatsd`,
				`^git.commit.sha:`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source docker image label
				`^image_id:ghcr.io/datadog/apps-dogstatsd@sha256:`,
				`^image_name:ghcr.io/datadog/apps-dogstatsd$`,
				`^image_tag:main$`,
				`^kube_container_name:dogstatsd$`,
				`^kube_deployment:` + regexp.QuoteMeta(kubeDeployment) + `$`,
				"^kube_namespace:" + regexp.QuoteMeta(kubeNamespace) + "$",
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:` + regexp.QuoteMeta(kubeDeployment) + `-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:` + regexp.QuoteMeta(kubeDeployment) + `-[[:alnum:]]+$`,
				`^pod_name:` + regexp.QuoteMeta(kubeDeployment) + `-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^series:`,
				`^short_image:apps-dogstatsd$`,
			},
		},
	})
}
